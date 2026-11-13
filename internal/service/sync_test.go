package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/service"
)

func request(key, category string) servicedesk.Request {
	return servicedesk.Request{Key: key, Title: key, Status: category, StatusCategory: category}
}

// stamped matches the tasks a full sync upserts: want, each carrying the same
// ClosedSyncedAt, the time of the sync.
func stamped(want []task.Task) any {
	return mock.MatchedBy(func(got []task.Task) bool {
		if len(got) != len(want) {
			return false
		}

		bare := make([]task.Task, 0, len(got))
		for _, g := range got {
			if g.ClosedSyncedAt == nil || !g.ClosedSyncedAt.Equal(*got[0].ClosedSyncedAt) {
				return false
			}

			g.ClosedSyncedAt = nil
			bare = append(bare, g)
		}

		return assert.ObjectsAreEqual(want, bare)
	})
}

func TestSyncAtlassian(t *testing.T) {
	issueErr := errors.New("bad status: key=WHF-2")
	requestErr := errors.Join(errors.New("missing title: key=SD-8"), errors.New("missing key: index=9"))
	fetched := []servicedesk.Request{
		request("SD-1", "NEW"),
		request("SD-2", "DONE"),
		request("SD-3", "DONE"),
		request("SD-4", "INDETERMINATE"),
	}

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, errors.Join(issueErr)).Once()
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(fetched, requestErr).Once()

	var recorded atlassiansync.Record

	manager := mocks.NewMockManager(t)
	// SD-2 was open and closes now; SD-3 was already closed.
	manager.EXPECT().
		ListServiceDeskRequests(mock.Anything, false).
		Return([]servicedesk.Request{request("SD-1", "NEW"), request("SD-2", "NEW")}, nil).
		Once()
	manager.EXPECT().ListTasks(mock.Anything).Return(tasks, nil).Once()

	var upserted []task.Task

	manager.EXPECT().UpsertTasks(mock.Anything, stamped(tasks)).
		Run(func(_ context.Context, ts []task.Task) { upserted = ts }).
		Return(nil).
		Once()
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, fetched).Return(nil).Once()
	manager.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).
		Run(func(_ context.Context, r atlassiansync.Record) { recorded = r }).
		Return(nil).
		Once()

	before := time.Now()

	got, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
	require.NoError(t, err)
	require.WithinRange(t, got.Record.SyncedAt, before.UTC(), time.Now().UTC())
	require.Equal(t, time.UTC, got.Record.SyncedAt.Location())
	require.Equal(t, recorded, got.Record)
	require.Equal(t, recorded.SyncedAt, *upserted[0].ClosedSyncedAt)
	require.Nil(t, tasks[0].ClosedSyncedAt, "the fetched tasks are not stamped in place")

	got.Record.SyncedAt = time.Time{}
	require.Equal(t, service.AtlassianSync{
		Record: atlassiansync.Record{
			Issues:          2,
			IssueFailures:   1,
			OpenRequests:    2,
			ClosedRequests:  1,
			RequestFailures: 2,
		},
		Errors: []string{"bad status: key=WHF-2", "missing title: key=SD-8", "missing key: index=9"},
	}, got)
}

func TestSyncAtlassianCompleteHasNoErrors(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return([]task.Task{}, nil).Once()
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return([]servicedesk.Request{}, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
	manager.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()
	manager.EXPECT().UpsertTasks(mock.Anything, []task.Task{}).Return(nil).Once()
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request{}).Return(nil).Once()
	manager.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).Return(nil).Once()

	got, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{}, got.Errors)
	require.Zero(t, got.Record.Issues)
}

func TestSyncAtlassianTreatsNilWithoutErrorAsEmpty(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(nil, nil).Once()
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(nil, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
	manager.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()
	manager.EXPECT().UpsertTasks(mock.Anything, []task.Task(nil)).Return(nil).Once()
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request(nil)).Return(nil).Once()
	manager.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).Return(nil).Once()

	got, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
	require.NoError(t, err)
	require.Zero(t, got.Record.Issues)
}

