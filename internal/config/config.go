package config

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// PathEnv is the environment variable that overrides the config file path.
const PathEnv = "WHARF_CONFIG"

const (
	defaultAddr                = "127.0.0.1:8080"
	defaultUpdateCheckInterval = 24 * time.Hour
	// minUpdateCheckInterval keeps the unauthenticated GitHub API rate limit
	// (60 requests an hour) far away.
	minUpdateCheckInterval = time.Hour
)

var (
	ErrFailedToFindUserConfigDir    = errors.New("config: failed to find user config dir")
	ErrFailedToReadConfigFile       = errors.New("config: failed to read config file")
	ErrFailedToParseConfigFile      = errors.New("config: failed to parse config file")
	ErrUnknownConfigKeys            = errors.New("config: unknown config keys")
	ErrInvalidDBURL                 = errors.New("config: invalid db url")
	ErrDBURLHasPassword             = errors.New("config: db url must not contain a password")
	ErrMissingDBURL                 = errors.New("config: db_url is required to start the server")
	ErrMissingConfigKey             = errors.New("config: missing required config key")
	ErrInvalidAtlassianSiteURL      = errors.New("config: invalid atlassian site url")
	ErrFailedToEncodeConfigValue    = errors.New("config: failed to encode config value")
	ErrFailedToResolveConfigPath    = errors.New("config: failed to resolve config file path")
	ErrFailedToReadConfigFileToSave = errors.New("config: failed to read config file to save")
	ErrFailedToCreateConfigDir      = errors.New("config: failed to create config dir")
	ErrFailedToCreateTempConfigFile = errors.New("config: failed to create temp config file")
	ErrFailedToWriteTempConfigFile  = errors.New("config: failed to write temp config file")
	ErrFailedToReplaceConfigFile    = errors.New("config: failed to replace config file")
	ErrUpdateCheckIntervalTooShort  = errors.New("config: update check interval is too short")
)

// atlassianKeys are the keys SaveAtlassian writes, in file order.
var atlassianKeys = []string{"atlassian_site_url", "atlassian_email", "atlassian_api_token"}

// Config holds the settings wharf reads from its TOML config file.
type Config struct {
	// Addr is the TCP address the server listens on.
	Addr string `toml:"addr"`
	// DBURL is the Postgres connection URL, such as
	// postgresql://user@host:5432/postgres?sslmode=require, without a password:
	// the password is read from the macOS Keychain. Empty means the database is
	// not configured yet.
	DBURL string `toml:"db_url"`
	// AtlassianSiteURL is the Jira Cloud REST API base URL, an absolute https
	// URL without query or fragment such as https://{site}.atlassian.net or
	// https://api.atlassian.com/ex/jira/{cloudId} for scoped API tokens.
	AtlassianSiteURL string `toml:"atlassian_site_url"`
	// AtlassianEmail is the Atlassian account email used for Basic auth.
	AtlassianEmail string `toml:"atlassian_email"`
	// AtlassianAPIToken is the Atlassian API token used for Basic auth.
	AtlassianAPIToken string `toml:"atlassian_api_token"`
	// UpdateCheck turns on the periodic check for a newer GitHub release. It
	// is the update_check key, true when unset.
	UpdateCheck bool `toml:"-"`
	// UpdateCheckInterval is how often the update check runs, a duration
	// string such as "12h" in the file, 24h when unset and at least 1h.
	UpdateCheckInterval time.Duration `toml:"update_check_interval"`
}

