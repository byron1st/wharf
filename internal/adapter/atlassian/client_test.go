package atlassian_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/task"
)

const (
	siteURL  = "https://example.atlassian.net"
	email    = "me@example.com"
	apiToken = "secret-token"
)

// header is the request header for email:apiToken, with the Basic credential
// written out so a wrong encoding in the client cannot also fix the test.
var header = http.Header{
	"Authorization": {"Basic bWVAZXhhbXBsZS5jb206c2VjcmV0LXRva2Vu"},
	"Accept":        {"application/json"},
	"Content-Type":  {"application/json"},
}

// searchBody returns the exact search request body for the page token.
func searchBody(token string) []byte {
	body := `{"jql":"assignee = currentUser() AND statusCategory != done",` +
		`"fields":["summary","description","status","duedate","parent","subtasks","issuelinks","customfield_10015"],` +
		`"maxResults":100`
	if token != "" {
		body += `,"nextPageToken":"` + token + `"`
	}

	return []byte(body + "}")
}

func fixture(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(b)
}

// page returns a search response holding the raw issues. An empty next token
// is omitted, as Jira does on the last page.
func page(t *testing.T, next string, isLast bool, issues ...string) []byte {
	t.Helper()

	raw := make([]json.RawMessage, 0, len(issues))
	for _, issue := range issues {
		raw = append(raw, json.RawMessage(issue))
	}

	resp := map[string]any{"issues": raw, "isLast": isLast}
	if next != "" {
		resp["nextPageToken"] = next
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	return b
}

func date(t *testing.T, value string) *time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, value)
	require.NoError(t, err)

	return &d
}

func newClient(m httputil.HTTPClient) atlassian.Client {
	return atlassian.NewClient(m, siteURL, email, apiToken)
}

const searchURL = siteURL + "/rest/api/3/search/jql"

func TestGetMyTasks(t *testing.T) {
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
		Return(page(t, "p2", false, fixture(t, "view_todo.json"), fixture(t, "view_in_progress.json")), nil).Once()
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("p2")).
		Return(page(t, "p3", false, fixture(t, "view_done.json")), nil).Once()
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("p3")).
		Return(page(t, "", true, fixture(t, "view_canceled.json")), nil).Once()

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, []task.Task{
		{
			ID:         "jira-WHF-1",
			Originated: task.OriginJira,
			Title:      "Write onboarding guide",
			Status:     task.StatusTodo,
		},
		{
			ID:         "jira-WHF-2",
			Originated: task.OriginJira,
			Title:      "Implement Jira sync",
			Description: "## Goal\n\n" +
				"Sync tasks from `wharf` every **10 minutes**.\n\n" +
				"- Search assigned work items\n" +
				"- View each work item\n\n" +
				"* [x] Parse ADF description\n" +
				"* [ ] Map statuses\n\n" +
				"Owner: @Jane Doe see <https://example.atlassian.net/browse/WHF-1>\n\n" +
				"line one\\\nline two\n\n" +
				"```\nwharf jira workitem search \\\n  --json\n```\n\n" +
				"---",
			Status:      task.StatusInProgress,
			StartDate:   date(t, "2026-10-01"),
			DueDate:     date(t, "2026-10-31"),
			ParentID:    "jira-WHF-10",
			ChildrenIDs: []string{"jira-WHF-5", "jira-WHF-6"},
			LinkedIDs:   []string{"jira-WHF-1", "jira-WHF-3"},
		},
		{
			ID:          "jira-WHF-3",
			Originated:  task.OriginJira,
			Title:       "Fix flaky login test",
			Description: "Retry is no longer needed.",
			Status:      task.StatusDone,
			StartDate:   date(t, "2026-09-01"),
			DueDate:     date(t, "2026-09-05"),
		},
		{
			ID:          "jira-WHF-4",
			Originated:  task.OriginJira,
			Title:       "Migrate legacy cron jobs",
			Description: "Superseded by WHF-2.",
			Status:      task.StatusCanceled,
		},
	}, tasks)
}

func TestGetMyTasksStopsPaging(t *testing.T) {
	tests := []struct {
		name   string
		next   string
		isLast bool
	}{
		{name: "isLast is true even with a next token", next: "p2", isLast: true},
		{name: "next token is missing even when isLast is false", isLast: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
				Return(page(t, tt.next, tt.isLast, fixture(t, "view_todo.json")), nil).Once()

			tasks, err := newClient(m).GetMyTasks(t.Context())
			require.NoError(t, err)
			require.Len(t, tasks, 1)
		})
	}
}

