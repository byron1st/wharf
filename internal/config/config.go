package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// PathEnv is the environment variable that overrides the config file path.
const PathEnv = "WHARF_CONFIG"

const (
	defaultAddr       = ":8080"
	defaultDBFileName = "wharf.db"
	defaultACLIName   = "acli"
)

var (
	ErrFailedToFindUserConfigDir = errors.New("config: failed to find user config dir")
	ErrFailedToReadConfigFile    = errors.New("config: failed to read config file")
	ErrFailedToParseConfigFile   = errors.New("config: failed to parse config file")
	ErrUnknownConfigKeys         = errors.New("config: unknown config keys")
	ErrFailedToCreateDBDir       = errors.New("config: failed to create default db dir")
	ErrFailedToFindACLI          = errors.New("config: failed to find acli")
)

// Config holds the settings wharf reads from its TOML config file.
type Config struct {
	// Addr is the TCP address the server listens on.
	Addr string `toml:"addr"`
	// DBPath is the SQLite database file path, defaulting to wharf/wharf.db
	// under os.UserConfigDir, whose directory Load creates when missing.
	DBPath string `toml:"db_path"`
	// ACLIPath is the acli executable path; after Load it is always resolved.
	ACLIPath string `toml:"acli_path"`
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

// Load reads the TOML config file at path, rejects unknown keys, fills unset
// values with defaults, and resolves the acli executable. A bare acli name,
// including the default "acli", is searched in PATH because tool.NewTool does
// not search it.
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

	cfg.Addr = cmp.Or(cfg.Addr, defaultAddr)

	if cfg.DBPath == "" {
		if cfg.DBPath, err = defaultFilePath(defaultDBFileName); err != nil {
			return Config{}, err
		}

		if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o700); err != nil {
			return Config{}, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateDBDir, cfg.DBPath), err)
		}
	}

	acliPath, err := exec.LookPath(cmp.Or(cfg.ACLIPath, defaultACLIName))
	if err != nil {
		return Config{}, errors.Join(fmt.Errorf("%w: set acli_path in %s", ErrFailedToFindACLI, path), err)
	}

	cfg.ACLIPath = acliPath

	return cfg, nil
}
