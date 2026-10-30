package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	ErrFailedToEncodeConfigValue = errors.New("config: failed to encode config value")
	ErrFailedToWriteConfigFile   = errors.New("config: failed to write config file")
)

// atlassianKeys are the keys SaveAtlassian writes, in file order.
var atlassianKeys = []string{"atlassian_site_url", "atlassian_email", "atlassian_api_token"}

// Config holds the settings wharf reads from its TOML config file.
type Config struct {
	// Addr is the TCP address the server listens on.
	Addr string `toml:"addr"`
	// DBPath is the SQLite database file path, defaulting to wharf/wharf.db
	// under os.UserConfigDir, whose directory Load creates when missing.
	DBPath string `toml:"db_path"`
	// AtlassianSiteURL is the Jira Cloud REST API base URL, an absolute https
	// URL without query or fragment such as https://{site}.atlassian.net or
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

// Load reads the TOML config file at path, rejects unknown keys, validates
// the Atlassian keys, and fills unset values with defaults. A missing file is
// an empty config. The Atlassian keys are all set or all unset; unset means
// Atlassian is not configured yet.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
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

	if err := validateAtlassian(path, cfg.AtlassianSiteURL, cfg.AtlassianEmail, cfg.AtlassianAPIToken); err != nil {
		return Config{}, err
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

// validateAtlassian checks the Atlassian keys as Load describes. Messages
// never carry the token.
func validateAtlassian(path, siteURL, email, apiToken string) error {
	values := []string{siteURL, email, apiToken}
	if siteURL == "" && email == "" && apiToken == "" {
		return nil
	}

	for i, key := range atlassianKeys {
		if values[i] == "" {
			return fmt.Errorf("%w: path=%s, key=%s", ErrMissingConfigKey, path, key)
		}
	}

	// Only https is accepted since Basic auth would send the token in plain
	// text over http. A query or fragment would end up before the API path.
	// Userinfo is rejected (credentials come from the email/token keys) and
	// never echoed: the message uses withoutUserinfo, and the *url.Error, which
	// quotes the raw URL, is unwrapped to its cause.
	u, err := url.Parse(siteURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.Join(
			fmt.Errorf("%w: path=%s, atlassian_site_url=%s", ErrInvalidAtlassianSiteURL, path, withoutUserinfo(u)),
			errors.Unwrap(err),
		)
	}

	return nil
}

// SaveAtlassian sets the Atlassian keys in the config file at path, creating
// it when missing. Only the lines of those keys change, so comments and other
// keys are kept: an existing top-level "key = ..." line is replaced and a
// missing one is appended. The values are validated as Load does. The file is
// replaced atomically and left readable only by its owner, since it holds the
// token.
func SaveAtlassian(path, siteURL, email, apiToken string) error {
	if err := validateAtlassian(path, siteURL, email, apiToken); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReadConfigFile, path), err)
	}

	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for i, value := range []string{siteURL, email, apiToken} {
		line, err := toml.Marshal(map[string]string{atlassianKeys[i]: value})
		if err != nil {
			return errors.Join(fmt.Errorf("%w: key=%s", ErrFailedToEncodeConfigValue, atlassianKeys[i]), err)
		}

		lines = setLine(lines, atlassianKeys[i], string(line))
	}

	if err := writeFileAtomic(path, []byte(strings.Join(lines, ""))); err != nil {
		return err
	}

	_, err = Load(path)

	return err
}

// setLine replaces the first top-level assignment of key in lines with line,
// or appends line when there is none.
func setLine(lines []string, key, line string) []string {
	for i, l := range lines {
		name, _, ok := strings.Cut(l, "=")
		if ok && strings.TrimSpace(name) == key && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			lines[i] = line

			return lines
		}
	}

	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		lines[n-1] += "\n"
	}

	return append(lines, line)
}

// writeFileAtomic writes data to a temp file next to path with mode 0600 and
// renames it over path, creating path's directory when missing.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteConfigFile, path), err)
	}

	f, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteConfigFile, path), err)
	}

	// Removing after a successful rename fails harmlessly.
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	if err == nil {
		err = os.Rename(f.Name(), path)
	}

	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteConfigFile, path), err)
	}

	return nil
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