func TestGetMyTasksRepeatedPageToken(t *testing.T) {
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
		Return(page(t, "p2", false, fixture(t, "view_todo.json")), nil).Once()
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("p2")).
		Return(page(t, "p2", false, fixture(t, "view_done.json")), nil).Once()

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.ErrorIs(t, err, atlassian.ErrRepeatedSearchPageToken)
	require.Nil(t, tasks)
}

func TestGetMyTasksPageLimit(t *testing.T) {
	// maxPages mirrors the client's page cap; the last allowed page still
	// returns a next token.
	const maxPages = 100

	m := mocks.NewMockHTTPClient(t)
	token := ""

	for i := 1; i <= maxPages; i++ {
		next := fmt.Sprintf("p%d", i+1)
		m.EXPECT().Post(t.Context(), searchURL, header, searchBody(token)).
			Return(page(t, next, false, fixture(t, "view_todo.json")), nil).Once()
		token = next
	}

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.ErrorIs(t, err, atlassian.ErrTooManySearchPages)
	require.Nil(t, tasks)
}

func TestGetMyTasksLastPageAtLimit(t *testing.T) {
	const maxPages = 100

	m := mocks.NewMockHTTPClient(t)
	token := ""

	for i := 1; i <= maxPages; i++ {
		next := fmt.Sprintf("p%d", i+1)

		isLast := i == maxPages
		if isLast {
			next = ""
		}

		m.EXPECT().Post(t.Context(), searchURL, header, searchBody(token)).
			Return(page(t, next, isLast, fixture(t, "view_todo.json")), nil).Once()
		token = next
	}

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.NoError(t, err)
	require.Len(t, tasks, maxPages)
}

func TestGetMyTasksJoinsSiteURL(t *testing.T) {
	tests := []struct {
		name    string
		siteURL string
		want    string
	}{
		{name: "trailing slash", siteURL: siteURL + "/", want: searchURL},
		{
			name:    "gateway path prefix",
			siteURL: "https://api.atlassian.com/ex/jira/abc-123",
			want:    "https://api.atlassian.com/ex/jira/abc-123/rest/api/3/search/jql",
		},
		{
			name:    "gateway path prefix with trailing slash",
			siteURL: "https://api.atlassian.com/ex/jira/abc-123/",
			want:    "https://api.atlassian.com/ex/jira/abc-123/rest/api/3/search/jql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Post(t.Context(), tt.want, header, searchBody("")).Return(page(t, "", true), nil).Once()

			tasks, err := atlassian.NewClient(m, tt.siteURL, email, apiToken).GetMyTasks(t.Context())
			require.NoError(t, err)
			require.NotNil(t, tasks)
		})
	}
}

func TestGetMyTasksNoWorkItems(t *testing.T) {
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).Return([]byte(`{"isLast":true}`), nil).Once()

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.NoError(t, err)
	require.NotNil(t, tasks)
	require.Empty(t, tasks)
}

func TestGetMyTasksPageErrors(t *testing.T) {
	tests := []struct {
		name    string
		out     []byte
		postErr error
		wantErr error
	}{
		{name: "request fails", postErr: httputil.ErrUnexpectedStatusCode, wantErr: httputil.ErrUnexpectedStatusCode},
		{name: "response is not json", out: []byte("<html>"), wantErr: atlassian.ErrFailedToParseSearchOutput},
		{name: "issues is not an array", out: []byte(`{"issues":{}}`), wantErr: atlassian.ErrFailedToParseSearchOutput},
	}

	for _, tt := range tests {
		t.Run("first page: "+tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).Return(tt.out, tt.postErr).Once()

			tasks, err := newClient(m).GetMyTasks(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, tasks)
		})

		t.Run("later page: "+tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
				Return(page(t, "p2", false, fixture(t, "view_todo.json"), "null"), nil).Once()
			m.EXPECT().Post(t.Context(), searchURL, header, searchBody("p2")).Return(tt.out, tt.postErr).Once()

			tasks, err := newClient(m).GetMyTasks(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			require.NotErrorIs(t, err, atlassian.ErrMissingWorkItemKey)
			require.Nil(t, tasks)
		})
	}
}

