package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/system"
	"github.com/byron1st/wharf/internal/config"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/service"
)

var saved = service.AtlassianSettings{
	SiteURL:  "https://example.atlassian.net",
	Email:    "me@example.com",
	APIToken: "secret-token",
}

type settingsFixture struct {
	settings  service.Settings
	jira      *mocks.MockClient
	desktop   *mocks.MockDesktop
	path      string
	restarted []string
}

func newSettings(t *testing.T) *settingsFixture {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("db_path = \"/data/w.db\"\n"), 0o600))

	f := &settingsFixture{jira: mocks.NewMockClient(t), desktop: mocks.NewMockDesktop(t), path: path}
	f.settings = service.NewSettings(service.SettingsDeps{
		Jira:      f.jira,
		System:    f.desktop,
		App:       service.AppSettings{ConfigPath: path, Addr: ":8080", DBPath: "/data/w.db", Version: "v1"},
		Atlassian: saved,
		Restart:   func(configPath string) { f.restarted = append(f.restarted, configPath) },
	})

	return f
}

func TestGetSettings(t *testing.T) {
	f := newSettings(t)

	require.Equal(t, saved, f.settings.GetAtlassianSettings(t.Context()))
	require.Equal(t, service.AppSettings{
		ConfigPath: f.path, Addr: ":8080", DBPath: "/data/w.db", Version: "v1",
	}, f.settings.GetAppSettings(t.Context()))
}

func TestSaveAtlassianSettings(t *testing.T) {
	f := newSettings(t)
	next := service.AtlassianSettings{SiteURL: "https://new.atlassian.net", Email: "new@example.com", APIToken: "new"}
	f.jira.EXPECT().Configure(next.SiteURL, next.Email, next.APIToken).Once()

	require.NoError(t, f.settings.SaveAtlassianSettings(t.Context(), next))
	require.Equal(t, next, f.settings.GetAtlassianSettings(t.Context()))

	cfg, err := config.Load(f.path)
	require.NoError(t, err)
	require.Equal(t, "/data/w.db", cfg.DBPath)
	require.Equal(t, next.SiteURL, cfg.AtlassianSiteURL)
	require.Equal(t, next.APIToken, cfg.AtlassianAPIToken)
}

func TestSaveAtlassianSettingsRejectsInvalid(t *testing.T) {
	// No Configure expectation: the client keeps its settings.
	f := newSettings(t)

	for _, a := range []service.AtlassianSettings{
		{SiteURL: "http://example.atlassian.net", Email: "me@example.com", APIToken: "t"},
		{SiteURL: "https://example.atlassian.net", APIToken: "t"},
	} {
		err := f.settings.SaveAtlassianSettings(t.Context(), a)
		require.ErrorIs(t, err, service.ErrInvalidAtlassianSettings)
	}

	require.Equal(t, saved, f.settings.GetAtlassianSettings(t.Context()))

	data, err := os.ReadFile(f.path)
	require.NoError(t, err)
	require.Equal(t, "db_path = \"/data/w.db\"\n", string(data))
}

func TestSaveAtlassianSettingsWriteFailure(t *testing.T) {
	f := newSettings(t)
	require.NoError(t, os.Remove(f.path))
	require.NoError(t, os.Mkdir(f.path, 0o700))

	err := f.settings.SaveAtlassianSettings(t.Context(), saved)
	require.ErrorIs(t, err, config.ErrFailedToReadConfigFileToSave)
	require.NotErrorIs(t, err, service.ErrInvalidAtlassianSettings)
}

func TestCheckAtlassianSettings(t *testing.T) {
	f := newSettings(t)
	f.jira.EXPECT().Check(mock.Anything, "https://x.atlassian.net", "a@b.c", "t").Return(nil).Once()
	f.jira.EXPECT().
		Check(mock.Anything, "https://x.atlassian.net", "a@b.c", "bad").
		Return(statusErr(t, 401)).
		Once()

	before := time.Now()

	checkedAt, err := f.settings.CheckAtlassianSettings(t.Context(), service.AtlassianSettings{
		SiteURL: "https://x.atlassian.net", Email: "a@b.c", APIToken: "t",
	})
	require.NoError(t, err)
	require.WithinRange(t, checkedAt, before.UTC(), time.Now().UTC())

	_, err = f.settings.CheckAtlassianSettings(t.Context(), service.AtlassianSettings{
		SiteURL: "https://x.atlassian.net", Email: "a@b.c", APIToken: "bad",
	})
	require.ErrorIs(t, err, service.ErrAtlassianUnauthorized)
}

