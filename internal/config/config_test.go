package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

// atlassianKeys is the minimal valid config: the required Atlassian keys.
const atlassianKeys = "atlassian_site_url = \"https://example.atlassian.net\"\n" +
	"atlassian_email = \"me@example.com\"\n" +
	"atlassian_api_token = \"secret-token\"\n"

func TestLoad(t *testing.T) {
	isolateUserConfigDir(t)

	t.Run("fills defaults, creates db dir, and reads atlassian keys", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), atlassianKeys, 0o644)

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, config.Config{
			Addr:              "127.0.0.1:8080",
			DBPath:            filepath.Join(dir, "wharf", "wharf.db"),
			AtlassianSiteURL:  "https://example.atlassian.net",
			AtlassianEmail:    "me@example.com",
			AtlassianAPIToken: "secret-token",
		}, cfg)
		require.DirExists(t, filepath.Join(dir, "wharf"))
	})

	t.Run("does not create default db dir when db_path is set", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		path := writeFile(
			t,
			filepath.Join(t.TempDir(), "config.toml"),
			atlassianKeys+"db_path = \"/data/w.db\"\n",
			0o644,
		)

		_, err := config.Load(path)
		require.NoError(t, err)
		require.NoDirExists(t, filepath.Join(dir, "wharf"))
	})

	t.Run("fails when default db dir cannot be created", func(t *testing.T) {
		dir := isolateUserConfigDir(t)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		writeFile(t, filepath.Join(dir, "wharf"), "", 0o644)
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), atlassianKeys, 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToCreateDBDir)
	})

	t.Run("uses configured values", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"),
			"addr = \"127.0.0.1:9090\"\ndb_path = \"/data/w.db\"\n"+
				"atlassian_site_url = \"https://api.atlassian.com/ex/jira/abc-123\"\n"+
				"atlassian_email = \"me@example.com\"\natlassian_api_token = \"secret-token\"\n", 0o644)

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, config.Config{
			Addr:              "127.0.0.1:9090",
			DBPath:            "/data/w.db",
			AtlassianSiteURL:  "https://api.atlassian.com/ex/jira/abc-123",
			AtlassianEmail:    "me@example.com",
			AtlassianAPIToken: "secret-token",
		}, cfg)
	})

	t.Run("uses defaults when file is missing", func(t *testing.T) {
		dir := isolateUserConfigDir(t)

		cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"))
		require.NoError(t, err)
		require.Equal(t, config.Config{Addr: "127.0.0.1:8080", DBPath: filepath.Join(dir, "wharf", "wharf.db")}, cfg)
	})

	t.Run("allows no atlassian keys", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "db_path = \"/data/w.db\"\n", 0o644)

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, config.Config{Addr: "127.0.0.1:8080", DBPath: "/data/w.db"}, cfg)
	})

	t.Run("fails when file cannot be read", func(t *testing.T) {
		_, err := config.Load(t.TempDir())
		require.ErrorIs(t, err, config.ErrFailedToReadConfigFile)
	})

	t.Run("fails on invalid toml", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "addr = ", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrFailedToParseConfigFile)
	})

	t.Run("fails on unknown keys", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), atlassianKeys+"adr = \":8080\"\n", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
		require.ErrorContains(t, err, "adr")
	})

	t.Run("rejects the removed acli_path key", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), atlassianKeys+"acli_path = \"acli\"\n", 0o644)

		_, err := config.Load(path)
		require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
		require.ErrorContains(t, err, "acli_path")
	})

	for _, key := range []string{"atlassian_site_url", "atlassian_email", "atlassian_api_token"} {
		t.Run("fails when "+key+" is missing", func(t *testing.T) {
			var lines []string

			for line := range strings.Lines(atlassianKeys) {
				if !strings.HasPrefix(line, key+" ") {
					lines = append(lines, line)
				}
			}

			path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), strings.Join(lines, ""), 0o644)

			_, err := config.Load(path)
			require.ErrorIs(t, err, config.ErrMissingConfigKey)
			require.ErrorContains(t, err, "key="+key)
			require.NotContains(t, err.Error(), "secret-token")
		})
	}

	for _, siteURL := range []string{
		"example.atlassian.net",
		"ftp://example.atlassian.net",
		"https://",
		"/rest/api/3",
		"https://exa mple.atlassian.net",
		"http://example.atlassian.net",
		"https://example.atlassian.net?a=b",
		"https://example.atlassian.net?",
		"https://example.atlassian.net#top",
	} {
		t.Run("fails on invalid site url "+siteURL, func(t *testing.T) {
			content := strings.Replace(atlassianKeys, "https://example.atlassian.net", siteURL, 1)
			path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), content, 0o644)

			_, err := config.Load(path)
			require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
			require.NotErrorIs(t, err, config.ErrMissingConfigKey)
			require.NotContains(t, err.Error(), "secret-token")
		})
	}
}

func TestLoadInvalidSiteURLDoesNotLeakUserinfo(t *testing.T) {
	content := strings.Replace(
		atlassianKeys,
		"https://example.atlassian.net",
		"ftp://user:hunter2@example.atlassian.net",
		1,
	)
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), content, 0o644)

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
	require.NotContains(t, err.Error(), "hunter2")
}

func TestLoadRejectsSiteURLWithUserinfo(t *testing.T) {
	content := strings.Replace(
		atlassianKeys,
		"https://example.atlassian.net",
		"https://user:hunter2@example.atlassian.net",
		1,
	)
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), content, 0o644)

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
	require.NotContains(t, err.Error(), "hunter2")
}

