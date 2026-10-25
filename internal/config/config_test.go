package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/config"
)

func writeFile(t *testing.T, path, content string, perm os.FileMode) string {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), perm))

	return path
}

// isolateUserConfigDir points os.UserConfigDir into a temp dir and returns it,
// so tests never touch the real user config dir.
func isolateUserConfigDir(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	dir, err := os.UserConfigDir()
	require.NoError(t, err)

	return dir
}

func TestPath(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv(config.PathEnv, "/custom/wharf.toml")

		path, err := config.Path()
		require.NoError(t, err)
		require.Equal(t, "/custom/wharf.toml", path)
	})

	t.Run("defaults to wharf/config.toml under user config dir", func(t *testing.T) {
		t.Setenv(config.PathEnv, "")

		dir, err := os.UserConfigDir()
		require.NoError(t, err)

		path, err := config.Path()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(dir, "wharf", "config.toml"), path)
	})
}

func TestLoad(t *testing.T) {
	isolateUserConfigDir(t)

	binDir := t.TempDir()
	pathACLI := writeFile(t, filepath.Join(binDir, "acli"), "#!/bin/sh\n", 0o755)
	t.Setenv("PATH", binDir)

	t.Run("fills defaults, creates db dir, and finds acli in PATH", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "", 0o644)

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, config.Config{
			Addr:     ":8080",
			DBPath:   filepath.Join(dir, "wharf", "wharf.db"),
			ACLIPath: pathACLI,
		}, cfg)
		require.DirExists(t, filepath.Join(dir, "wharf"))
	})

	t.Run("does not create default db dir when db_path is set", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "db_path = \"/data/w.db\"\n", 0o644)

		_, err := config.Load(path)
		require.NoError(t, err)
		require.NoDirExists(t, filepath.Join(dir, "wharf"))
	})

	t.Run("fails when default db dir cannot be created", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		writeFile(t, filepath.Join(dir, "wharf"), "", 0o644)
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToCreateDBDir)
	})

	t.Run("uses configured values", func(t *testing.T) {
		dir := t.TempDir()
		customACLI := writeFile(t, filepath.Join(dir, "my-acli"), "#!/bin/sh\n", 0o755)
		path := writeFile(t, filepath.Join(dir, "config.toml"),
			"addr = \"127.0.0.1:9090\"\ndb_path = \"/data/w.db\"\nacli_path = \""+customACLI+"\"\n", 0o644)

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, config.Config{Addr: "127.0.0.1:9090", DBPath: "/data/w.db", ACLIPath: customACLI}, cfg)
	})

	t.Run("fails when file is missing", func(t *testing.T) {
		_, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"))
		require.ErrorIs(t, err, config.ErrFailedToReadConfigFile)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("fails on invalid toml", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "addr = ", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToParseConfigFile)
	})

	t.Run("fails on unknown keys", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "adr = \":8080\"\n", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
		require.ErrorContains(t, err, "adr")
	})

	t.Run("fails when acli is not in PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToFindACLI)
	})

	t.Run("fails when configured acli is not executable", func(t *testing.T) {
		dir := t.TempDir()
		plain := writeFile(t, filepath.Join(dir, "acli"), "", 0o644)
		path := writeFile(t, filepath.Join(dir, "config.toml"), "acli_path = \""+plain+"\"\n", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToFindACLI)
	})
}