func TestCheckAtlassianSettingsRejectsInvalidWithoutSending(t *testing.T) {
	// No Check expectation: the mock fails the test if anything is sent.
	f := newSettings(t)

	for _, a := range []service.AtlassianSettings{
		{},
		{SiteURL: "http://x.atlassian.net", Email: "a@b.c", APIToken: "t"},
		{SiteURL: "https://user@x.atlassian.net", Email: "a@b.c", APIToken: "t"},
		{SiteURL: "https://x.atlassian.net", APIToken: "t"},
		{SiteURL: "https://x.atlassian.net", Email: "a@b.c"},
	} {
		_, err := f.settings.CheckAtlassianSettings(t.Context(), a)
		require.ErrorIs(t, err, service.ErrInvalidAtlassianSettingsToCheck, "%+v", a)
		require.NotErrorIs(t, err, service.ErrInvalidAtlassianSettings)
	}
}

func TestChooseConfigFile(t *testing.T) {
	f := newSettings(t)
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("/other/config.toml", nil).Once()

	path, err := f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)
	require.Equal(t, "/other/config.toml", path)
	require.Equal(t, "/other/config.toml", f.settings.GetAppSettings(t.Context()).PendingConfigPath)

	f.settings.ClearPendingConfigFile(t.Context())
	require.Empty(t, f.settings.GetAppSettings(t.Context()).PendingConfigPath)

	// Choosing the current file leaves nothing pending.
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("/other/config.toml", nil).Once()
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return(f.path, nil).Once()

	_, err = f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)

	_, err = f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)
	require.Empty(t, f.settings.GetAppSettings(t.Context()).PendingConfigPath)
}

func TestChooseConfigFileFailures(t *testing.T) {
	failure := errors.New("osascript crashed")

	f := newSettings(t)
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("/other/config.toml", nil).Once()
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("", system.ErrFileChoiceCanceled).Once()
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("", system.ErrUnsupportedOS).Once()
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("", failure).Once()

	_, err := f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)

	_, err = f.settings.ChooseConfigFile(t.Context())
	require.ErrorIs(t, err, service.ErrConfigChoiceCanceled)

	_, err = f.settings.ChooseConfigFile(t.Context())
	require.ErrorIs(t, err, service.ErrUnsupportedOS)

	_, err = f.settings.ChooseConfigFile(t.Context())
	require.ErrorIs(t, err, failure)

	// A failed choice keeps the earlier pending path.
	require.Equal(t, "/other/config.toml", f.settings.GetAppSettings(t.Context()).PendingConfigPath)
}

func TestOpenConfigFile(t *testing.T) {
	f := newSettings(t)
	f.desktop.EXPECT().Open(mock.Anything, f.path).Return(nil).Once()
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return("/other/config.toml", nil).Once()
	f.desktop.EXPECT().Open(mock.Anything, "/other/config.toml").Return(system.ErrUnsupportedOS).Once()

	require.NoError(t, f.settings.OpenConfigFile(t.Context()))

	_, err := f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)
	require.ErrorIs(t, f.settings.OpenConfigFile(t.Context()), service.ErrUnsupportedOS)
}

func TestRestartServer(t *testing.T) {
	f := newSettings(t)

	require.NoError(t, f.settings.RestartServer(t.Context()))

	other := filepath.Join(t.TempDir(), "other.toml")
	require.NoError(t, os.WriteFile(other, []byte("addr = \":9090\"\ndb_path = \"/data/w.db\"\n"), 0o600))
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return(other, nil).Once()

	_, err := f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)
	require.NoError(t, f.settings.RestartServer(t.Context()))
	require.Equal(t, []string{f.path, other}, f.restarted)
}

func TestRestartServerRejectsInvalidConfig(t *testing.T) {
	f := newSettings(t)

	bad := filepath.Join(t.TempDir(), "bad.toml")
	require.NoError(t, os.WriteFile(bad, []byte("adr = 1\n"), 0o600))
	f.desktop.EXPECT().ChooseTOMLFile(mock.Anything).Return(bad, nil).Once()

	_, err := f.settings.ChooseConfigFile(t.Context())
	require.NoError(t, err)

	err = f.settings.RestartServer(t.Context())
	require.ErrorIs(t, err, service.ErrInvalidConfigFile)
	require.ErrorIs(t, err, config.ErrUnknownConfigKeys)
	require.Empty(t, f.restarted)
}
