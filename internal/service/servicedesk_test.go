package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/service"
)

var requests = []servicedesk.Request{
	{
		Key:      "SD-1",
		Title:    "Mouse",
		Status:   "Open",
		Created:  time.UnixMilli(1).UTC(),
		StatusAt: time.UnixMilli(2).UTC(),
	},
}

func TestLinkServiceDeskRequest(t *testing.T) {
	manager := mocks.NewMockManager(t)
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, int64(7), "SD-1").Return(nil).Once()

	err := service.NewService(mocks.NewMockClient(t), manager).LinkServiceDeskRequest(t.Context(), 7, "SD-1")
	require.NoError(t, err)
}

func TestLinkServiceDeskRequestFailures(t *testing.T) {
	failure := errors.New("insert failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, int64(9), "SD-1").Return(db.ErrTodoToLinkNotFound).Once()
	manager.EXPECT().
		LinkServiceDeskRequest(mock.Anything, int64(1), "SD-9").
		Return(db.ErrServiceDeskRequestToLinkNotFound).
		Once()
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, int64(1), "SD-1").Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	err := svc.LinkServiceDeskRequest(t.Context(), 9, "SD-1")
	require.ErrorIs(t, err, service.ErrTodoToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
	require.ErrorContains(t, err, "todo_id=9")

	err = svc.LinkServiceDeskRequest(t.Context(), 1, "SD-9")
	require.ErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrTodoToLinkNotFound)
	require.ErrorContains(t, err, "key=SD-9")

	err = svc.LinkServiceDeskRequest(t.Context(), 1, "SD-1")
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrTodoToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
}

func TestUnlinkServiceDeskRequest(t *testing.T) {
	failure := errors.New("delete failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UnlinkServiceDeskRequest(mock.Anything, int64(1), "SD-1").Return(nil).Once()
	manager.EXPECT().
		UnlinkServiceDeskRequest(mock.Anything, int64(1), "SD-9").
		Return(db.ErrServiceDeskRequestLinkToDeleteNotFound).
		Once()
	manager.EXPECT().UnlinkServiceDeskRequest(mock.Anything, int64(1), "SD-2").Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	require.NoError(t, svc.UnlinkServiceDeskRequest(t.Context(), 1, "SD-1"))

	err := svc.UnlinkServiceDeskRequest(t.Context(), 1, "SD-9")
	require.ErrorIs(t, err, service.ErrServiceDeskRequestLinkToDeleteNotFound)
	require.ErrorContains(t, err, "todo_id=1, key=SD-9")

	err = svc.UnlinkServiceDeskRequest(t.Context(), 1, "SD-2")
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestLinkToDeleteNotFound)
}