func TestSyncAtlassianRefetchesDroppedJiraTasks(t *testing.T) {
	jiraTask := func(key string, status task.Status) task.Task {
		return task.Task{ID: task.NewID(task.OriginJira, key), Originated: task.OriginJira, Title: key, Status: status}
	}

	searched := jiraTask("WHF-1", task.StatusTodo)
	finished := jiraTask("WHF-2", task.StatusDone)
	stored := []task.Task{
		searched,
		jiraTask("WHF-2", task.StatusInProgress), // done in Jira since the last sync
		jiraTask("WHF-3", task.StatusDone),
		jiraTask("WHF-4", task.StatusCanceled),
		jiraTask("WHF-5", task.StatusTodo), // gone from Jira
		jiraTask("WHF-6", task.StatusTodo), // moved to another project
		{ID: "local-WHF-001", Originated: task.OriginLocal, Title: "local", Status: task.StatusTodo},
	}
	gone := fmt.Errorf("%w: key=WHF-5", atlassian.ErrWorkItemNotFound)

	// Only WHF-2, WHF-5 and WHF-6 are fetched again: the mock fails the test
	// on any other GetTask call.
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return([]task.Task{searched}, nil).Once()
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return([]servicedesk.Request{}, nil).Once()
	jira.EXPECT().GetTask(mock.Anything, "WHF-2").Return(finished, nil).Once()
	jira.EXPECT().GetTask(mock.Anything, "WHF-5").Return(task.Task{}, gone).Once()
	jira.EXPECT().GetTask(mock.Anything, "WHF-6").Return(jiraTask("ABC-1", task.StatusTodo), nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
	manager.EXPECT().ListTasks(mock.Anything).Return(stored, nil).Once()
	manager.EXPECT().UpsertTasks(mock.Anything, stamped([]task.Task{searched, finished})).Return(nil).Once()
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request{}).Return(nil).Once()
	manager.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).Return(nil).Once()

	got, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, got.Record.Issues)
	require.Equal(t, 2, got.Record.IssueFailures)
	require.Equal(t, []string{
		"atlassian: work item not found: key=WHF-5: id=jira-WHF-5",
		"service: jira issue key changed: id=jira-WHF-6, new_id=jira-ABC-1",
	}, got.Errors)
}

func TestSyncAtlassianStopsRefetchingOnRequestFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"status":      {err: statusErr(t, http.StatusTooManyRequests), want: httputil.ErrUnexpectedStatusCode},
		"unreachable": {err: fmt.Errorf("%w: method=GET", httputil.ErrFailedToSendRequest), want: service.ErrAtlassianUnreachable},
	} {
		t.Run(name, func(t *testing.T) {
			searched := task.Task{ID: "jira-WHF-1", Originated: task.OriginJira, Status: task.StatusTodo}
			stored := []task.Task{
				searched,
				{ID: "jira-WHF-2", Originated: task.OriginJira, Status: task.StatusTodo},
				{ID: "jira-WHF-3", Originated: task.OriginJira, Status: task.StatusTodo},
			}

			// WHF-3 is never fetched: the first failure stops the refetch.
			jira := mocks.NewMockClient(t)
			jira.EXPECT().GetMyTasks(mock.Anything).Return([]task.Task{searched}, nil).Once()
			jira.EXPECT().GetOwnedRequests(mock.Anything).Return([]servicedesk.Request{}, nil).Once()
			jira.EXPECT().GetTask(mock.Anything, "WHF-2").Return(task.Task{}, tc.err).Once()

			manager := mocks.NewMockManager(t)
			manager.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
			manager.EXPECT().ListTasks(mock.Anything).Return(stored, nil).Once()
			manager.EXPECT().UpsertTasks(mock.Anything, stamped([]task.Task{searched})).Return(nil).Once()
			manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request{}).Return(nil).Once()
			manager.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).Return(nil).Once()

			got, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, got.Record.IssueFailures)
			require.Len(t, got.Errors, 1)
			require.Contains(t, got.Errors[0], tc.want.Error())
			require.Contains(t, got.Errors[0], "id=jira-WHF-2")
		})
	}
}

func TestSyncAtlassianChangesNothingWhenAFetchFails(t *testing.T) {
	failure := errors.New("page failed")

	t.Run("issues", func(t *testing.T) {
		jira := mocks.NewMockClient(t)
		jira.EXPECT().GetMyTasks(mock.Anything).Return(nil, failure).Once()

		// No manager expectation: the mock fails the test if it is called.
		_, err := service.NewService(jira, mocks.NewMockManager(t)).SyncAtlassian(t.Context())
		require.ErrorIs(t, err, failure)
	})

	t.Run("requests", func(t *testing.T) {
		jira := mocks.NewMockClient(t)
		jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, nil).Once()
		jira.EXPECT().GetOwnedRequests(mock.Anything).Return(nil, failure).Once()

		_, err := service.NewService(jira, mocks.NewMockManager(t)).SyncAtlassian(t.Context())
		require.ErrorIs(t, err, failure)
	})
}

