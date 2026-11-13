package atlassian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrUnknownStatusCategory  = errors.New("atlassian: unknown status category")
	ErrFailedToParseStartDate = errors.New("atlassian: failed to parse start date")
	ErrFailedToParseDueDate   = errors.New("atlassian: failed to parse due date")
	ErrFailedToParseClosedAt  = errors.New("atlassian: failed to parse status category change date")
	ErrWorkItemNotFound       = errors.New("atlassian: work item not found")
)

const (
	searchPath = "/rest/api/3/search/jql"
	issuePath  = "/rest/api/3/issue/"

	// maxSearchPages caps the pages one GetMyTasks follows (100 pages of
	// searchMaxResults = 10,000 work items) so a server that never ends the
	// token chain cannot loop until ctx ends.
	maxSearchPages = 100

	myTasksJQL = "assignee = currentUser() AND statusCategory != done"

	// Jira Cloud's default "Start date" field.
	startDateField = "customfield_10015"

	// searchMaxResults is the requested page size; Jira may return fewer
	// items per page when many fields are requested.
	searchMaxResults = 100
)

// GetMyTasks returns the Jira work items assigned to the current user that are
// not done, following every search page. A failed page, a repeated page token,
// or more than maxSearchPages pages returns nil and the error. Items that fail
// to decode or convert are reported in the joined error alongside the tasks
// that succeeded.
func (c *client) GetMyTasks(ctx context.Context) ([]task.Task, error) {
	a, err := c.current()
	if err != nil {
		return nil, err
	}

	tasks := []task.Task{}

	var (
		errs  []error
		token string
		seen  int
	)

	for pages := 1; ; pages++ {
		page, err := c.search(ctx, a, token)
		if err != nil {
			return nil, err
		}

		for i, raw := range page.Issues {
			index := seen + i

			var item ViewWorkItem
			if err := json.Unmarshal(raw, &item); err != nil {
				errs = append(errs, errors.Join(fmt.Errorf("%w: index=%d", ErrFailedToParseWorkItem, index), err))
				continue
			}

			if item.Key == "" {
				errs = append(errs, fmt.Errorf("%w: index=%d", ErrMissingWorkItemKey, index))
				continue
			}

			t, err := ToTask(item)
			errs = append(errs, err)
			tasks = append(tasks, t)
		}

		seen += len(page.Issues)

		if page.IsLast || page.NextPageToken == "" {
			return tasks, errors.Join(errs...)
		}

		if page.NextPageToken == token {
			return nil, fmt.Errorf("%w: page=%d", ErrRepeatedSearchPageToken, pages)
		}

		if pages == maxSearchPages {
			return nil, fmt.Errorf("%w: max=%d", ErrTooManySearchPages, maxSearchPages)
		}

		token = page.NextPageToken
	}
}

func (c *client) search(ctx context.Context, a api, token string) (searchResponse, error) {
	body, err := json.Marshal(searchRequest{
		JQL:           myTasksJQL,
		Fields:        searchFields(),
		MaxResults:    searchMaxResults,
		NextPageToken: token,
	})
	if err != nil {
		return searchResponse{}, errors.Join(ErrFailedToEncodeSearchRequest, err)
	}

	out, err := c.http.Post(ctx, a.base+searchPath, a.header, body)
	if err != nil {
		return searchResponse{}, err
	}

	var page searchResponse
	if err := json.Unmarshal(out, &page); err != nil {
		return searchResponse{}, errors.Join(ErrFailedToParseSearchOutput, err)
	}

	return page, nil
}

// GetTask returns the Jira work item key as a task. It fails with
// ErrWorkItemNotFound when Jira answers 404, and with ToTask's error, without
// a task, when a field fails to convert.
func (c *client) GetTask(ctx context.Context, key string) (task.Task, error) {
	a, err := c.current()
	if err != nil {
		return task.Task{}, err
	}

	query := url.Values{"fields": {strings.Join(searchFields(), ",")}}
	rawURL := a.base + issuePath + url.PathEscape(key) + "?" + query.Encode()

	out, err := c.http.Get(ctx, rawURL, a.header)
	if statusErr, ok := errors.AsType[*httputil.StatusError](err); ok && statusErr.StatusCode == http.StatusNotFound {
		return task.Task{}, fmt.Errorf("%w: key=%s", ErrWorkItemNotFound, key)
	}

	if err != nil {
		return task.Task{}, err
	}

	var item ViewWorkItem
	if err := json.Unmarshal(out, &item); err != nil {
		return task.Task{}, errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseWorkItem, key), err)
	}

	t, err := ToTask(item)
	if err != nil {
		return task.Task{}, err
	}

	return t, nil
}

// searchFields returns a fresh slice of the fields every search page returns.
func searchFields() []string {
	return []string{
		"summary", "description", "status", "statuscategorychangedate", "duedate", "parent", "subtasks", "issuelinks",
		startDateField,
	}
}