func TestLoadInvalidSiteURLVariantsDoNotLeakUserinfo(t *testing.T) {
	for name, siteURL := range map[string]string{
		"parse failure with password": "https://user:hunter2@exa mple.atlassian.net",
		// url.URL.Redacted masks only the password, so a username-only
		// userinfo (e.g. an API token as the user) must be dropped as well.
		"username only":        "https://hunter2@example.atlassian.net",
		"empty host, password": "http://user:hunter2@",
	} {
		t.Run(name, func(t *testing.T) {
			content := strings.Replace(atlassianKeys, "https://example.atlassian.net", siteURL, 1)
			path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), content, 0o644)

			_, err := config.Load(path)
			require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
			require.NotContains(t, err.Error(), "hunter2")
		})
	}
}

func TestSaveAtlassian(t *testing.T) {
	isolateUserConfigDir(t)

	t.Run("replaces and appends only the atlassian lines", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"),
			"# my config\naddr = \":9090\"\n# atlassian_email = \"commented@example.com\"\n"+
				"atlassian_site_url = \"https://old.atlassian.net\" # old site\ndb_path = \"/data/w.db\"", 0o644)

		require.NoError(t, config.SaveAtlassian(path, "https://new.atlassian.net", "me@example.com", `to"ken`))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t,
			"# my config\naddr = \":9090\"\n# atlassian_email = \"commented@example.com\"\n"+
				"atlassian_site_url = \"https://new.atlassian.net\"\ndb_path = \"/data/w.db\"\n"+
				"atlassian_email = \"me@example.com\"\natlassian_api_token = \"to\\\"ken\"\n",
			string(data))

		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

		cfg, err := config.Load(path)
		require.NoError(t, err)
		require.Equal(t, ":9090", cfg.Addr)
		require.Equal(t, "https://new.atlassian.net", cfg.AtlassianSiteURL)
		require.Equal(t, "me@example.com", cfg.AtlassianEmail)
		require.Equal(t, `to"ken`, cfg.AtlassianAPIToken)
	})

	t.Run("creates a missing file and its directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "wharf", "config.toml")

		require.NoError(
			t,
			config.SaveAtlassian(path, "https://example.atlassian.net", "me@example.com", "secret-token"),
		)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, atlassianKeys, string(data))
	})

	t.Run("rejects invalid values and keeps the file", func(t *testing.T) {
		path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), atlassianKeys, 0o644)

		err := config.SaveAtlassian(path, "http://example.atlassian.net", "me@example.com", "new-secret")
		require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
		require.NotContains(t, err.Error(), "new-secret")

		err = config.SaveAtlassian(path, "https://example.atlassian.net", "", "new-secret")
		require.ErrorIs(t, err, config.ErrMissingConfigKey)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, atlassianKeys, string(data))
	})

	t.Run("keeps the file when it would not load afterwards", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, filepath.Join(dir, "config.toml"), "adr = \":8080\"\n", 0o644)

		err := config.SaveAtlassian(path, "https://example.atlassian.net", "me@example.com", "secret-token")
		require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
		require.ErrorContains(t, err, "path="+path)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "adr = \":8080\"\n", string(data))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "the temp file is removed")
	})

	t.Run("writes through a symlink and keeps it", func(t *testing.T) {
		target := writeFile(t, filepath.Join(t.TempDir(), "real.toml"), "# dotfiles\n", 0o600)
		link := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.Symlink(target, link))

		require.NoError(
			t,
			config.SaveAtlassian(link, "https://example.atlassian.net", "me@example.com", "secret-token"),
		)

		info, err := os.Lstat(link)
		require.NoError(t, err)
		require.Equal(t, os.ModeSymlink, info.Mode()&os.ModeSymlink)

		data, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "# dotfiles\n"+atlassianKeys, string(data))
	})

	t.Run("names the symlink, not its target, in errors", func(t *testing.T) {
		target := writeFile(t, filepath.Join(t.TempDir(), "real.toml"), "adr = \":8080\"\n", 0o600)
		link := filepath.Join(t.TempDir(), "config.toml")
		require.NoError(t, os.Symlink(target, link))

		err := config.SaveAtlassian(link, "https://example.atlassian.net", "me@example.com", "secret-token")
		require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
		require.ErrorContains(t, err, "path="+link)
		require.NotContains(t, err.Error(), target)
	})

	t.Run("fails when the file cannot be read", func(t *testing.T) {
		err := config.SaveAtlassian(t.TempDir(), "https://example.atlassian.net", "me@example.com", "secret-token")
		require.ErrorIs(t, err, config.ErrFailedToReadConfigFileToSave)
		require.NotErrorIs(t, err, config.ErrFailedToReadConfigFile)
	})
}

func TestValidateAtlassian(t *testing.T) {
	require.NoError(t, config.ValidateAtlassian("https://example.atlassian.net", "me@example.com", "secret-token"))
	require.NoError(t, config.ValidateAtlassian("", "", ""), "all empty is not configured, not invalid")

	err := config.ValidateAtlassian("http://example.atlassian.net", "me@example.com", "secret-token")
	require.ErrorIs(t, err, config.ErrInvalidAtlassianSiteURL)
	require.NotContains(t, err.Error(), "secret-token")

	err = config.ValidateAtlassian("https://example.atlassian.net", "", "secret-token")
	require.ErrorIs(t, err, config.ErrMissingConfigKey)
}