func TestGetMyTasksSkipsBadItems(t *testing.T) {
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
		Return(page(t, "p2", false, "null", `{"key":"WHF-9","fields":{"summary":7}}`), nil).Once()
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("p2")).
		Return(page(t, "", true, `{"fields":{"summary":"no key"}}`, fixture(t, "view_canceled.json")), nil).Once()

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.Equal(t, []task.Task{
		{
			ID:          "jira-WHF-4",
			Originated:  task.OriginJira,
			Title:       "Migrate legacy cron jobs",
			Description: "Superseded by WHF-2.",
			Status:      task.StatusCanceled,
		},
	}, tasks)
	require.ErrorIs(t, err, atlassian.ErrMissingWorkItemKey)
	require.ErrorContains(t, err, "missing work item key: index=0")
	require.ErrorIs(t, err, atlassian.ErrFailedToParseWorkItem)
	require.ErrorContains(t, err, "failed to parse work item: index=1")
	require.ErrorContains(t, err, "missing work item key: index=2")
}

func TestGetMyTasksOnlyNullWorkItems(t *testing.T) {
	m := mocks.NewMockHTTPClient(t)
	m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).Return(page(t, "", true, "null"), nil).Once()

	tasks, err := newClient(m).GetMyTasks(t.Context())
	require.ErrorIs(t, err, atlassian.ErrMissingWorkItemKey)
	require.NotNil(t, tasks)
	require.Empty(t, tasks)
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
			want:     task.Task{ID: "jira-WHF-1", Originated: task.OriginJira, Title: "Write onboarding guide"},
			wantErrs: []error{atlassian.ErrUnknownStatusCategory},
		},
		{
			name: "start date is invalid",
			out:  fixture(t, "view_invalid_start_date.json"),
			want: task.Task{
				ID:         "jira-WHF-1",
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
				ID:         "jira-WHF-1",
				Originated: task.OriginJira,
				Title:      "Write onboarding guide",
				Status:     task.StatusTodo,
			},
			wantErrs: []error{atlassian.ErrFailedToParseDueDate},
		},
		{
			name: "every field is invalid",
			out:  fixture(t, "view_invalid_fields.json"),
			want: task.Task{ID: "jira-WHF-1", Originated: task.OriginJira, Title: "Write onboarding guide"},
			wantErrs: []error{
				atlassian.ErrUnknownStatusCategory,
				atlassian.ErrFailedToParseStartDate,
				atlassian.ErrFailedToParseDueDate,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mocks.NewMockHTTPClient(t)
			m.EXPECT().Post(t.Context(), searchURL, header, searchBody("")).
				Return(page(t, "", true, tt.out, fixture(t, "view_done.json")), nil).Once()

			tasks, err := newClient(m).GetMyTasks(t.Context())
			require.Len(t, tasks, 2)
			require.Equal(t, tt.want, tasks[0])
			require.Equal(t, "jira-WHF-3", tasks[1].ID)
			require.ErrorContains(t, err, "key=WHF-1")

			for _, wantErr := range tt.wantErrs {
				require.ErrorIs(t, err, wantErr)
			}
		})
	}
}

// TestGetMyTasksCanceledContext runs the real HTTP client so the context
// reaches the request, as it does in production.
func TestGetMyTasksCanceledContext(t *testing.T) {
	resp := page(t, "", true, fixture(t, "view_todo.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	tasks, err := atlassian.NewClient(httputil.NewHTTPClient(), srv.URL, email, apiToken).GetMyTasks(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tasks)
}

// TestGetMyTasksOverHTTP checks the wire request against a real server.
func TestGetMyTasksOverHTTP(t *testing.T) {
	var (
		got  *http.Request
		body []byte
	)

	resp := page(t, "", true, fixture(t, "view_todo.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		body, _ = io.ReadAll(r.Body)

		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	tasks, err := atlassian.NewClient(httputil.NewHTTPClient(), srv.URL+"/ex/jira/abc/", email, apiToken).
		GetMyTasks(t.Context())
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, http.MethodPost, got.Method)
	require.Equal(t, "/ex/jira/abc/rest/api/3/search/jql", got.URL.Path)

	user, pass, ok := got.BasicAuth()
	require.True(t, ok)
	require.Equal(t, email, user)
	require.Equal(t, apiToken, pass)
	require.Equal(t, "application/json", got.Header.Get("Content-Type"))
	require.Equal(t, "application/json", got.Header.Get("Accept"))
	require.JSONEq(t, string(searchBody("")), string(body))
}