// file is the config file as decoded: update_check needs a pointer to tell
// unset from false.
type file struct {
	Config

	UpdateCheck *bool `toml:"update_check"`
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
// the database URL and the Atlassian keys, and fills unset values with defaults. A missing file is
// an empty config. The Atlassian keys are all set or all unset; unset means
// Atlassian is not configured yet.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReadConfigFile, path), err)
	}

	var f file

	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToParseConfigFile, path), err)
	}

	if keys := md.Undecoded(); len(keys) > 0 {
		return Config{}, fmt.Errorf("%w: path=%s, keys=%v", ErrUnknownConfigKeys, path, keys)
	}

	cfg := f.Config
	if err := validateAtlassian(path, cfg.AtlassianSiteURL, cfg.AtlassianEmail, cfg.AtlassianAPIToken); err != nil {
		return Config{}, err
	}

	cfg.Addr = cmp.Or(cfg.Addr, defaultAddr)
	cfg.UpdateCheck = f.UpdateCheck == nil || *f.UpdateCheck

	cfg.UpdateCheckInterval = cmp.Or(cfg.UpdateCheckInterval, defaultUpdateCheckInterval)
	if cfg.UpdateCheckInterval < minUpdateCheckInterval {
		return Config{}, fmt.Errorf(
			"%w: path=%s, update_check_interval=%s, min=%s",
			ErrUpdateCheckIntervalTooShort, path, cfg.UpdateCheckInterval, minUpdateCheckInterval,
		)
	}

	if err := validateDBURL(path, cfg.DBURL); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// LoadForServer loads path as Load does and also requires db_url, which the
// server needs to start. Load allows it to be missing so that a file holding
// only the Atlassian keys still loads.
func LoadForServer(path string) (Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return Config{}, err
	}

	if cfg.DBURL == "" {
		return Config{}, fmt.Errorf("%w: path=%s", ErrMissingDBURL, path)
	}

	return cfg, nil
}

// DBTarget returns the host and database of DBURL without its user, for
// display. It is empty when DBURL is.
func (c Config) DBTarget() string {
	u, err := url.Parse(c.DBURL)
	if err != nil {
		return ""
	}

	return u.Host + u.Path
}

// validateDBURL accepts an empty url or a postgres:// or postgresql:// URL
// with a host and without a password. Messages never carry the userinfo, and
// the *url.Error, which quotes the raw URL, is unwrapped to its cause.
func validateDBURL(path, dbURL string) error {
	if dbURL == "" {
		return nil
	}

	u, err := url.Parse(dbURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return errors.Join(
			fmt.Errorf("%w: path=%s, db_url=%s", ErrInvalidDBURL, path, withoutUserinfo(u)),
			errors.Unwrap(err),
		)
	}

	if _, ok := u.User.Password(); ok {
		return fmt.Errorf("%w: path=%s, db_url=%s", ErrDBURLHasPassword, path, withoutUserinfo(u))
	}

	return nil
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

// ValidateAtlassian checks Atlassian settings that are not saved yet as Load
// checks the saved keys, so nothing is sent to an unvalidated site.
func ValidateAtlassian(siteURL, email, apiToken string) error {
	return validateAtlassian("", siteURL, email, apiToken)
}

// SaveAtlassian sets the Atlassian keys in the config file at path, creating
// it when missing. Only the lines of those keys change, so comments and other
// keys are kept: an existing top-level "key = ..." line is replaced and a
// missing one is appended. The values are validated as Load does, and so is
// the new file before it replaces the old one, which is kept on any failure.
// When path is a symlink, its target is written and the link kept. The file
// is replaced atomically and left readable only by its owner, since it holds
// the token.
func SaveAtlassian(path, siteURL, email, apiToken string) error {
	if err := validateAtlassian(path, siteURL, email, apiToken); err != nil {
		return err
	}

	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		target, err = path, nil
	}

	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToResolveConfigPath, path), err)
	}

	data, err := os.ReadFile(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReadConfigFileToSave, path), err)
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

	return writeFileAtomic(path, target, []byte(strings.Join(lines, "")))
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

// writeFileAtomic writes data to a temp file next to target with mode 0600,
// checks it loads, and renames it over target, creating target's directory
// when missing. Errors name path, the config file as the caller gave it.
func writeFileAtomic(path, target string, data []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateConfigDir, dir), err)
	}

	f, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateTempConfigFile, path), err)
	}

	// Removing after a successful rename fails harmlessly.
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteTempConfigFile, path), err)
	}

	if _, err := Load(f.Name()); err != nil {
		return fmt.Errorf("%w: path=%s", err, path)
	}

	if err := os.Rename(f.Name(), target); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReplaceConfigFile, path), err)
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
