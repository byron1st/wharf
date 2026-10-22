package tool_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/tool"
)

const fakeScript = `#!/bin/sh
case "$1" in
  echo) shift; echo "$@" ;;
  cwd) cat marker.txt ;;
  warn) echo "warning" >&2; echo "ok" ;;
  fail) echo "boom" >&2; exit 3 ;;
esac
`

func writeScript(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "fake.sh")
	require.NoError(t, os.WriteFile(path, []byte(fakeScript), 0o755))

	return path
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	execCmd := writeScript(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("in-cwd"), 0o644))

	tl, err := tool.NewTool(execCmd)
	require.NoError(t, err)

	t.Run("returns stdout with cmd and args passed", func(t *testing.T) {
		out, err := tl.Run(tool.RunOption{Cmd: "echo", Args: []string{"a", "b"}, Cwd: dir})
		require.NoError(t, err)
		require.Equal(t, "a b\n", out)
	})

	t.Run("runs in cwd", func(t *testing.T) {
		out, err := tl.Run(tool.RunOption{Cmd: "cwd", Cwd: dir})
		require.NoError(t, err)
		require.Equal(t, "in-cwd", out)
	})

	t.Run("ignores stderr on success", func(t *testing.T) {
		out, err := tl.Run(tool.RunOption{Cmd: "warn", Cwd: dir})
		require.NoError(t, err)
		require.Equal(t, "ok\n", out)
	})

	t.Run("returns error with stderr on non-zero exit", func(t *testing.T) {
		out, err := tl.Run(tool.RunOption{Cmd: "fail", Cwd: dir})
		require.ErrorIs(t, err, tool.ErrFailedToExecuteCmd)
		require.ErrorContains(t, err, "stderr=boom")
		require.Empty(t, out)

		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 3, exitErr.ExitCode())
	})
}

func TestRunResolvesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "marker.txt"), []byte("in-sub"), 0o644))
	t.Chdir(dir)

	tl, err := tool.NewTool("fake.sh")
	require.NoError(t, err)

	out, err := tl.Run(tool.RunOption{Cmd: "cwd", Cwd: "sub"})
	require.NoError(t, err)
	require.Equal(t, "in-sub", out)
}

func TestRunMissingExecCmd(t *testing.T) {
	tl, err := tool.NewTool(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)

	_, err = tl.Run(tool.RunOption{Cmd: "echo", Cwd: t.TempDir()})
	require.ErrorIs(t, err, tool.ErrFailedToExecuteCmd)

	var exitErr *exec.ExitError
	require.False(t, errors.As(err, &exitErr))
}
