package server_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
)

// serve sends method path with the request ID "req-1" to a handler backed by
// svc and returns the response and the logs.
func serve(t *testing.T, svc service.Service, method, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()

	var logs bytes.Buffer

	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	req.Header.Set("X-Request-ID", "req-1")

	rec := httptest.NewRecorder()
	server.NewHandler(svc, slog.New(slog.NewJSONHandler(&logs, nil))).ServeHTTP(rec, req)

	return rec, logs.String()
}

func TestHealthz(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/healthz")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestSyncJiraTasks(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().SyncJiraTasks(mock.Anything).Return(service.SyncResult{Upserted: 3}, nil).Once()

	rec, logs := serve(t, svc, http.MethodPost, "/tasks/jira")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"upserted":3,"partial":false}`, rec.Body.String())
	require.NotContains(t, logs, `"level":"WARN"`)
}

func TestSyncJiraTasksPartial(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		SyncJiraTasks(mock.Anything).
		Return(service.SyncResult{Upserted: 2, Partial: errors.New("view failed: key=WHF-3")}, nil).
		Once()

	rec, logs := serve(t, svc, http.MethodPost, "/tasks/jira")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"upserted":2,"partial":true}`, rec.Body.String())
	require.Contains(t, logs, `"level":"WARN"`)
	require.Contains(t, logs, `"request_id":"req-1"`)
	require.Contains(t, logs, "key=WHF-3")
}

func TestSyncJiraTasksFailure(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().SyncJiraTasks(mock.Anything).Return(service.SyncResult{}, errors.New("secret detail")).Once()

	rec, logs := serve(t, svc, http.MethodPost, "/tasks/jira")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"internal server error"}`, rec.Body.String())
	require.Contains(t, logs, `"request_id":"req-1"`)
	require.Contains(t, logs, "secret detail")
}

func TestRoutesRejectOtherMethods(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/tasks/jira")

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
