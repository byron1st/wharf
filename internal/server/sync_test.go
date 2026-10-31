package server_test

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/service"
)

var record = atlassiansync.Record{
	SyncedAt:        time.Date(2026, 10, 26, 5, 2, 0, 0, time.UTC),
	Issues:          6,
	IssueFailures:   1,
	OpenRequests:    4,
	ClosedRequests:  2,
	RequestFailures: 0,
}

func TestSyncAtlassianRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		SyncAtlassian(mock.Anything).
		Return(service.AtlassianSync{Record: record, Errors: []string{}}, nil).
		Once()
	svc.EXPECT().
		SyncAtlassian(mock.Anything).
		Return(service.AtlassianSync{Record: record, Errors: []string{"bad status: key=SEC-171"}}, nil).
		Once()

	rec, logs := serve(t, svc, http.MethodPost, "/api/sync/atlassian")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"synced_at":"2026-10-26T05:02:00Z","issues":6,"issue_failures":1,"open_requests":4,
		"closed_requests":2,"request_failures":0}`, rec.Body.String())
	require.NotContains(t, logs, `"level":"WARN"`)

	rec, logs = serve(t, svc, http.MethodPost, "/api/sync/atlassian")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"synced_at":"2026-10-26T05:02:00Z","issues":6,"issue_failures":1,"open_requests":4,
		"closed_requests":2,"request_failures":0,"errors":["bad status: key=SEC-171"]}`, rec.Body.String())
	require.Contains(t, logs, `"level":"WARN"`)
	require.Contains(t, logs, "key=SEC-171")
}

func TestAtlassianRoutesMapErrorCodes(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: service.ErrAtlassianNotConfigured, status: http.StatusConflict, code: "atlassian_not_configured"},
		{err: service.ErrAtlassianUnauthorized, status: http.StatusBadGateway, code: "atlassian_unauthorized"},
		{err: service.ErrAtlassianUnreachable, status: http.StatusGatewayTimeout, code: "atlassian_unreachable"},
		{err: service.ErrJiraIssueNotFound, status: http.StatusNotFound, code: "jira_issue_not_found"},
		{err: service.ErrTaskToSyncNotFound, status: http.StatusNotFound},
		{err: service.ErrTaskNotJira, status: http.StatusConflict},
		{err: errors.New("disk failed"), status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			svc := mocks.NewMockService(t)
			svc.EXPECT().SyncAtlassian(mock.Anything).Return(service.AtlassianSync{}, tt.err).Once()
			svc.EXPECT().SyncJiraTask(mock.Anything, "jira-A-1").Return(service.TaskSync{}, tt.err).Once()

			for _, path := range []string{"/api/sync/atlassian", "/api/tasks/jira-A-1/sync"} {
				rec, _ := serve(t, svc, http.MethodPost, path)

				require.Equal(t, tt.status, rec.Code, path)

				want := `{"error":` + strconv.Quote(tt.err.Error())
				if tt.code != "" {
					want += `,"code":"` + tt.code + `"`
				}

				require.JSONEq(t, want+"}", rec.Body.String(), path)
			}
		})
	}
}

func TestGetLastAtlassianSyncRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().GetLastAtlassianSync(mock.Anything).Return(record, nil).Once()
	svc.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{}, service.ErrNoAtlassianSync).Once()
	svc.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{}, errors.New("disk failed")).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/sync/atlassian")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"last_sync":{"synced_at":"2026-10-26T05:02:00Z","issues":6,"issue_failures":1,
		"open_requests":4,"closed_requests":2,"request_failures":0}}`, rec.Body.String())

	rec, _ = serve(t, svc, http.MethodGet, "/api/sync/atlassian")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"last_sync":null}`, rec.Body.String())

	rec, _ = serve(t, svc, http.MethodGet, "/api/sync/atlassian")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestListServiceDeskRequestsRoute(t *testing.T) {
	at := time.Date(2026, 10, 24, 7, 40, 0, 0, time.UTC)
	requests := []servicedesk.Request{{
		Key: "CIS-12345", Title: "KMS", Status: "Security Review", StatusCategory: "INDETERMINATE",
		Created: at.Add(-72 * time.Hour), StatusAt: at,
	}}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ListServiceDeskRequests(mock.Anything, false).Return(requests, nil).Once()
	svc.EXPECT().ListServiceDeskRequests(mock.Anything, true).Return(nil, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/service-desk/requests")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"requests":[{"key":"CIS-12345","title":"KMS","status":"Security Review",
		"status_category":"INDETERMINATE","created_at":"2026-10-21T07:40:00Z","status_at":"2026-10-24T07:40:00Z"}]}`,
		rec.Body.String())

	rec, _ = serve(t, svc, http.MethodGet, "/api/service-desk/requests?include_closed=true")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"requests":[]}`, rec.Body.String())

	rec, _ = serve(t, mocks.NewMockService(t), http.MethodGet, "/api/service-desk/requests?include_closed=maybe")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "server: include_closed filter is not a bool")
}

func TestSyncJiraTaskRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().SyncJiraTask(mock.Anything, "jira-SEC-142").Return(service.TaskSync{
		Task: task.Task{
			ID:         "jira-SEC-142",
			Originated: task.OriginJira,
			Title:      "KMS",
			Status:     task.StatusInProgress,
		},
		Changes: []string{"status"},
	}, nil).Once()

	rec, _ := serve(t, svc, http.MethodPost, "/api/tasks/jira-SEC-142/sync")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"task":{"id":"jira-SEC-142","originated":"jira","title":"KMS","description":"",
		"status":"in-progress","start_date":null,"due_date":null},"changes":["status"]}`, rec.Body.String())
}
