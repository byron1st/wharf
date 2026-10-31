package system_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/system"
)

type call struct {
	name string
	args []string
}

func recorder(out []byte, err error) (*[]call, func(context.Context, string, ...string) ([]byte, error)) {
	var calls []call

	return &calls, func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name: name, args: args})

		return out, err
	}
}

func TestUnsupportedOS(t *testing.T) {
	calls, run := recorder(nil, nil)
	c := system.NewDesktopWith("linux", run)

	_, err := c.ChooseTOMLFile(t.Context())
	require.ErrorIs(t, err, system.ErrUnsupportedOS)
	require.ErrorIs(t, c.Open(t.Context(), "/tmp/config.toml"), system.ErrUnsupportedOS)
	require.Empty(t, *calls)
}

func TestChooseTOMLFile(t *testing.T) {
	calls, run := recorder([]byte("/Users/me/dotfiles/wharf/config.toml\n"), nil)

	path, err := system.NewDesktopWith("darwin", run).ChooseTOMLFile(t.Context())
	require.NoError(t, err)
	require.Equal(t, "/Users/me/dotfiles/wharf/config.toml", path)
	require.Equal(t, []call{{
		name: "osascript",
		args: []string{"-e", `POSIX path of (choose file of type {"toml"} with prompt "Choose a Wharf config file")`},
	}}, *calls)
}

func TestChooseTOMLFileFailures(t *testing.T) {
	canceled := &exec.ExitError{Stderr: []byte("execution error: User canceled. (-128)")}
	failure := errors.New("osascript missing")

	for name, tc := range map[string]struct {
		out  []byte
		err  error
		want error
	}{
		"canceled": {err: canceled, want: system.ErrFileChoiceCanceled},
		"other":    {err: failure, want: system.ErrFailedToChooseFile},
		"empty":    {out: []byte(" \n"), want: system.ErrEmptyChosenFilePath},
	} {
		t.Run(name, func(t *testing.T) {
			_, run := recorder(tc.out, tc.err)

			_, err := system.NewDesktopWith("darwin", run).ChooseTOMLFile(t.Context())
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestOpen(t *testing.T) {
	calls, run := recorder(nil, nil)

	require.NoError(t, system.NewDesktopWith("darwin", run).Open(t.Context(), "-weird.toml"))
	require.Equal(t, []call{{name: "open", args: []string{"--", "-weird.toml"}}}, *calls)

	failure := errors.New("no app")
	_, run = recorder(nil, failure)

	err := system.NewDesktopWith("darwin", run).Open(t.Context(), "/tmp/c.toml")
	require.ErrorIs(t, err, system.ErrFailedToOpenFile)
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "path=/tmp/c.toml")
}