func TestSyncAtlassianStorageFailures(t *testing.T) {
	failure := errors.New("disk failed")

	for name, fail := range map[string]func(*mocks.MockManager){
		"list open": func(m *mocks.MockManager) {
			m.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, failure).Once()
		},
		"list tasks": func(m *mocks.MockManager) {
			m.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
			m.EXPECT().ListTasks(mock.Anything).Return(nil, failure).Once()
		},
		"tasks": func(m *mocks.MockManager) {
			m.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
			m.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()
			m.EXPECT().UpsertTasks(mock.Anything, stamped(tasks)).Return(failure).Once()
		},
		"requests": func(m *mocks.MockManager) {
			m.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
			m.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()
			m.EXPECT().UpsertTasks(mock.Anything, stamped(tasks)).Return(nil).Once()
			m.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(failure).Once()
		},
		"record": func(m *mocks.MockManager) {
			m.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(nil, nil).Once()
			m.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()
			m.EXPECT().UpsertTasks(mock.Anything, stamped(tasks)).Return(nil).Once()
			m.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(nil).Once()
			m.EXPECT().CreateAtlassianSync(mock.Anything, mock.Anything).Return(failure).Once()
		},
	} {
		t.Run(name, func(t *testing.T) {
			jira := mocks.NewMockClient(t)
			jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, nil).Once()
			jira.EXPECT().GetOwnedRequests(mock.Anything).Return(requests, nil).Once()

			manager := mocks.NewMockManager(t)
			fail(manager)

			_, err := service.NewService(jira, manager).SyncAtlassian(t.Context())
			require.ErrorIs(t, err, failure)
		})
	}
}

// statusErr returns the error an HTTP client returns for a status.
func statusErr(t *testing.T, status int) error {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	_, err := httputil.NewHTTPClient().Get(t.Context(), srv.URL, nil)
	require.Error(t, err)

	return err
}

func TestSyncAtlassianClassifiesAtlassianErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		want     error
		unwanted []error
	}{
		"not configured": {err: atlassian.ErrNotConfigured, want: service.ErrAtlassianNotConfigured},
		"401":            {err: statusErr(t, http.StatusUnauthorized), want: service.ErrAtlassianUnauthorized},
		"403":            {err: statusErr(t, http.StatusForbidden), want: service.ErrAtlassianUnauthorized},
		"unreachable": {
			err:  fmt.Errorf("%w: method=POST", httputil.ErrFailedToSendRequest),
			want: service.ErrAtlassianUnreachable,
		},
		"other status": {
			err:  statusErr(t, http.StatusInternalServerError),
			want: httputil.ErrUnexpectedStatusCode,
			unwanted: []error{
				service.ErrAtlassianUnauthorized, service.ErrAtlassianUnreachable, service.ErrAtlassianNotConfigured,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			jira := mocks.NewMockClient(t)
			jira.EXPECT().GetMyTasks(mock.Anything).Return(nil, tc.err).Once()

			_, err := service.NewService(jira, mocks.NewMockManager(t)).SyncAtlassian(t.Context())
			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, err, tc.err)

			for _, u := range tc.unwanted {
				require.NotErrorIs(t, err, u)
			}
		})
	}
}

func TestGetLastAtlassianSync(t *testing.T) {
	record := atlassiansync.Record{SyncedAt: time.Unix(1, 0).UTC(), Issues: 3}
	failure := errors.New("read failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetLastAtlassianSync(mock.Anything).Return(record, nil).Once()
	manager.EXPECT().
		GetLastAtlassianSync(mock.Anything).
		Return(atlassiansync.Record{}, db.ErrAtlassianSyncNotFound).
		Once()
	manager.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{}, failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	got, err := svc.GetLastAtlassianSync(t.Context())
	require.NoError(t, err)
	require.Equal(t, record, got)

	_, err = svc.GetLastAtlassianSync(t.Context())
	require.ErrorIs(t, err, service.ErrNoAtlassianSync)

	_, err = svc.GetLastAtlassianSync(t.Context())
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrNoAtlassianSync)
}

func TestSyncJiraTask(t *testing.T) {
	due := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	stored := task.Task{
		ID: "jira-SEC-142", Originated: task.OriginJira, Title: "KMS", Description: "old",
		Status: task.StatusTodo, DueDate: &due, ParentID: "jira-SEC-120",
	}
	sameDue := due
	fresh := task.Task{
		ID: "jira-SEC-142", Originated: task.OriginJira, Title: "KMS", Description: "new",
		Status: task.StatusInProgress, DueDate: &sameDue, ParentID: "jira-SEC-120",
		ChildrenIDs: []string{"jira-SEC-143"},
	}

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetTask(mock.Anything, "SEC-142").Return(fresh, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "jira-SEC-142").Return(stored, nil).Once()
	manager.EXPECT().UpsertTasks(mock.Anything, []task.Task{fresh}).Return(nil).Once()

	got, err := service.NewService(jira, manager).SyncJiraTask(t.Context(), "jira-SEC-142")
	require.NoError(t, err)
	require.Equal(t, []string{"description", "status"}, got.Changes)
	require.Equal(t, "new", got.Task.Description)
	require.Empty(t, got.Task.ParentID)
	require.Nil(t, got.Task.ChildrenIDs)
}

