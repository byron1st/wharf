package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
