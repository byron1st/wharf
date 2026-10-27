package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// PathEnv is the environment variable that overrides the config file path.
const PathEnv = "WHARF_CONFIG"

const (
	defaultAddr       = ":8080"
	defaultDBFileName = "wharf.db"
)

var (
	ErrFailedToFindUserConfigDir = errors.New("config: failed to find user config dir")
	ErrFailedToReadConfigFile    = errors.New("config: failed to read config file")
	ErrFailedToParseConfigFile   = errors.New("config: failed to parse config file")
	ErrUnknownConfigKeys         = errors.New("config: unknown config keys")
	ErrFailedToCreateDBDir       = errors.New("config: failed to create default db dir")
	ErrMissingConfigKey          = errors.New("config: missing required config key")
	ErrInvalidAtlassianSiteURL   = errors.New("config: invalid atlassian site url")
)

// Config holds the settings wharf reads from its TOML config file.
type Config struct {
	// Addr is the TCP address the server listens on.
	Addr string `toml:"addr"`
	// DBPath is the SQLite database file path, defaulting to wharf/wharf.db
	// under os.UserConfigDir, whose directory Load creates when missing.
	DBPath string `toml:"db_path"`
	// AtlassianSiteURL is the Jira Cloud REST API base URL, an absolute
	// http(s) URL such as https://{site}.atlassian.net or
	// https://api.atlassian.com/ex/jira/{cloudId} for scoped API tokens.
	AtlassianSiteURL string `toml:"atlassian_site_url"`
	// AtlassianEmail is the Atlassian account email used for Basic auth.
	AtlassianEmail string `toml:"atlassian_email"`
	// AtlassianAPIToken is the Atlassian API token used for Basic auth.
	AtlassianAPIToken string `toml:"atlassian_api_token"`
}

// Path returns the config file path: the value of PathEnv when set, otherwise
// wharf/config.toml under os.UserConfigDir.
func Path() (string, error) {
	if path := os.Getenv(PathEnv); path != "" {
		return path, nil
	}

	return defaultFilePath("config.toml")
}

// defaultFilePath returns name under the wharf directory in os.UserConfigDir.
func defaultFilePath(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", errors.Join(ErrFailedToFindUserConfigDir, err)
	}

	return filepath.Join(dir, "wharf", name), nil
}

// Load reads the TOML config file at path, rejects unknown keys, requires the
// Atlassian keys, validates the site URL, and fills unset values with defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReadConfigFile, path), err)
	}

	var cfg Config

	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToParseConfigFile, path), err)
	}

	if keys := md.Undecoded(); len(keys) > 0 {
		return Config{}, fmt.Errorf("%w: path=%s, keys=%v", ErrUnknownConfigKeys, path, keys)
	}

	for _, kv := range [][2]string{
		{"atlassian_site_url", cfg.AtlassianSiteURL},
		{"atlassian_email", cfg.AtlassianEmail},
		{"atlassian_api_token", cfg.AtlassianAPIToken},
	} {
		if kv[1] == "" {
			return Config{}, fmt.Errorf("%w: path=%s, key=%s", ErrMissingConfigKey, path, kv[0])
		}
	}

	// Userinfo is rejected (credentials come from the email/token keys) and
	// never echoed: the message uses withoutUserinfo, and the *url.Error, which
	// quotes the raw URL, is unwrapped to its cause.
	u, err := url.Parse(cfg.AtlassianSiteURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return Config{}, errors.Join(
			fmt.Errorf("%w: path=%s, atlassian_site_url=%s", ErrInvalidAtlassianSiteURL, path, withoutUserinfo(u)),
			errors.Unwrap(err),
		)
	}

	cfg.Addr = cmp.Or(cfg.Addr, defaultAddr)

	if cfg.DBPath == "" {
		if cfg.DBPath, err = defaultFilePath(defaultDBFileName); err != nil {
			return Config{}, err
		}

		if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o700); err != nil {
			return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateDBDir, cfg.DBPath), err)
		}
	}

	return cfg, nil
}

// withoutUserinfo formats u with its userinfo dropped, since url.URL.Redacted
// masks only the password and would echo a username-only secret.
func withoutUserinfo(u *url.URL) string {
	if u == nil {
		return ""
	}

	c := *u
	c.User = nil

	return c.String()
}