type ViewWorkItem struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string   `json:"summary"`
		Description *adfNode `json:"description"`
		Status      struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key StatusCategoryKey `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		StatusCategoryChangeDate string       `json:"statuscategorychangedate"`
		DueDate                  string       `json:"duedate"`
		StartDate                string       `json:"customfield_10015"`
		Parent                   *LinkedItem  `json:"parent"`
		Subtasks                 []LinkedItem `json:"subtasks"`
		IssueLinks               []IssueLink  `json:"issuelinks"`
	} `json:"fields"`
}

type LinkedItem struct {
	Key string `json:"key"`
}

// IssueLink holds the other side of a link in either inwardIssue or
// outwardIssue, depending on the link direction.
type IssueLink struct {
	InwardIssue  *LinkedItem `json:"inwardIssue"`
	OutwardIssue *LinkedItem `json:"outwardIssue"`
}

// ToTask converts a work item even when some fields are invalid: invalid
// fields are left empty and their errors are joined into the returned error.
// The task ID and the related task IDs are "jira-{key}", while error messages
// keep the raw key.
func ToTask(item ViewWorkItem) (task.Task, error) {
	f := item.Fields

	status, statusErr := ToStatus(f.Status.Name, f.Status.StatusCategory.Key)
	if statusErr != nil {
		statusErr = fmt.Errorf("%w: key=%s", statusErr, item.Key)
	}

	startDate, startDateErr := parseDate(f.StartDate)
	if startDateErr != nil {
		startDateErr = errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseStartDate, item.Key), startDateErr)
	}

	dueDate, dueDateErr := parseDate(f.DueDate)
	if dueDateErr != nil {
		dueDateErr = errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseDueDate, item.Key), dueDateErr)
	}

	var (
		closedAt    *time.Time
		closedAtErr error
	)
	if status.Closed() {
		closedAt, closedAtErr = parseDateTime(f.StatusCategoryChangeDate)
		if closedAtErr != nil {
			closedAtErr = errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToParseClosedAt, item.Key), closedAtErr)
		}
	}

	var parentID string
	if f.Parent != nil && f.Parent.Key != "" {
		parentID = task.NewID(task.OriginJira, f.Parent.Key)
	}

	var childrenIDs []string

	for _, subtask := range f.Subtasks {
		if subtask.Key != "" {
			childrenIDs = append(childrenIDs, task.NewID(task.OriginJira, subtask.Key))
		}
	}

	var linkedIDs []string

	for _, link := range f.IssueLinks {
		for _, linked := range []*LinkedItem{link.InwardIssue, link.OutwardIssue} {
			if linked != nil && linked.Key != "" {
				linkedIDs = append(linkedIDs, task.NewID(task.OriginJira, linked.Key))
			}
		}
	}

	return task.Task{
		ID:          task.NewID(task.OriginJira, item.Key),
		Originated:  task.OriginJira,
		Title:       f.Summary,
		Description: f.Description.markdown(),
		Status:      status,
		StartDate:   startDate,
		DueDate:     dueDate,
		ParentID:    parentID,
		ChildrenIDs: childrenIDs,
		LinkedIDs:   linkedIDs,
		ClosedAt:    closedAt,
	}, errors.Join(statusErr, startDateErr, dueDateErr, closedAtErr)
}

func parseDate(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	t, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// jiraDateTimeLayout is how Jira writes date-time fields, such as
// "2026-10-23T14:05:09.123+0900". The fraction is optional when parsing.
const jiraDateTimeLayout = "2006-01-02T15:04:05.999999999-0700"

// parseDateTime returns a Jira date-time at UTC. It also takes RFC 3339, whose
// offset has a colon.
func parseDateTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	t, err := time.Parse(jiraDateTimeLayout, value)
	if err != nil {
		var rfcErr error
		if t, rfcErr = time.Parse(time.RFC3339Nano, value); rfcErr != nil {
			return nil, err
		}
	}

	t = t.UTC()

	return &t, nil
}

type StatusCategoryKey string

const (
	StatusCategoryKeyNew           StatusCategoryKey = "new"
	StatusCategoryKeyIndeterminate StatusCategoryKey = "indeterminate"
	StatusCategoryKeyDone          StatusCategoryKey = "done"
)

const (
	StatusNameCanceled = "Canceled"
)

// ToStatus maps Jira status categories to task statuses. Jira has no
// "canceled" category, so the Canceled status (category done) is matched by name.
func ToStatus(name string, categoryKey StatusCategoryKey) (task.Status, error) {
	if name == StatusNameCanceled {
		return task.StatusCanceled, nil
	}

	switch categoryKey {
	case StatusCategoryKeyNew:
		return task.StatusTodo, nil
	case StatusCategoryKeyIndeterminate:
		return task.StatusInProgress, nil
	case StatusCategoryKeyDone:
		return task.StatusDone, nil
	}

	return "", fmt.Errorf("%w: status=%s, category=%s", ErrUnknownStatusCategory, name, categoryKey)
}
