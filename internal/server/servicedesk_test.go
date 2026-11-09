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

func TestLinkServiceDeskRequestRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().LinkServiceDeskRequest(mock.Anything, int64(7), "SD-1").Return(nil).Twice()

	// Repeating the PUT is a no-op the service reports as success.
	for range 2 {
		rec, _ := serve(t, svc, http.MethodPut, "/api/todos/7/service-desk-requests/SD-1")

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Empty(t, rec.Body.String())
	}
}

func TestUnlinkServiceDeskRequestRoute(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().UnlinkServiceDeskRequest(mock.Anything, int64(7), "SD-1").Return(nil).Once()

	rec, _ := serve(t, svc, http.MethodDelete, "/api/todos/7/service-desk-requests/SD-1")

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
			name:   "todo to link not found",
			err:    fmt.Errorf("%w: todo_id=1", service.ErrTodoToLinkNotFound),
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
			svc.EXPECT().LinkServiceDeskRequest(mock.Anything, int64(1), "SD-1").Return(tt.err).Once()
			svc.EXPECT().UnlinkServiceDeskRequest(mock.Anything, int64(1), "SD-1").Return(tt.err).Once()

			for _, method := range []string{http.MethodPut, http.MethodDelete} {
				rec, logs := serve(t, svc, method, "/api/todos/1/service-desk-requests/SD-1")

				require.Equal(t, tt.status, rec.Code, method)
				require.JSONEq(t, `{"error":`+strconv.Quote(tt.err.Error())+`}`, rec.Body.String())
				require.Contains(t, logs, `"request_id":"req-1"`)
			}
		})
	}
}
