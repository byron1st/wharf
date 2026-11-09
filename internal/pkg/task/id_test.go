package task_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/task"
)

func TestNewID(t *testing.T) {
	require.Equal(t, "jira-WHF-1", task.NewID(task.OriginJira, "WHF-1"))
	require.Equal(t, "local-WHF-001", task.NewID(task.OriginLocal, "WHF-001"))
}

func TestLocalKey(t *testing.T) {
	for n, want := range map[int64]string{1: "WHF-001", 42: "WHF-042", 999: "WHF-999", 1000: "WHF-1000"} {
		require.Equal(t, want, task.LocalKey(n))
	}
}
