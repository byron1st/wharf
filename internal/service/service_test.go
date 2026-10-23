package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/service"
)

var tasks = []task.Task{
	{ID: "WHF-1", Originated: task.OriginJira, Title: "first", Status: task.StatusTodo},
	{ID: "WHF-2", Originated: task.OriginJira, Title: "second", Status: task.StatusInProgress},
}

func TestSyncJiraTasks(t *testing.T) {
	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, nil).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertTasks(mock.Anything, tasks).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncJiraTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, service.SyncResult{Upserted: 2}, result)
}

func TestSyncJiraTasksStoresPartialResult(t *testing.T) {
	partial := errors.New("view failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, partial).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertTasks(mock.Anything, tasks).Return(nil).Once()

	result, err := service.NewService(jira, manager).SyncJiraTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, result.Upserted)
	require.ErrorIs(t, result.Partial, partial)
}

func TestSyncJiraTasksSkipsUpsertWhenJiraFails(t *testing.T) {
	failure := errors.New("search failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(nil, failure).Once()

	// No UpsertTasks expectation: the mock fails the test if it is called.
	manager := mocks.NewMockManager(t)

	result, err := service.NewService(jira, manager).SyncJiraTasks(t.Context())
	require.ErrorIs(t, err, failure)
	require.Equal(t, service.SyncResult{}, result)
}

func TestSyncJiraTasksReturnsUpsertFailure(t *testing.T) {
	failure := errors.New("upsert failed")

	jira := mocks.NewMockClient(t)
	jira.EXPECT().GetMyTasks(mock.Anything).Return(tasks, errors.New("view failed")).Once()

	manager := mocks.NewMockManager(t)
	manager.EXPECT().UpsertTasks(mock.Anything, tasks).Return(failure).Once()

	result, err := service.NewService(jira, manager).SyncJiraTasks(t.Context())
	require.ErrorIs(t, err, failure)
	require.Equal(t, service.SyncResult{}, result)
}
