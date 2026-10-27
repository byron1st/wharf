package service_test

import (
	"context"
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

func TestSyncServiceDeskRequests(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(requests, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, service.SyncResult{Upserted: 1}, result)
}

func TestSyncServiceDeskRequestsStoresPartialResult(t *testing.T) {
	partial := errors.New("missing title: key=SD-2")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(requests, partial).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, result.Upserted)
	require.ErrorIs(t, result.Partial, partial)
}

func TestSyncServiceDeskRequestsSkipsUpsertWhenFetchFails(t *testing.T) {
	failure := errors.New("page failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(nil, failure).Once()

	manager := mocks.NewMockManager(t)

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.ErrorIs(t, err, failure)
	require.Equal(t, service.SyncResult{}, result)
}

func TestSyncServiceDeskRequestsEmptyKeepsStoredRows(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return([]servicedesk.Request{}, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request{}).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, service.SyncResult{Upserted: 0}, result)
}

func TestSyncServiceDeskRequestsAllDropped(t *testing.T) {
	partial := errors.New("missing title: key=SD-1")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return([]servicedesk.Request{}, partial).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, []servicedesk.Request{}).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, result.Upserted)
	require.ErrorIs(t, result.Partial, partial)
}

func TestSyncServiceDeskRequestsUpsertFailureDropsPartial(t *testing.T) {
	partial := errors.New("missing title: key=SD-2")
	failure := errors.New("upsert failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(requests, partial).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(failure).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, partial)
	require.Equal(t, service.SyncResult{}, result)
}

func TestSyncServiceDeskRequestsReturnsUpsertFailure(t *testing.T) {
	failure := errors.New("upsert failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(requests, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertServiceDeskRequests(mock.Anything, requests).Return(failure).Once()

	result, err := service.NewService(jira, manager).SyncServiceDeskRequests(t.Context())
	require.ErrorIs(t, err, failure)
	require.Equal(t, service.SyncResult{}, result)
}

func TestSyncServiceDeskRequestsCanceled(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetOwnedRequests(mock.Anything).Return(nil, context.Canceled).Once()

	result, err := service.NewService(jira, mocks.NewMockManager(t)).SyncServiceDeskRequests(t.Context())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, service.SyncResult{}, result)
}

func TestLinkServiceDeskRequest(t *testing.T) {
	manager := mocks.NewMockManager(t)
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, "local-WHF-001", "SD-1").Return(nil).Once()

	err := service.NewService(mocks.NewMockClient(t), manager).
		LinkServiceDeskRequest(t.Context(), "local-WHF-001", "SD-1")
	require.NoError(t, err)
}

func TestLinkServiceDeskRequestFailures(t *testing.T) {
	failure := errors.New("insert failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, "T9", "SD-1").Return(db.ErrTaskToLinkNotFound).Once()
	manager.EXPECT().
		LinkServiceDeskRequest(mock.Anything, "T1", "SD-9").
		Return(db.ErrServiceDeskRequestToLinkNotFound).
		Once()
	manager.EXPECT().LinkServiceDeskRequest(mock.Anything, "T1", "SD-1").Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	err := svc.LinkServiceDeskRequest(t.Context(), "T9", "SD-1")
	require.ErrorIs(t, err, service.ErrTaskToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
	require.ErrorContains(t, err, "task_id=T9")

	err = svc.LinkServiceDeskRequest(t.Context(), "T1", "SD-9")
	require.ErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrTaskToLinkNotFound)
	require.ErrorContains(t, err, "key=SD-9")

	err = svc.LinkServiceDeskRequest(t.Context(), "T1", "SD-1")
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrTaskToLinkNotFound)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestToLinkNotFound)
}

func TestUnlinkServiceDeskRequest(t *testing.T) {
	failure := errors.New("delete failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UnlinkServiceDeskRequest(mock.Anything, "T1", "SD-1").Return(nil).Once()
	manager.EXPECT().
		UnlinkServiceDeskRequest(mock.Anything, "T1", "SD-9").
		Return(db.ErrServiceDeskRequestLinkToDeleteNotFound).
		Once()
	manager.EXPECT().UnlinkServiceDeskRequest(mock.Anything, "T1", "SD-2").Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	require.NoError(t, svc.UnlinkServiceDeskRequest(t.Context(), "T1", "SD-1"))

	err := svc.UnlinkServiceDeskRequest(t.Context(), "T1", "SD-9")
	require.ErrorIs(t, err, service.ErrServiceDeskRequestLinkToDeleteNotFound)
	require.ErrorContains(t, err, "task_id=T1, key=SD-9")

	err = svc.UnlinkServiceDeskRequest(t.Context(), "T1", "SD-2")
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrServiceDeskRequestLinkToDeleteNotFound)
}
