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
		categoryKey string
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
		categoryKey string
	}{
		{name: "undefined category", statusName: "Triage", categoryKey: "undefined"},
		{name: "empty category", statusName: "", categoryKey: ""},
		{name: "canceled name is case sensitive", statusName: "canceled", categoryKey: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := atlassian.ToStatus(tt.statusName, tt.categoryKey)
			require.ErrorIs(t, err, atlassian.ErrUnknownStatusCategory)
			require.ErrorContains(t, err, "status="+tt.statusName+", category="+tt.categoryKey)
			require.Empty(t, status)
		})
	}
}

func TestToTask(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_done.json")))
	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:          "WHF-3",
		Originated:  task.OriginJira,
		Title:       "Fix flaky login test",
		Description: "Retry is no longer needed.",
		Status:      task.StatusDone,
		StartDate:   date(t, "2026-09-01"),
		DueDate:     date(t, "2026-09-05"),
	}, got)
	require.Equal(t, time.UTC, got.StartDate.Location())
}

func TestToTaskWithoutOptionalFields(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_todo.json")))
	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:         "WHF-1",
		Originated: task.OriginJira,
		Title:      "Write onboarding guide",
		Status:     task.StatusTodo,
	}, got)
}

func TestToTaskInvalidFields(t *testing.T) {
	got, err := atlassian.ToTask(viewWorkItem(t, fixture(t, "view_invalid_fields.json")))
	require.Equal(t, task.Task{
		ID:         "WHF-1",
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
			name: "marks are dropped",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "run "},
				{"type": "text", "text": "acli", "marks": [{"type": "code"}]},
				{"type": "text", "text": " now", "marks": [{"type": "strong"}]}
			]}]}`,
			want: "run acli now",
		},
		{
			name: "blocks are separated by newlines",
			description: `{"type": "doc", "content": [
				{"type": "heading", "attrs": {"level": 1}, "content": [{"type": "text", "text": "Title"}]},
				{"type": "paragraph", "content": [{"type": "text", "text": "Body"}]},
				{"type": "codeBlock", "content": [{"type": "text", "text": "go test"}]}
			]}`,
			want: "Title\nBody\ngo test",
		},
		{
			name: "nested lists flatten to one item per line",
			description: `{"type": "doc", "content": [{"type": "bulletList", "content": [
				{"type": "listItem", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "parent"}]},
					{"type": "orderedList", "content": [{"type": "listItem", "content": [
						{"type": "paragraph", "content": [{"type": "text", "text": "child"}]}
					]}]}
				]}
			]}]}`,
			want: "parent\nchild",
		},
		{
			name: "task items without paragraphs get their own lines",
			description: `{"type": "doc", "content": [{"type": "taskList", "content": [
				{"type": "taskItem", "attrs": {"state": "DONE"}, "content": [{"type": "text", "text": "first"}]},
				{"type": "taskItem", "attrs": {"state": "TODO"}, "content": [{"type": "text", "text": "second"}]}
			]}]}`,
			want: "first\nsecond",
		},
		{
			name: "hard break becomes newline",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "text", "text": "one"}, {"type": "hardBreak"}, {"type": "text", "text": "two"}
			]}]}`,
			want: "one\ntwo",
		},
		{
			name: "mention and inline card use attrs",
			description: `{"type": "doc", "content": [{"type": "paragraph", "content": [
				{"type": "mention", "attrs": {"id": "712020:x", "text": "@Jane Doe"}},
				{"type": "text", "text": " "},
				{"type": "inlineCard", "attrs": {"url": "https://example.atlassian.net/browse/WHF-2"}}
			]}]}`,
			want: "@Jane Doe https://example.atlassian.net/browse/WHF-2",
		},
		{
			name: "unknown containers keep their text",
			description: `{"type": "doc", "content": [{"type": "expand", "attrs": {"title": "Details"}, "content": [
				{"type": "table", "content": [{"type": "tableRow", "content": [{"type": "tableCell", "content": [
					{"type": "paragraph", "content": [{"type": "text", "text": "cell"}]}
				]}]}]}
			]}]}`,
			want: "cell",
		},
		{
			name: "media, rule and empty paragraphs add no text",
			description: `{"type": "doc", "content": [
				{"type": "paragraph", "content": [{"type": "text", "text": "before"}]},
				{"type": "rule"},
				{"type": "mediaSingle", "content": [{"type": "media", "attrs": {"id": "1", "type": "file"}}]},
				{"type": "paragraph"},
				{"type": "paragraph", "content": [{"type": "text", "text": "after"}]},
				{"type": "paragraph"}
			]}`,
			want: "before\n\nafter",
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

func TestJiraRunOptions(t *testing.T) {
	require.Equal(t, searchOption, atlassian.JiraSearchOption)
	require.Equal(t, viewOption("WHF-1"), atlassian.JiraWorkOption("WHF-1"))
}
