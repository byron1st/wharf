package atlassian

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/tool"
)

var (
	ErrUnknownStatusCategory  = errors.New("atlassian: unknown status category")
	ErrFailedToParseStartDate = errors.New("atlassian: failed to parse start date")
	ErrFailedToParseDueDate   = errors.New("atlassian: failed to parse due date")
)

type SearchItem struct {
	Key string `json:"key"`
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
	if f.Parent != nil {
		parentID = f.Parent.Key
	}

	var childrenIDs []string

	for _, subtask := range f.Subtasks {
		if subtask.Key != "" {
			childrenIDs = append(childrenIDs, subtask.Key)
		}
	}

	var linkedIDs []string

	for _, link := range f.IssueLinks {
		for _, linked := range []*LinkedItem{link.InwardIssue, link.OutwardIssue} {
			if linked != nil && linked.Key != "" {
				linkedIDs = append(linkedIDs, linked.Key)
			}
		}
	}

	return task.Task{
		ID:          item.Key,
		Originated:  task.OriginJira,
		Title:       f.Summary,
		Description: f.Description.plainText(),
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

	// Jira Cloud's default "Start date" field. `workitem search` rejects date
	// fields, so they are fetched per work item with `workitem view`.
	startDateField = "customfield_10015"
	viewFields     = "summary,description,status,duedate,parent,subtasks,issuelinks," + startDateField
)

// JiraSearchOption returns the CLI option that searches the work items
// assigned to the current user that are not done.
func JiraSearchOption() tool.RunOption {
	return tool.RunOption{
		Cmd: "jira",
		Args: []string{
			"workitem",
			"search",
			"--jql",
			myTasksJQL,
			"--fields",
			"key,summary",
			"--paginate",
			"--json",
		},
	}
}

// JiraViewOption returns the CLI option that fetches the fields of the work
// item identified by key.
func JiraViewOption(key string) tool.RunOption {
	return tool.RunOption{
		Cmd: "jira",
		Args: []string{
			"workitem",
			"view",
			key,
			"--fields",
			viewFields,
			"--json",
		},
	}
}

// TODO: Convert ADF to Markdown
//   - Render date nodes from attrs.timestamp (epoch milliseconds); they are
//     dropped today.
//   - Render status and emoji nodes from attrs.text; they are dropped today.
//   - Keep list and task markers (`- `, `1. `, `[x]`/`[ ]` from taskItem
//     attrs.state) so items stay distinguishable.

// adfNode is a node of the Atlassian Document Format that Jira uses for rich
// text fields such as description.
type adfNode struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Attrs struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"attrs"`
	Content []adfNode `json:"content"`
}

// plainText flattens the document into text, putting each block on its own line.
func (n *adfNode) plainText() string {
	if n == nil {
		return ""
	}

	var b strings.Builder
	n.writeText(&b)

	return strings.TrimSpace(b.String())
}

func (n *adfNode) writeText(b *strings.Builder) {
	switch n.Type {
	case "text":
		b.WriteString(n.Text)
	case "hardBreak":
		b.WriteString("\n")
	case "mention":
		b.WriteString(n.Attrs.Text)
	case "inlineCard":
		b.WriteString(n.Attrs.URL)
	}

	for i := range n.Content {
		n.Content[i].writeText(b)
	}

	switch n.Type {
	case "paragraph", "heading", "codeBlock", "taskItem":
		b.WriteString("\n")
	}
}
