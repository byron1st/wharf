package atlassian_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/tool"
)

var searchOption = tool.RunOption{
	Cmd: "jira",
	Args: []string{
		"workitem",
		"search",
		"--jql",
		"assignee = currentUser()",
		"--fields",
		"key,summary",
		"--paginate",
		"--json",
	},
}

func viewOption(key string) tool.RunOption {
	return tool.RunOption{
		Cmd: "jira",
		Args: []string{
			"workitem",
			"view",
			key,
			"--fields",
			"summary,description,status,duedate,customfield_10015",
			"--json",
		},
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(b)
}

func date(t *testing.T, value string) *time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, value)
	require.NoError(t, err)

	return &d
}

func TestGetMyTasks(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return(fixture(t, "view_todo.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-2")).Return(fixture(t, "view_in_progress.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-3")).Return(fixture(t, "view_done.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-4")).Return(fixture(t, "view_canceled.json"), nil).Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, []task.Task{
		{
			ID:         "WHF-1",
			Originated: task.OriginJira,
			Title:      "Write onboarding guide",
			Status:     task.StatusTodo,
		},
		{
			ID:         "WHF-2",
			Originated: task.OriginJira,
			Title:      "Implement Jira sync",
			Description: "Goal\n" +
				"Sync tasks from acli every 10 minutes.\n" +
				"Search assigned work items\n" +
				"View each work item\n" +
				"Parse ADF description\n" +
				"Map statuses\n" +
				"Owner: @Jane Doe see https://example.atlassian.net/browse/WHF-1\n" +
				"line one\nline two\n" +
				"acli jira workitem search \\\n  --json",
			Status:    task.StatusInProgress,
			StartDate: date(t, "2026-10-01"),
			DueDate:   date(t, "2026-10-31"),
		},
		{
			ID:          "WHF-3",
			Originated:  task.OriginJira,
			Title:       "Fix flaky login test",
			Description: "Retry is no longer needed.",
			Status:      task.StatusDone,
			StartDate:   date(t, "2026-09-01"),
			DueDate:     date(t, "2026-09-05"),
		},
		{
			ID:          "WHF-4",
			Originated:  task.OriginJira,
			Title:       "Migrate legacy cron jobs",
			Description: "Superseded by WHF-2.",
			Status:      task.StatusCanceled,
		},
	}, tasks)
}

func TestGetMyTasksNoWorkItems(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search_empty.json"), nil).Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
	require.NoError(t, err)
	require.Empty(t, tasks)
}

func TestGetMyTasksSearchErrors(t *testing.T) {
	acliErr := errors.Join(
		fmt.Errorf("%w: stderr=✗ Error: failed to search work items", tool.ErrFailedToExecuteCmd),
		errors.New("exit status 1"),
	)

	tests := []struct {
		name    string
		out     string
		runErr  error
		wantErr error
	}{
		{name: "acli fails", runErr: acliErr, wantErr: tool.ErrFailedToExecuteCmd},
		{
			name:    "output is not json",
			out:     "✓ Number of work items in the search: 4\n",
			wantErr: atlassian.ErrFailedToParseSearchOutput,
		},
		{name: "work item is null", out: fixture(t, "search_null_item.json"), wantErr: atlassian.ErrMissingWorkItemKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockTool(t)
			m.EXPECT().Run(mock.Anything, searchOption).Return(tt.out, tt.runErr).Once()

			tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, tasks)
		})
	}
}

func TestGetMyTasksViewErrors(t *testing.T) {
	acliErr := errors.Join(
		fmt.Errorf(
			"%w: stderr=✗ Error: Issue does not exist or you do not have permission to see it.",
			tool.ErrFailedToExecuteCmd,
		),
		errors.New("exit status 1"),
	)

	tests := []struct {
		name    string
		out     string
		runErr  error
		wantErr error
	}{
		{name: "acli fails", runErr: acliErr, wantErr: tool.ErrFailedToExecuteCmd},
		{name: "output is not json", out: "Key: WHF-1\n", wantErr: atlassian.ErrFailedToParseViewOutput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockTool(t)
			m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search_single.json"), nil).Once()
			m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return(tt.out, tt.runErr).Once()

			tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			require.ErrorContains(t, err, "key=WHF-1")
			require.Nil(t, tasks)
		})
	}
}

