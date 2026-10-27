package atlassian

import (
	"errors"
	"fmt"
	"time"

	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrUnknownStatusCategory  = errors.New("atlassian: unknown status category")
	ErrFailedToParseStartDate = errors.New("atlassian: failed to parse start date")
	ErrFailedToParseDueDate   = errors.New("atlassian: failed to parse due date")
)

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
		DueDate    string       `json:"duedate"`
		StartDate  string       `json:"customfield_10015"`
		Parent     *LinkedItem  `json:"parent"`
		Subtasks   []LinkedItem `json:"subtasks"`
		IssueLinks []IssueLink  `json:"issuelinks"`
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
	}, errors.Join(statusErr, startDateErr, dueDateErr)
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

const (
	myTasksJQL = "assignee = currentUser() AND statusCategory != done"

	// Jira Cloud's default "Start date" field.
	startDateField = "customfield_10015"

	// searchMaxResults is the requested page size; Jira may return fewer
	// items per page when many fields are requested.
	searchMaxResults = 100
)

// searchFields returns a fresh slice of the fields every search page returns.
func searchFields() []string {
	return []string{"summary", "description", "status", "duedate", "parent", "subtasks", "issuelinks", startDateField}
}
