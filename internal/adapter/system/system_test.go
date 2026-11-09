package system_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func TestNotify(t *testing.T) {
	calls, run := recorder(nil, nil)

	require.NoError(t, system.NewDesktopWith("darwin", run).Notify(t.Context(), "Wharf", `"0.2.0" is out`))
	require.Len(t, *calls, 1)
	require.Equal(t, "osascript", (*calls)[0].name)
	// Title and message are passed as arguments, never spliced into the script.
	args := (*calls)[0].args
	require.Equal(t, []string{"Wharf", `"0.2.0" is out`}, args[len(args)-2:])

	failure := errors.New("osascript failed")
	_, run = recorder(nil, failure)

	err := system.NewDesktopWith("darwin", run).Notify(t.Context(), "Wharf", "m")
	require.ErrorIs(t, err, system.ErrFailedToNotify)
	require.ErrorIs(t, err, failure)

	require.ErrorIs(t, system.NewDesktopWith("linux", run).Notify(t.Context(), "Wharf", "m"), system.ErrUnsupportedOS)
}

func TestReplaceExecutable(t *testing.T) {
	t.Run("swaps in the new binary and keeps the old one", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "wharf")
		require.NoError(t, os.WriteFile(exe, []byte("old"), 0o755))
		// A backup left by an earlier update is replaced.
		require.NoError(t, os.WriteFile(exe+".old", []byte("older"), 0o755))
		before, err := os.Stat(exe)
		require.NoError(t, err)

		d := system.NewDesktopFor("darwin", nil, exe, dir, 501)
		require.NoError(t, d.ReplaceExecutable(t.Context(), []byte("new")))

		got, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, "new", string(got))

		info, err := os.Stat(exe)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		// A rename leaves a new file, not the old one rewritten in place.
		require.False(t, os.SameFile(before, info))

		old, err := os.ReadFile(exe + ".old")
		require.NoError(t, err)
		require.Equal(t, "old", string(old))

		oldInfo, err := os.Stat(exe + ".old")
		require.NoError(t, err)
		require.True(t, os.SameFile(before, oldInfo))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 2)
	})

	t.Run("replaces the target of a symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "wharf")
		link := filepath.Join(dir, "link")

		require.NoError(t, os.WriteFile(target, []byte("old"), 0o755))
		require.NoError(t, os.Symlink(target, link))

		require.NoError(
			t,
			system.NewDesktopFor("darwin", nil, link, dir, 501).ReplaceExecutable(t.Context(), []byte("new")),
		)

		got, err := os.ReadFile(link)
		require.NoError(t, err)
		require.Equal(t, "new", string(got))
		require.FileExists(t, target+".old")
	})

	t.Run("leaves the executable when it cannot be found", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "missing")

		err := system.NewDesktopFor("darwin", nil, exe, "", 501).ReplaceExecutable(t.Context(), []byte("new"))
		require.ErrorIs(t, err, system.ErrFailedToFindExecutableToReplace)
	})

	t.Run("leaves the executable when it cannot be backed up", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "wharf")
		// A directory cannot be hard-linked, so the backup fails first.
		require.NoError(t, os.MkdirAll(filepath.Join(exe, "keep"), 0o755))

		err := system.NewDesktopFor("darwin", nil, exe, dir, 501).ReplaceExecutable(t.Context(), []byte("new"))
		require.ErrorIs(t, err, system.ErrFailedToBackUpExecutable)
		require.DirExists(t, filepath.Join(exe, "keep"))
	})
}

func TestInstallLaunchAgent(t *testing.T) {
	home := t.TempDir()
	calls, run := recorder(nil, nil)
	d := system.NewDesktopFor("darwin", run, "/opt/wharf & co/wharf", home, 501)

	require.NoError(t, d.InstallLaunchAgent(t.Context(), "/etc/wharf.toml"))

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.byron1st.wharf.plist")
	plist, err := os.ReadFile(plistPath)
	require.NoError(t, err)
	require.Contains(t, string(plist), "<string>com.byron1st.wharf</string>")
	require.Contains(t, string(plist), "<string>/opt/wharf &amp; co/wharf</string>")
	require.Contains(t, string(plist), "<key>WHARF_CONFIG</key>\n\t\t<string>/etc/wharf.toml</string>")
	require.Contains(t, string(plist), "<key>SuccessfulExit</key>\n\t\t<false/>")
	require.Contains(t, string(plist), filepath.Join(home, "Library", "Logs", "wharf", "wharf.log"))
	require.DirExists(t, filepath.Join(home, "Library", "Logs", "wharf"))
	require.Equal(t, []call{
		{name: "launchctl", args: []string{"bootout", "gui/501/com.byron1st.wharf"}},
		{name: "launchctl", args: []string{"bootstrap", "gui/501", plistPath}},
	}, *calls)
}

func TestInstallLaunchAgentWithoutConfigPath(t *testing.T) {
	home := t.TempDir()
	_, run := recorder(nil, nil)

	require.NoError(t, system.NewDesktopFor("darwin", run, "/bin/wharf", home, 501).InstallLaunchAgent(t.Context(), ""))

	plist, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", "com.byron1st.wharf.plist"))
	require.NoError(t, err)
	require.NotContains(t, string(plist), "WHARF_CONFIG")
}

func TestInstallLaunchAgentFailsToBootstrap(t *testing.T) {
	failure := errors.New("bootstrap failed")
	_, run := recorder(nil, failure)

	err := system.NewDesktopFor("darwin", run, "/bin/wharf", t.TempDir(), 501).InstallLaunchAgent(t.Context(), "")
	require.ErrorIs(t, err, system.ErrFailedToBootstrapLaunchAgent)
	require.ErrorIs(t, err, failure)
}

func TestUninstallLaunchAgent(t *testing.T) {
	home := t.TempDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.byron1st.wharf.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(plistPath), 0o755))
	require.NoError(t, os.WriteFile(plistPath, []byte("plist"), 0o644))

	// bootout failing means the agent is not loaded, which is fine.
	calls, run := recorder(nil, errors.New("not loaded"))
	d := system.NewDesktopFor("darwin", run, "/bin/wharf", home, 501)

	require.NoError(t, d.UninstallLaunchAgent(t.Context()))
	require.NoFileExists(t, plistPath)
	require.Equal(t, []call{{name: "launchctl", args: []string{"bootout", "gui/501/com.byron1st.wharf"}}}, *calls)

	// A second uninstall finds nothing to remove.
	require.NoError(t, d.UninstallLaunchAgent(t.Context()))
}

func TestLaunchAgentRequiresMacOS(t *testing.T) {
	calls, run := recorder(nil, nil)
	d := system.NewDesktopFor("linux", run, "/bin/wharf", t.TempDir(), 501)

	require.ErrorIs(t, d.InstallLaunchAgent(t.Context(), ""), system.ErrUnsupportedOS)
	require.ErrorIs(t, d.UninstallLaunchAgent(t.Context()), system.ErrUnsupportedOS)
	require.Empty(t, *calls)
}
