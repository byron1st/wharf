package atlassian_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/pkg/task"
)

func viewWorkItem(t *testing.T, raw string) atlassian.ViewWorkItem {
	t.Helper()

	var item atlassian.ViewWorkItem
	require.NoError(t, json.Unmarshal([]byte(raw), &item))

	return item
}

// descriptionItem wraps an ADF document in a To Do work item.
func descriptionItem(t *testing.T, description string) atlassian.ViewWorkItem {
	t.Helper()

	return viewWorkItem(t, `{
		"key": "WHF-1",
		"fields": {
			"summary": "Write onboarding guide",
			"status": {"name": "To Do", "statusCategory": {"key": "new"}},
			"description": `+description+`
		}
	}`)
}

func TestToStatus(t *testing.T) {
	tests := []struct {
		name        string
		statusName  string
		categoryKey atlassian.StatusCategoryKey
		want        task.Status
	}{
		{name: "new is todo", statusName: "To Do", categoryKey: "new", want: task.StatusTodo},
		{
			name:        "indeterminate is in progress",
			statusName:  "In Progress",
			categoryKey: "indeterminate",
			want:        task.StatusInProgress,
		},
		{name: "done is done", statusName: "Done", categoryKey: "done", want: task.StatusDone},
		{
			name:        "custom status follows category",
			statusName:  "In Review",
			categoryKey: "indeterminate",
			want:        task.StatusInProgress,
		},
		{name: "canceled is matched by name", statusName: "Canceled", categoryKey: "done", want: task.StatusCanceled},
		{
			name:        "canceled name wins over category",
			statusName:  "Canceled",
			categoryKey: "undefined",
			want:        task.StatusCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := atlassian.ToStatus(tt.statusName, tt.categoryKey)
			require.NoError(t, err)
			require.Equal(t, tt.want, status)
		})
	}
}

func TestToStatusUnknownCategory(t *testing.T) {
	tests := []struct {
		name        string
		statusName  string
		categoryKey atlassian.StatusCategoryKey
	}{
		{name: "undefined category", statusName: "Triage", categoryKey: "undefined"},
		{name: "empty category", statusName: "", categoryKey: ""},
		{name: "canceled name is case sensitive", statusName: "canceled", categoryKey: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := atlassian.ToStatus(tt.statusName, tt.categoryKey)
			require.ErrorIs(t, err, atlassian.ErrUnknownStatusCategory)
			require.ErrorContains(t, err, "status="+tt.statusName+", category="+string(tt.categoryKey))
			require.Empty(t, status)
		})
	}
}

func TestToTask(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_done.json")))
	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:          "jira-WHF-3",
		Originated:  task.OriginJira,
		Title:       "Fix flaky login test",
		Description: "Retry is no longer needed.",
		Status:      task.StatusDone,
		StartDate:   date(t, "2026-09-01"),
		DueDate:     date(t, "2026-09-05"),
		ClosedAt:    new(time.Date(2026, 9, 4, 9, 30, 12, 345e6, time.UTC)),
	}, got)
	require.Equal(t, time.UTC, got.StartDate.Location())
	require.Equal(t, time.UTC, got.ClosedAt.Location())
}

