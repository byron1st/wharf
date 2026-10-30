package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/scripts"
)

func TestAtlassianSyncs(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	ctx := context.Background()

	_, err := manager.GetLastAtlassianSync(ctx)
	require.ErrorIs(t, err, db.ErrAtlassianSyncNotFound)

	first := atlassiansync.Record{SyncedAt: time.Date(2026, 10, 26, 5, 2, 0, 0, time.UTC), Issues: 6, OpenRequests: 4}
	second := atlassiansync.Record{
		SyncedAt:        time.Date(2026, 10, 26, 6, 0, 0, 0, time.UTC),
		Issues:          5,
		IssueFailures:   1,
		OpenRequests:    3,
		ClosedRequests:  2,
		RequestFailures: 1,
	}

	require.NoError(t, manager.CreateAtlassianSync(ctx, first))
	require.NoError(t, manager.CreateAtlassianSync(ctx, second))

	got, err := manager.GetLastAtlassianSync(ctx)
	require.NoError(t, err)
	require.True(t, second.SyncedAt.Equal(got.SyncedAt))

	got.SyncedAt = second.SyncedAt
	require.Equal(t, second, got)
}

func TestAtlassianSyncsCanceled(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.CreateAtlassianSync(ctx, atlassiansync.Record{SyncedAt: time.Now()})
	require.ErrorIs(t, err, db.ErrFailedToCreateAtlassianSync)
	require.ErrorIs(t, err, context.Canceled)

	_, err = manager.GetLastAtlassianSync(ctx)
	require.ErrorIs(t, err, db.ErrFailedToGetAtlassianSync)

	_, err = manager.ListServiceDeskRequests(ctx, false)
	require.ErrorIs(t, err, db.ErrFailedToListServiceDeskRequests)

	_, err = manager.CountTodosByTask(ctx)
	require.ErrorIs(t, err, db.ErrFailedToCountTodosByTask)
}