func TestSyncJiraTaskReportsEveryChangedField(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stored := task.Task{ID: "jira-A-1", Originated: task.OriginJira, Title: "a", StartDate: &start}
	fresh := task.Task{ID: "jira-A-1", Originated: task.OriginJira, Title: "b", DueDate: &start}

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetTask(mock.Anything, "A-1").Return(fresh, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(stored, nil).Once()
	manager.EXPECT().UpsertTasks(mock.Anything, mock.Anything).Return(nil).Once()

	got, err := service.NewService(jira, manager).SyncJiraTask(t.Context(), "jira-A-1")
	require.NoError(t, err)
	require.Equal(t, []string{"title", "start_date", "due_date"}, got.Changes)
}

func TestSyncJiraTaskFailures(t *testing.T) {
	failure := errors.New("disk failed")
	jiraTask := task.Task{ID: "jira-A-1", Originated: task.OriginJira}

	t.Run("not stored", func(t *testing.T) {
		manager := mocks.NewMockManager(t)
		manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(task.Task{}, db.ErrTaskNotFound).Once()

		_, err := service.NewService(mocks.NewMockClient(t), manager).SyncJiraTask(t.Context(), "jira-A-1")
		require.ErrorIs(t, err, service.ErrTaskToSyncNotFound)
		require.ErrorContains(t, err, "id=jira-A-1")
	})

	t.Run("read failure", func(t *testing.T) {
		manager := mocks.NewMockManager(t)
		manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(task.Task{}, failure).Once()

		_, err := service.NewService(mocks.NewMockClient(t), manager).SyncJiraTask(t.Context(), "jira-A-1")
		require.ErrorIs(t, err, failure)
	})

	t.Run("local", func(t *testing.T) {
		manager := mocks.NewMockManager(t)
		manager.EXPECT().
			GetTask(mock.Anything, "local-WHF-001").
			Return(task.Task{ID: "local-WHF-001", Originated: task.OriginLocal}, nil).
			Once()

		_, err := service.NewService(mocks.NewMockClient(t), manager).SyncJiraTask(t.Context(), "local-WHF-001")
		require.ErrorIs(t, err, service.ErrTaskNotJira)
	})

	t.Run("gone from jira", func(t *testing.T) {
		jira := mocks.NewMockClient(t)
		jira.EXPECT().GetTask(mock.Anything, "A-1").Return(task.Task{}, atlassian.ErrWorkItemNotFound).Once()

		// No UpsertTasks expectation: the stored copy is kept.
		manager := mocks.NewMockManager(t)
		manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(jiraTask, nil).Once()

		_, err := service.NewService(jira, manager).SyncJiraTask(t.Context(), "jira-A-1")
		require.ErrorIs(t, err, service.ErrJiraIssueNotFound)
		require.ErrorContains(t, err, "key=A-1")
	})

	t.Run("unauthorized", func(t *testing.T) {
		jira := mocks.NewMockClient(t)
		jira.EXPECT().GetTask(mock.Anything, "A-1").Return(task.Task{}, statusErr(t, http.StatusUnauthorized)).Once()

		manager := mocks.NewMockManager(t)
		manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(jiraTask, nil).Once()

		_, err := service.NewService(jira, manager).SyncJiraTask(t.Context(), "jira-A-1")
		require.ErrorIs(t, err, service.ErrAtlassianUnauthorized)
	})

	t.Run("store failure", func(t *testing.T) {
		jira := mocks.NewMockClient(t)
		jira.EXPECT().GetTask(mock.Anything, "A-1").Return(jiraTask, nil).Once()

		manager := mocks.NewMockManager(t)
		manager.EXPECT().GetTask(mock.Anything, "jira-A-1").Return(jiraTask, nil).Once()
		manager.EXPECT().UpsertTasks(mock.Anything, []task.Task{jiraTask}).Return(failure).Once()

		_, err := service.NewService(jira, manager).SyncJiraTask(t.Context(), "jira-A-1")
		require.ErrorIs(t, err, failure)
	})
}

func TestListServiceDeskRequests(t *testing.T) {
	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListServiceDeskRequests(mock.Anything, true).Return(requests, nil).Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).ListServiceDeskRequests(t.Context(), true)
	require.NoError(t, err)
	require.Equal(t, requests, got)
}