func TestToTaskClosedAt(t *testing.T) {
	tests := []struct {
		name   string
		status string
		change string
		want   *time.Time
	}{
		{
			name:   "done takes the status category change date",
			status: `{"name": "Done", "statusCategory": {"key": "done"}}`,
			change: `"2026-10-23T14:05:09.123+0900"`,
			want:   new(time.Date(2026, 10, 23, 5, 5, 9, 123e6, time.UTC)),
		},
		{
			name:   "canceled takes the status category change date",
			status: `{"name": "Canceled", "statusCategory": {"key": "done"}}`,
			change: `"2026-10-23T14:05:09.000-0300"`,
			want:   new(time.Date(2026, 10, 23, 17, 5, 9, 0, time.UTC)),
		},
		{
			name:   "the fraction is optional",
			status: `{"name": "Done", "statusCategory": {"key": "done"}}`,
			change: `"2026-10-23T14:05:09+0900"`,
			want:   new(time.Date(2026, 10, 23, 5, 5, 9, 0, time.UTC)),
		},
		{
			name:   "RFC 3339 is accepted",
			status: `{"name": "Done", "statusCategory": {"key": "done"}}`,
			change: `"2026-10-23T14:05:09.5+09:00"`,
			want:   new(time.Date(2026, 10, 23, 5, 5, 9, 5e8, time.UTC)),
		},
		{
			name:   "an open status has none",
			status: `{"name": "In Progress", "statusCategory": {"key": "indeterminate"}}`,
			change: `"2026-10-23T14:05:09.123+0900"`,
		},
		{
			name:   "a closed status without the date has none",
			status: `{"name": "Done", "statusCategory": {"key": "done"}}`,
			change: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := atlassian.ToTask(viewWorkItem(t, `{
				"key": "WHF-1",
				"fields": {"summary": "s", "status": `+tt.status+`, "statuscategorychangedate": `+tt.change+`}
			}`))
			require.NoError(t, err)
			require.Equal(t, tt.want, got.ClosedAt)
		})
	}
}

func TestToTaskInvalidClosedAt(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, `{
		"key": "WHF-1",
		"fields": {
			"summary": "s",
			"status": {"name": "Done", "statusCategory": {"key": "done"}},
			"statuscategorychangedate": "2026-10-23"
		}
	}`))
	require.ErrorIs(t, err, atlassian.ErrFailedToParseClosedAt)
	require.ErrorContains(t, err, "key=WHF-1")
	require.Equal(t, task.Status(task.StatusDone), got.Status)
	require.Nil(t, got.ClosedAt)

	var parseErr *time.ParseError
	require.ErrorAs(t, err, &parseErr)
}

func TestToTaskWithoutOptionalFields(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_todo.json")))
	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:         "jira-WHF-1",
		Originated: task.OriginJira,
		Title:      "Write onboarding guide",
		Status:     task.StatusTodo,
	}, got)
}

