package server_test

import (
	"errors"
	"net/http"
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
