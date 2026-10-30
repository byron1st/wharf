package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/system"
	"github.com/byron1st/wharf/internal/config"
)

var (
	ErrInvalidAtlassianSettings = errors.New("service: invalid atlassian settings")
	ErrInvalidConfigFile        = errors.New("service: invalid config file")
	ErrConfigChoiceCanceled     = errors.New("service: config file choice was canceled")
	ErrUnsupportedOS            = errors.New("service: not supported on this operating system")
)

// AtlassianSettings are the Atlassian site and credentials. All empty means
// Atlassian is not configured.
type AtlassianSettings struct {
	SiteURL  string
	Email    string
	APIToken string
}

// AppSettings describe the running server.
type AppSettings struct {
	// ConfigPath is the config file the server was started with.
	ConfigPath string
	// PendingConfigPath is a config file chosen since then, used from the next
	// restart, or empty when none is chosen.
	PendingConfigPath string
	Addr              string
	DBPath            string
	Version           string
}

type Settings interface {
	GetAtlassianSettings(ctx context.Context) AtlassianSettings
	SaveAtlassianSettings(ctx context.Context, a AtlassianSettings) error
	CheckAtlassianSettings(ctx context.Context, a AtlassianSettings) (time.Time, error)
	GetAppSettings(ctx context.Context) AppSettings
	ChooseConfigFile(ctx context.Context) (string, error)
	ClearPendingConfigFile(ctx context.Context)
	OpenConfigFile(ctx context.Context) error
	RestartServer(ctx context.Context) error
}

// SettingsDeps are what NewSettings needs.
type SettingsDeps struct {
	Jira      atlassian.Client
	System    system.Desktop
	App       AppSettings
	Atlassian AtlassianSettings
	// Restart stops the server and starts it again with configPath. It
	// returns once the stop is requested; the response is still sent.
	Restart func(configPath string)
}

type settings struct {
	jira    atlassian.Client
	system  system.Desktop
	restart func(configPath string)

	mu        sync.Mutex
	app       AppSettings
	atlassian AtlassianSettings
}

// NewSettings returns the Settings of a server started with deps.App and
// deps.Atlassian.
func NewSettings(deps SettingsDeps) Settings {
	return &settings{
		jira:      deps.Jira,
		system:    deps.System,
		restart:   deps.Restart,
		app:       deps.App,
		atlassian: deps.Atlassian,
	}
}

// GetAtlassianSettings returns the saved Atlassian settings, token included,
// so the settings screen can show it.
func (s *settings) GetAtlassianSettings(context.Context) AtlassianSettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.atlassian
}

// SaveAtlassianSettings writes a to the config file the server was started
// with and makes later syncs use it. It fails with
// ErrInvalidAtlassianSettings when a is incomplete or its site URL is not an
// https URL.
func (s *settings) SaveAtlassianSettings(_ context.Context, a AtlassianSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := config.SaveAtlassian(s.app.ConfigPath, a.SiteURL, a.Email, a.APIToken)
	if errors.Is(err, config.ErrMissingConfigKey) || errors.Is(err, config.ErrInvalidAtlassianSiteURL) {
		return errors.Join(ErrInvalidAtlassianSettings, err)
	}

	if err != nil {
		return err
	}

	s.atlassian = a
	s.jira.Configure(a.SiteURL, a.Email, a.APIToken)

	return nil
}

// CheckAtlassianSettings reports whether Jira and Service Desk accept a,
// saved or not, and returns when it checked. It fails with
// ErrAtlassianNotConfigured when a has no site URL.
func (s *settings) CheckAtlassianSettings(ctx context.Context, a AtlassianSettings) (time.Time, error) {
	if err := s.jira.Check(ctx, a.SiteURL, a.Email, a.APIToken); err != nil {
		return time.Time{}, classifyAtlassianError(err)
	}

	return time.Now().UTC(), nil
}

// GetAppSettings returns the running server's settings.
func (s *settings) GetAppSettings(context.Context) AppSettings {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.app
}

// ChooseConfigFile asks the user for a config file with the system file
// dialog and keeps it as the pending config path. It fails with
// ErrConfigChoiceCanceled when the dialog is canceled and with
// ErrUnsupportedOS off macOS.
func (s *settings) ChooseConfigFile(ctx context.Context) (string, error) {
	path, err := s.system.ChooseTOMLFile(ctx)
	if err != nil {
		return "", classifySystemError(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.app.PendingConfigPath = path
	if path == s.app.ConfigPath {
		s.app.PendingConfigPath = ""
	}

	return path, nil
}

// ClearPendingConfigFile forgets the pending config path.
func (s *settings) ClearPendingConfigFile(context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.app.PendingConfigPath = ""
}

// OpenConfigFile opens the pending config file, or the current one, with its
// default app. It fails with ErrUnsupportedOS off macOS.
func (s *settings) OpenConfigFile(ctx context.Context) error {
	s.mu.Lock()
	path := s.nextConfigPath()
	s.mu.Unlock()

	return classifySystemError(s.system.Open(ctx, path))
}

// RestartServer restarts the server with the pending config file, or the
// current one. It fails with ErrInvalidConfigFile, without restarting, when
// that file does not load.
func (s *settings) RestartServer(context.Context) error {
	s.mu.Lock()
	path := s.nextConfigPath()
	s.mu.Unlock()

	if _, err := config.Load(path); err != nil {
		return errors.Join(ErrInvalidConfigFile, err)
	}

	s.restart(path)

	return nil
}

func (s *settings) nextConfigPath() string {
	if s.app.PendingConfigPath != "" {
		return s.app.PendingConfigPath
	}

	return s.app.ConfigPath
}

func classifySystemError(err error) error {
	if errors.Is(err, system.ErrFileChoiceCanceled) {
		return errors.Join(ErrConfigChoiceCanceled, err)
	}

	if errors.Is(err, system.ErrUnsupportedOS) {
		return errors.Join(ErrUnsupportedOS, err)
	}

	return err
}