func TestToTaskRelations(t *testing.T) {
	tests := []struct {
		name            string
		fields          string
		wantParentID    string
		wantChildrenIDs []string
		wantLinkedIDs   []string
	}{
		{name: "no relations", fields: `"parent": null, "subtasks": [], "issuelinks": []`},
		{
			name:         "parent key",
			fields:       `"parent": {"id": "10100", "key": "WHF-10", "fields": {"summary": "Task sync"}}`,
			wantParentID: "jira-WHF-10",
		},
		{name: "parent without a key is skipped", fields: `"parent": {"id": "10100"}`},
		{
			name: "subtask keys keep their order",
			fields: `"subtasks": [
				{"id": "10106", "key": "WHF-6", "fields": {"summary": "Store synced tasks"}},
				{"id": "10105", "key": "WHF-5", "fields": {"summary": "Add sync scheduler"}}
			]`,
			wantChildrenIDs: []string{"jira-WHF-6", "jira-WHF-5"},
		},
		{
			name:            "subtasks without a key are skipped",
			fields:          `"subtasks": [{"id": "10105"}, {"id": "10106", "key": "WHF-6"}]`,
			wantChildrenIDs: []string{"jira-WHF-6"},
		},
		{
			name: "links in both directions keep their order",
			fields: `"issuelinks": [
				{"id": "1", "type": {"name": "Blocks"}, "outwardIssue": {"key": "WHF-3"}},
				{"id": "2", "type": {"name": "Relates"}, "inwardIssue": {"key": "WHF-2"}}
			]`,
			wantLinkedIDs: []string{"jira-WHF-3", "jira-WHF-2"},
		},
		{
			name: "links without a key are skipped",
			fields: `"issuelinks": [
				{"id": "1", "type": {"name": "Relates"}},
				{"id": "2", "type": {"name": "Relates"}, "inwardIssue": {"key": ""}},
				{"id": "3", "type": {"name": "Relates"}, "inwardIssue": {"key": "WHF-2"}}
			]`,
			wantLinkedIDs: []string{"jira-WHF-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := atlassian.ToTask(viewWorkItem(t, `{
				"key": "WHF-1",
				"fields": {
					"summary": "Write onboarding guide",
					"status": {"name": "To Do", "statusCategory": {"key": "new"}},
					`+tt.fields+`
				}
			}`))
			require.NoError(t, err)
			require.Equal(t, tt.wantParentID, got.ParentID)
			require.Equal(t, tt.wantChildrenIDs, got.ChildrenIDs)
			require.Equal(t, tt.wantLinkedIDs, got.LinkedIDs)
		})
	}
}

func TestToTaskInvalidFields(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_invalid_fields.json")))
	require.Equal(t, task.Task{
		ID:         "jira-WHF-1",
		Originated: task.OriginJira,
		Title:      "Write onboarding guide",
	}, got)
	require.ErrorIs(t, err, atlassian.ErrUnknownStatusCategory)
	require.ErrorIs(t, err, atlassian.ErrFailedToParseStartDate)
	require.ErrorIs(t, err, atlassian.ErrFailedToParseDueDate)
	require.ErrorContains(t, err, "key=WHF-1")

	var parseErr *time.ParseError
	require.ErrorAs(t, err, &parseErr)
}

func TestToTaskDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "null description", description: `null`, want: ""},
		{name: "empty document", description: `{"type": "doc", "version": 1, "content": []}`, want: ""},
		{
			name: "heading levels are clamped to 1-6",
			description: `{"type": "doc", "content": [
				{"type": "heading", "attrs": {"level": 3}, "content": [{"type": "text", "text": "Three"}]},
				{"type": "heading", "attrs": {"level": 9}, "content": [{"type": "text", "text": "Nine"}]},
				{"type": "heading", "content": [{"type": "text", "text": "None"}]}
			]}`,
			want: "### Three\n\n###### Nine\n\n# None",
		},
		{
			name: "marks become delimiters around trimmed text",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "a "},
				{"type": "text", "text": "bold ", "marks": [{"type": "strong"}]},
				{"type": "text", "text": "it", "marks": [{"type": "em"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "gone", "marks": [{"type": "strike"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "x", "marks": [{"type": "code"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "docs", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
			]}]}`,
			want: "a **bold** *it* ~~gone~~ `x` [docs](https://example.com)",
		},
		{
			name: "overlapping marks nest link, strike, strong, em, code",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "all", "marks": [
					{"type": "code"}, {"type": "em"}, {"type": "strong"}, {"type": "strike"},
					{"type": "link", "attrs": {"href": "https://example.com"}}
				]}
			]}]}`,
			want: "[~~***`all`***~~](https://example.com)",
		},
		{
			name: "adjacent text with the same marks is merged",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "a", "marks": [{"type": "strong"}, {"type": "em"}]},
				{"type": "text", "text": "b", "marks": [{"type": "em"}, {"type": "strong"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "c", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]},
				{"type": "text", "text": "d", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
			]}]}`,
			want: "***ab*** [cd](https://example.com)",
		},
		{
			name: "adjacent text with different marks stays separate",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "a", "marks": [{"type": "strong"}]},
				{"type": "text", "text": "b", "marks": [{"type": "strong"}, {"type": "em"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "c", "marks": [{"type": "link", "attrs": {"href": "https://a.example"}}]},
				{"type": "text", "text": "d", "marks": [{"type": "link", "attrs": {"href": "https://b.example"}}]}
			]}]}`,
			want: "**a*****b*** [c](https://a.example)[d](https://b.example)",
		},
		{
			name: "code mark with backticks uses a longer delimiter",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "a` + "`" + `b", "marks": [{"type": "code"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "` + "`" + `x", "marks": [{"type": "code"}]}
			]}]}`,
			want: "``a`b`` `` `x ``",
		},
		{
			name: "marks without a Markdown equivalent keep the text",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "under", "marks": [{"type": "underline"}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "red", "marks": [{"type": "textColor", "attrs": {"color": "#ff0000"}}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "2", "marks": [{"type": "subsup", "attrs": {"type": "sup"}}]},
				{"type": "text", "text": " "},
				{"type": "text", "text": "nolink", "marks": [{"type": "link", "attrs": {}}]}
			]}]}`,
			want: "under red 2 nolink",
		},
		{
			name: "hard break becomes a backslash line break",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "one"}, {"type": "hardBreak"}, {"type": "text", "text": "two"}, {"type": "hardBreak"}
			]}]}`,
			want: "one\\\ntwo",
		},
		{
			name: "bullet and ordered lists nest with indentation",
			description: `{"type": "doc", "content": [{"type": "bulletList", "content": [
				{"type": "listItem", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "parent"}]},
					{"type": "orderedList", "attrs": {"order": 9}, "content": [
						{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "nine"}]}]},
						{"type": "listItem", "content": [
							{"type": "paragraph", "content": [{"type": "text", "text": "ten"}]},
							{"type": "paragraph", "content": [{"type": "text", "text": "more"}]}
						]}
					]}
				]},
				{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "second"}]}]}
			]}]}`,
			want: "- parent\n\n  9. nine\n  10. ten\n\n      more\n- second",
		},
		{
			name: "ordered list starts at one without order",
			description: `{"type": "doc", "content": [{"type": "orderedList", "content": [
				{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]},
				{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}]}
			]}]}`,
			want: "1. a\n2. b",
		},
		{
			name: "list item blocks align under the item content",
			description: `{"type": "doc", "content": [
				{"type": "bulletList", "content": [{"type": "listItem", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "a"}]},
					{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}
				]}]},
				{"type": "orderedList", "attrs": {"order": 3}, "content": [{"type": "listItem", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "c"}]},
					{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "x := 1"}]}
				]}]}
			]}`,
			want: "- a\n\n  b\n\n3. c\n\n   ```go\n   x := 1\n   ```",
		},
		{
			name: "task items get checkboxes and nested task lists indent",
			description: `{"type": "doc", "content": [{"type": "taskList", "content": [
				{"type": "taskItem", "attrs": {"state": "DONE"}, "content": [{"type": "text", "text": "first"}]},
				{"type": "taskList", "content": [
					{"type": "taskItem", "attrs": {"state": "TODO"}, "content": [{"type": "text", "text": "sub"}]}
				]},
				{"type": "taskItem", "attrs": {"state": "TODO"}, "content": [{"type": "text", "text": "second"}]}
			]}]}`,
			want: "- [x] first\n  - [ ] sub\n- [ ] second",
		},
		{
			name: "adjacent bullet-style lists alternate markers",
			description: `{"type": "doc", "content": [
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}]},
				{"type": "taskList", "content": [{"type": "taskItem", "attrs": {"state": "DONE"}, "content": [{"type": "text", "text": "b"}]}]},
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "c"}]}]}]}
			]}`,
			want: "- a\n\n* [x] b\n\n- c",
		},
		{
			name: "adjacent ordered lists alternate delimiters",
			description: `{"type": "doc", "content": [
				{"type": "orderedList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}]},
				{"type": "orderedList", "attrs": {"order": 5}, "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}]}]}
			]}`,
			want: "1. a\n\n5) b",
		},
		{
			name: "lists separated by another block keep default markers",
			description: `{"type": "doc", "content": [
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}]},
				{"type": "paragraph", "content": [{"type": "text", "text": "between"}]},
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}]}]}
			]}`,
			want: "- a\n\nbetween\n\n- b",
		},
		{
			name: "adjacent lists inside a blockquote alternate markers",
			description: `{"type": "doc", "content": [{"type": "blockquote", "content": [
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}]},
				{"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}]}]}
			]}]}`,
			want: "> - a\n>\n> * b",
		},
		{
			name: "decision items are bullets",
			description: `{"type": "doc", "content": [{"type": "decisionList", "content": [
				{"type": "decisionItem", "attrs": {"state": "DECIDED"}, "content": [{"type": "text", "text": "ship it"}]}
			]}]}`,
			want: "- ship it",
		},
		{
			name: "code block is fenced with its language and a longer fence for backticks",
			description: `{"type": "doc", "content": [
				{"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "x := 1\n"}]},
				{"type": "codeBlock", "content": [{"type": "text", "text": "` + "```" + `md\n` + "```" + `"}]},
				{"type": "codeBlock", "content": []}
			]}`,
			want: "```go\nx := 1\n```\n\n````\n```md\n```\n````",
		},
		{
			name: "blockquote and panel quote every line",
			description: `{"type": "doc", "content": [
				{"type": "blockquote", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "one"}]},
					{"type": "paragraph", "content": [{"type": "text", "text": "two"}]}
				]},
				{"type": "panel", "attrs": {"panelType": "info"}, "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "note"}]}
				]}
			]}`,
			want: "> one\n>\n> two\n\n> note",
		},
		{
			name: "rule is a thematic break",
			description: `{"type": "doc", "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "a"}]},
				{"type": "rule"},
				{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}
			]}`,
			want: "a\n\n---\n\nb",
		},
		{
			name: "table becomes a GFM table with escaped pipes and padded rows",
			description: `{"type": "doc", "content": [{"type": "table", "content": [
				{"type": "tableRow", "content": [
					{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Name"}]}]},
					{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Value"}]}]}
				]},
				{"type": "tableRow", "content": [
					{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a|b"}]}]},
					{"type": "tableCell", "content": [
						{"type": "paragraph", "content": [{"type": "text", "text": "x"}, {"type": "hardBreak"}, {"type": "text", "text": "y"}]},
						{"type": "paragraph", "content": [{"type": "text", "text": "z"}]}
					]}
				]},
				{"type": "tableRow", "content": [
					{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "only"}]}]}
				]}
			]}]}`,
			want: "| Name | Value |\n| --- | --- |\n| a\\|b | x<br>y<br>z |\n| only |  |",
		},
		{
			name: "table drops rows without cells",
			description: `{"type": "doc", "content": [{"type": "table", "content": [
				{"type": "tableRow", "content": []},
				{"type": "tableRow", "content": [
					{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Name"}]}]}
				]},
				{"type": "tableRow"},
				{"type": "tableRow", "content": [
					{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}
				]}
			]}]}`,
			want: "| Name |\n| --- |\n| a |",
		},
		{
			name: "table with only cellless rows renders nothing between paragraphs",
			description: `{"type": "doc", "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "before"}]},
				{"type": "table", "content": [{"type": "tableRow", "content": []}, {"type": "tableRow"}]},
				{"type": "paragraph", "content": [{"type": "text", "text": "after"}]}
			]}`,
			want: "before\n\nafter",
		},
		{
			name: "expand title becomes a bold line",
			description: `{"type": "doc", "content": [
				{"type": "expand", "attrs": {"title": "Details"}, "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "hidden"}]}
				]},
				{"type": "nestedExpand", "attrs": {}, "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "untitled"}]}
				]}
			]}`,
			want: "**Details**\n\nhidden\n\nuntitled",
		},
		{
			name: "inline nodes use their attrs",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "mention", "attrs": {"id": "712020:x", "text": "@Jane Doe"}},
				{"type": "text", "text": " "},
				{"type": "emoji", "attrs": {"shortName": ":smile:", "text": "😄"}},
				{"type": "emoji", "attrs": {"shortName": ":custom:"}},
				{"type": "text", "text": " "},
				{"type": "status", "attrs": {"text": "IN REVIEW", "color": "blue"}},
				{"type": "text", "text": " "},
				{"type": "date", "attrs": {"timestamp": "1761436800000"}},
				{"type": "date", "attrs": {"timestamp": "soon"}},
				{"type": "text", "text": "/"},
				{"type": "date", "attrs": {"timestamp": 1761436800000}},
				{"type": "text", "text": " "},
				{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/WHF-2"}}
			]}]}`,
			want: "@Jane Doe 😄:custom: \\[IN REVIEW\\] 2025-10-26/2025-10-26 <https://example.atlassian.net/browse/WHF-2>",
		},
		{
			name: "block and embed cards are autolink blocks",
			description: `{"type": "doc", "content": [
				{"type": "blockCard", "attrs": {"url": "https://example.com/a"}},
				{"type": "embedCard", "attrs": {"url": "https://example.com/b", "layout": "wide"}},
				{"type": "blockCard", "attrs": {}}
			]}`,
			want: "<https://example.com/a>\n\n<https://example.com/b>",
		},
		{
			name: "unknown nodes keep their text",
			description: `{"type": "doc", "content": [
				{"type": "layoutSection", "content": [{"type": "layoutColumn", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "left"}]}
				]}]},
				{"type": "paragraph", "content": [
					{"type": "inlineExtension", "content": [{"type": "text", "text": "inline"}]}
				]}
			]}`,
			want: "left\n\ninline",
		},
		{
			name: "media and empty blocks add nothing",
			description: `{"type": "doc", "content": [
				{"type": "paragraph"},
				{"type": "paragraph", "content": [{"type": "text", "text": "before"}]},
				{"type": "mediaSingle", "content": [{"type": "media", "attrs": {"id": "1", "type": "file", "alt": "a.png"}}]},
				{"type": "mediaGroup", "content": [{"type": "media", "attrs": {"id": "2", "type": "file"}}]},
				{"type": "heading", "attrs": {"level": 2}},
				{"type": "bulletList", "content": []},
				{"type": "paragraph", "content": [{"type": "text", "text": "  "}]},
				{"type": "paragraph", "content": [{"type": "text", "text": "after"}]},
				{"type": "paragraph"}
			]}`,
			want: "before\n\nafter",
		},
		{
			name: "unsafe link and card urls are not linked",
			description: `{"type": "doc", "content": [
				{"type": "paragraph", "content": [
					{"type": "text", "text": "click", "marks": [{"type": "link", "attrs": {"href": "javascript:alert(1)"}}]},
					{"type": "text", "text": " "},
					{"type": "inlineCard", "attrs": {"url": "javascript:alert(2)"}},
					{"type": "text", "text": " "},
					{"type": "text", "text": "mail", "marks": [{"type": "link", "attrs": {"href": "mailto:a@example.com"}}]},
					{"type": "text", "text": " "},
					{"type": "text", "text": "wiki", "marks": [{"type": "link", "attrs": {"href": "https://example.com/a (b)"}}]}
				]},
				{"type": "blockCard", "attrs": {"url": "data:text/html,x"}}
			]}`,
			want: "click `javascript:alert(2)` [mail](mailto:a@example.com) [wiki](<https://example.com/a (b)>)\n\n`data:text/html,x`",
		},
		{
			name: "less-than is escaped outside code",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "List<String> <img src=x onerror=alert(3)> "},
				{"type": "text", "text": "<b>", "marks": [{"type": "code"}]},
				{"type": "mention", "attrs": {"text": "<@x>"}}
			]}]}`,
			want: "List\\<String> \\<img src=x onerror=alert(3)> `<b>`\\<@x>",
		},
		{
			name: "link syntax typed into text is escaped",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "see [x](javascript:alert(1)) and \\[y](javascript:alert(2)) "},
				{"type": "status", "attrs": {"text": "[z](javascript:alert(3))"}},
				{"type": "text", "text": " "},
				{"type": "mention", "attrs": {"text": "[m](javascript:alert(4))"}}
			]}]}`,
			want: `see \[x\](javascript:alert(1)) and \\\[y\](javascript:alert(2)) \[\[z\](javascript:alert(3))\] \[m\](javascript:alert(4))`,
		},
		{
			name: "backslash before less-than cannot undo the escape",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "\\<img src=x onerror=alert(3)> "},
				{"type": "mention", "attrs": {"text": "\\<b>"}}
			]}]}`,
			want: `\\\<img src=x onerror=alert(3)> \\\<b>`,
		},
		{
			name: "line endings in code spans become spaces",
			description: `{"type": "doc", "content": [
				{"type": "paragraph", "content": [
					{"type": "text", "text": "x\n\n<img src=x onerror=alert(4)>", "marks": [{"type": "code"}]},
					{"type": "text", "text": " "},
					{"type": "inlineCard", "attrs": {"url": "x\r\n<script>alert(1)</script>"}}
				]},
				{"type": "blockCard", "attrs": {"url": "a\r\r<img src=x onerror=alert(1)>"}}
			]}`,
			want: "`x  <img src=x onerror=alert(4)>` `x <script>alert(1)</script>`\n\n`a  <img src=x onerror=alert(1)>`",
		},
		{
			name: "em and strong share one delimiter run while strike cannot flank there",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "이건"},
				{"type": "text", "text": "중요", "marks": [{"type": "em"}, {"type": "strong"}, {"type": "strike"}]},
				{"type": "text", "text": "해요"}
			]}]}`,
			want: "이건***중요***해요",
		},
		{
			name: "ordered list start is clamped to the CommonMark range",
			description: `{"type": "doc", "content": [
				{"type": "orderedList", "attrs": {"order": -5}, "content": [
					{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}
				]},
				{"type": "paragraph", "content": [{"type": "text", "text": "gap"}]},
				{"type": "orderedList", "attrs": {"order": 2000000000}, "content": [
					{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "b"}]}]}
				]}
			]}`,
			want: "0. a\n\ngap\n\n999999999. b",
		},
		{
			name: "status brackets are escaped so they never form a link",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "status", "attrs": {"text": "DONE"}},
				{"type": "text", "text": "(2026-10-01)"}
			]}]}`,
			want: "\\[DONE\\](2026-10-01)",
		},
		{
			name: "emphasis that cannot open or close is dropped",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "API(v2)", "marks": [{"type": "strong"}]},
				{"type": "text", "text": "는 중요하고 "},
				{"type": "text", "text": "굵게", "marks": [{"type": "strong"}]},
				{"type": "text", "text": "는 유지, 앞"},
				{"type": "text", "text": "(v3)", "marks": [{"type": "strike"}]},
				{"type": "text", "text": " 그리고 "},
				{"type": "text", "text": "(링크)", "marks": [{"type": "strong"}, {"type": "link", "attrs": {"href": "https://example.com"}}]},
				{"type": "text", "text": "도"}
			]}]}`,
			want: "API(v2)는 중요하고 **굵게**는 유지, 앞(v3) 그리고 [**(링크)**](https://example.com)도",
		},
		{
			name: "non-integer level and order fall back or round-trip",
			description: `{"type": "doc", "content": [
				{"type": "heading", "attrs": {"level": 2.0}, "content": [{"type": "text", "text": "float"}]},
				{"type": "heading", "attrs": {"level": "3"}, "content": [{"type": "text", "text": "string"}]},
				{"type": "heading", "attrs": {"level": 2.5}, "content": [{"type": "text", "text": "fraction"}]},
				{"type": "orderedList", "attrs": {"order": "x"}, "content": [
					{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "a"}]}]}
				]}
			]}`,
			want: "## float\n\n### string\n\n# fraction\n\n1. a",
		},
		{
			name: "code block language keeps only safe characters",
			description: `{"type": "doc", "content": [{"type": "codeBlock", "attrs": {"language": "go\n<script>` + "`" + `"},
				"content": [{"type": "text", "text": "x"}]}]}`,
			want: "```goscript\nx\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := atlassian.ToTask(descriptionItem(t, tt.description))
			require.NoError(t, err)
			require.Equal(t, tt.want, got.Description)
		})
	}
}
