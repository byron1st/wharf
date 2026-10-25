package task_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/task"
)

func TestNewID(t *testing.T) {
	require.Equal(t, "jira-BLC-000", task.NewID(task.OriginJira, "BLC-000"))
	require.Equal(t, "local-WHF-001", task.NewID(task.OriginLocal, task.LocalKey(1)))
}

func TestLocalKey(t *testing.T) {
	require.Equal(t, "WHF-001", task.LocalKey(1))
	require.Equal(t, "WHF-999", task.LocalKey(999))
	require.Equal(t, "WHF-1000", task.LocalKey(1000))
}