func TestGetMyTasksCollectsFieldErrors(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		want     task.Task
		wantErrs []error
	}{
		{
			name:     "status category is unknown",
			out:      fixture(t, "view_unknown_status.json"),
			want:     task.Task{ID: "WHF-1", Originated: task.OriginJira, Title: "Write onboarding guide"},
			wantErrs: []error{atlassian.ErrUnknownStatusCategory},
		},
		{
			name: "start date is invalid",
			out:  fixture(t, "view_invalid_start_date.json"),
			want: task.Task{
				ID:         "WHF-1",
				Originated: task.OriginJira,
				Title:      "Write onboarding guide",
				Status:     task.StatusTodo,
			},
			wantErrs: []error{atlassian.ErrFailedToParseStartDate},
		},
		{
			name: "due date is invalid",
			out:  fixture(t, "view_invalid_due_date.json"),
			want: task.Task{
				ID:         "WHF-1",
				Originated: task.OriginJira,
				Title:      "Write onboarding guide",
				Status:     task.StatusTodo,
			},
			wantErrs: []error{atlassian.ErrFailedToParseDueDate},
		},
		{
			name: "every field is invalid",
			out:  fixture(t, "view_invalid_fields.json"),
			want: task.Task{ID: "WHF-1", Originated: task.OriginJira, Title: "Write onboarding guide"},
			wantErrs: []error{
				atlassian.ErrUnknownStatusCategory,
				atlassian.ErrFailedToParseStartDate,
				atlassian.ErrFailedToParseDueDate,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockTool(t)
			m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search_single.json"), nil).Once()
			m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return(tt.out, nil).Once()

			tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
			require.Equal(t, []task.Task{tt.want}, tasks)
			require.ErrorContains(t, err, "key=WHF-1")

			for _, wantErr := range tt.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
		})
	}
}

func TestGetMyTasksCollectsErrorsAcrossWorkItems(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search_with_null_item.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return(fixture(t, "view_unknown_status.json"), nil).Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
	require.Equal(t, []task.Task{
		{ID: "WHF-1", Originated: task.OriginJira, Title: "Write onboarding guide"},
	}, tasks)
	require.ErrorIs(t, err, atlassian.ErrMissingWorkItemKey)
	require.ErrorContains(t, err, "index=0")
	require.ErrorIs(t, err, atlassian.ErrUnknownStatusCategory)
}

func TestGetMyTasksKeepsSearchErrorsWhenViewFails(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search_with_null_item.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return("Key: WHF-1\n", nil).Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
	require.ErrorIs(t, err, atlassian.ErrFailedToParseViewOutput)
	require.ErrorIs(t, err, atlassian.ErrMissingWorkItemKey)
	require.Nil(t, tasks)
}

func TestGetMyTasksStopsViewingWhenContextCanceled(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search.json"), nil).Once()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	tasks, err := atlassian.NewClient(m).GetMyTasks(ctx)
	require.ErrorIs(t, err, atlassian.ErrGetMyTasksCanceled)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tasks)
}

func TestGetMyTasksStopsWhenContextCanceledBetweenViews(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search.json"), nil).Once()
	m.EXPECT().
		Run(mock.Anything, viewOption("WHF-1")).
		RunAndReturn(func(context.Context, tool.RunOption) (string, error) {
			cancel()

			return fixture(t, "view_todo.json"), nil
		}).
		Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(ctx)
	require.ErrorIs(t, err, atlassian.ErrGetMyTasksCanceled)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tasks)
}

func TestGetMyTasksDiscardsConvertedTasksWhenLaterViewFails(t *testing.T) {
	m := mocks.NewMockTool(t)
	m.EXPECT().Run(mock.Anything, searchOption).Return(fixture(t, "search.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-1")).Return(fixture(t, "view_unknown_status.json"), nil).Once()
	m.EXPECT().Run(mock.Anything, viewOption("WHF-2")).Return("", tool.ErrFailedToExecuteCmd).Once()

	tasks, err := atlassian.NewClient(m).GetMyTasks(t.Context())
	require.ErrorIs(t, err, tool.ErrFailedToExecuteCmd)
	require.ErrorContains(t, err, "key=WHF-2")
	require.Nil(t, tasks)
}
