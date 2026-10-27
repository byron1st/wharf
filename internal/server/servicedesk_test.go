package server_test

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/service"
)

func TestSyncServiceDeskRequestsRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().SyncServiceDeskRequests(mock.Anything).Return(service.SyncResult{Upserted: 3}, nil).Once()

	rec, logs := serve(t, svc, http.MethodPost, "/service-desk/requests")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"upserted":3,"partial":false}`, rec.Body.String())
	require.NotContains(t, logs, `"level":"WARN"`)
}

func TestSyncServiceDeskRequestsRoutePartial(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		SyncServiceDeskRequests(mock.Anything).
		Return(service.SyncResult{Upserted: 2, Partial: errors.New("missing title: key=SD-3")}, nil).
		Once()

	rec, logs := serve(t, svc, http.MethodPost, "/service-desk/requests")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"upserted":2,"partial":true}`, rec.Body.String())
	require.Contains(t, logs, `"level":"WARN"`)
	require.Contains(t, logs, `"request_id":"req-1"`)
	require.Contains(t, logs, "key=SD-3")
}

func TestSyncServiceDeskRequestsRouteFailure(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		SyncServiceDeskRequests(mock.Anything).
		Return(service.SyncResult{}, errors.New("secret detail")).
		Once()

	rec, logs := serve(t, svc, http.MethodPost, "/service-desk/requests")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"secret detail"}`, rec.Body.String())
	require.Contains(t, logs, `"request_id":"req-1"`)
	require.Contains(t, logs, "secret detail")
}

func TestLinkServiceDeskRequestRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().LinkServiceDeskRequest(mock.Anything, "local-WHF-001", "SD-1").Return(nil).Twice()

	// Repeating the PUT is a no-op the service reports as success.
	for range 2 {
		rec, _ := serve(t, svc, http.MethodPut, "/tasks/local-WHF-001/service-desk-requests/SD-1")

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Empty(t, rec.Body.String())
	}
}

func TestUnlinkServiceDeskRequestRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().UnlinkServiceDeskRequest(mock.Anything, "jira-WHF-1", "SD-1").Return(nil).Once()

	rec, _ := serve(t, svc, http.MethodDelete, "/tasks/jira-WHF-1/service-desk-requests/SD-1")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestServiceDeskRequestLinkRoutesMapServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "task to link not found",
			err:    fmt.Errorf("%w: task_id=T1", service.ErrTaskToLinkNotFound),
			status: http.StatusNotFound,
		},
		{
			name:   "request to link not found",
			err:    fmt.Errorf("%w: key=SD-1", service.ErrServiceDeskRequestToLinkNotFound),
			status: http.StatusNotFound,
		},
		{
			name:   "link to delete not found",
			err:    service.ErrServiceDeskRequestLinkToDeleteNotFound,
			status: http.StatusNotFound,
		},
		{name: "other", err: errors.New("disk failed"), status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := mocks.NewMockService(t)
			svc.EXPECT().LinkServiceDeskRequest(mock.Anything, "T1", "SD-1").Return(tt.err).Once()
			svc.EXPECT().UnlinkServiceDeskRequest(mock.Anything, "T1", "SD-1").Return(tt.err).Once()

			for _, method := range []string{http.MethodPut, http.MethodDelete} {
				rec, logs := serve(t, svc, method, "/tasks/T1/service-desk-requests/SD-1")

				require.Equal(t, tt.status, rec.Code, method)
				require.JSONEq(t, `{"error":`+strconv.Quote(tt.err.Error())+`}`, rec.Body.String())
				require.Contains(t, logs, `"request_id":"req-1"`)
			}
		})
	}
}
