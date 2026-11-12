// Package system runs the desktop commands the settings screen and the updater
// need on the machine that runs wharf, and manages its own executable and
// LaunchAgent.
package system

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

var (
	ErrUnsupportedOS       = errors.New("system: desktop commands are supported only on macOS")
	ErrFailedToChooseFile  = errors.New("system: failed to choose file")
	ErrFileChoiceCanceled  = errors.New("system: file choice was canceled")
	ErrFailedToOpenFile    = errors.New("system: failed to open file")
	ErrEmptyChosenFilePath = errors.New("system: chosen file path is empty")

	ErrFailedToNotify                  = errors.New("system: failed to show notification")
	ErrDBPasswordNotFound              = errors.New("system: database password not found in keychain")
	ErrFailedToReadDBPassword          = errors.New("system: failed to read database password from keychain")
	ErrEmptyDBPassword                 = errors.New("system: database password in keychain is empty")
	ErrFailedToReadGitHubToken         = errors.New("system: failed to read GitHub token from keychain")
	ErrEmptyGitHubToken                = errors.New("system: GitHub token in keychain is empty")
	ErrFailedToFindExecutableToReplace = errors.New("system: failed to find executable to replace")
	ErrFailedToFindExecutableToInstall = errors.New("system: failed to find executable to install")
	ErrFailedToCreateTempExecutable    = errors.New("system: failed to create temp executable")
	ErrFailedToWriteTempExecutable     = errors.New("system: failed to write temp executable")
	ErrFailedToBackUpExecutable        = errors.New("system: failed to back up executable")
	ErrFailedToReplaceBackup           = errors.New("system: failed to replace executable backup")
	ErrFailedToReplaceExecutable       = errors.New("system: failed to replace executable")
	ErrFailedToFindHomeDir             = errors.New("system: failed to find home dir")
	ErrFailedToResolveConfigPath       = errors.New("system: failed to resolve config path")
	ErrFailedToCreateLaunchAgentDir    = errors.New("system: failed to create launch agent dir")
	ErrFailedToCreateLogDir            = errors.New("system: failed to create log dir")
	ErrFailedToRenderLaunchAgent       = errors.New("system: failed to render launch agent")
	ErrFailedToWriteLaunchAgent        = errors.New("system: failed to write launch agent")
	ErrFailedToBootstrapLaunchAgent    = errors.New("system: failed to bootstrap launch agent")
	ErrFailedToRemoveLaunchAgentConfig = errors.New("system: failed to remove launch agent")
)

// chooseTOMLScript asks for a .toml file and prints its POSIX path.
const chooseTOMLScript = `POSIX path of (choose file of type {"toml"} with prompt "Choose a Wharf config file")`

// notifyScript shows a notification whose title and message are its
// arguments, so they need no AppleScript quoting.
var notifyScript = []string{
	"-e", "on run argv",
	"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
	"-e", "end run",
}

// LaunchAgentLabel is the launchd label of the wharf LaunchAgent.
const LaunchAgentLabel = "com.byron1st.wharf"

// launchAgent keeps wharf running in the user's GUI session, where osascript
// dialogs and notifications work. KeepAlive restarts it unless it exits 0.
var launchAgent = template.Must(template.New("plist").Funcs(template.FuncMap{"xml": xmlText}).Parse(
	`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{xml .Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{xml .Executable}}</string>
	</array>
{{- if .ConfigPath}}
	<key>EnvironmentVariables</key>
	<dict>
		<key>WHARF_CONFIG</key>
		<string>{{xml .ConfigPath}}</string>
	</dict>
{{- end}}
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>{{xml .LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{xml .LogPath}}</string>
</dict>
</plist>
`))

// securityPath is the macOS keychain command, by absolute path so that no
// other "security" earlier on PATH is handed the request.
const securityPath = "/usr/bin/security"

// KeychainDBService is the service name of the generic password item in the
// login keychain that holds the database password.
const KeychainDBService = "wharf-db"

// KeychainGitHubService is the service name of the optional generic password
// item in the login keychain that holds a GitHub token for the update check.
const KeychainGitHubService = "wharf-github"

// itemNotFoundExitCode is the exit code of security when no keychain item
// matches (errSecItemNotFound).
const itemNotFoundExitCode = 44

// userCanceled is in osascript's stderr when the dialog is canceled: it is
// AppleScript error -128.
const userCanceled = "(-128)"

type Desktop interface {
	ChooseTOMLFile(ctx context.Context) (string, error)
	Open(ctx context.Context, path string) error
	Notify(ctx context.Context, title, message string) error
	DBPassword(ctx context.Context) (string, error)
	GitHubToken(ctx context.Context) (string, error)
	ReplaceExecutable(ctx context.Context, data []byte) error
	InstallLaunchAgent(ctx context.Context, configPath string) error
	UninstallLaunchAgent(ctx context.Context) error
}

type client struct {
	goos       string
	run        func(ctx context.Context, name string, args ...string) ([]byte, error)
	executable func() (string, error)
	homeDir    func() (string, error)
	uid        int
}

// NewDesktop returns a Desktop that runs osascript, open and launchctl on
// macOS and manages the running executable.
func NewDesktop() Desktop {
	return &client{
		goos: runtime.GOOS,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		executable: os.Executable,
		homeDir:    os.UserHomeDir,
		uid:        os.Getuid(),
	}
}

// ChooseTOMLFile shows the macOS file dialog for a .toml file and returns the
// chosen absolute path. It fails with ErrFileChoiceCanceled when the dialog is
// canceled.
func (c *client) ChooseTOMLFile(ctx context.Context) (string, error) {
	if c.goos != "darwin" {
		return "", ErrUnsupportedOS
	}

	out, err := c.run(ctx, "osascript", "-e", chooseTOMLScript)
	if exitErr, ok := errors.AsType[*exec.ExitError](
		err,
	); ok &&
		strings.Contains(string(exitErr.Stderr), userCanceled) {
		return "", ErrFileChoiceCanceled
	}

	if err != nil {
		return "", errors.Join(ErrFailedToChooseFile, err)
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrEmptyChosenFilePath
	}

	return path, nil
}

// Open opens path with its default macOS app.
func (c *client) Open(ctx context.Context, path string) error {
	if c.goos != "darwin" {
		return ErrUnsupportedOS
	}

	if _, err := c.run(ctx, "open", "--", path); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToOpenFile, path), err)
	}

	return nil
}

// Notify shows a macOS notification. Clicking it does not open wharf.
func (c *client) Notify(ctx context.Context, title, message string) error {
	if c.goos != "darwin" {
		return ErrUnsupportedOS
	}

	if _, err := c.run(ctx, "osascript", append(notifyScript, title, message)...); err != nil {
		return errors.Join(ErrFailedToNotify, err)
	}

	return nil
}

// DBPassword returns the password of the KeychainDBService generic password
// item in the user's keychains. It fails with ErrDBPasswordNotFound when there
// is no such item and with ErrEmptyDBPassword when its password is empty.
// Error messages never carry the password.
func (c *client) DBPassword(ctx context.Context) (string, error) {
	if c.goos != "darwin" {
		return "", ErrUnsupportedOS
	}

	out, err := c.run(ctx, securityPath, "find-generic-password", "-s", KeychainDBService, "-w")
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == itemNotFoundExitCode {
		return "", fmt.Errorf("%w: service=%s", ErrDBPasswordNotFound, KeychainDBService)
	}

	if err != nil {
		return "", errors.Join(fmt.Errorf("%w: service=%s", ErrFailedToReadDBPassword, KeychainDBService), err)
	}

	// security ends the password with a newline.
	password := strings.TrimSuffix(string(out), "\n")
	if password == "" {
		return "", fmt.Errorf("%w: service=%s", ErrEmptyDBPassword, KeychainDBService)
	}

	return password, nil
}

// GitHubToken returns the password of the KeychainGitHubService generic
// password item in the user's keychains, or "" when there is no such item: the
// token is optional. It fails with ErrEmptyGitHubToken when the item's
// password is empty. Error messages never carry the token.
func (c *client) GitHubToken(ctx context.Context) (string, error) {
	if c.goos != "darwin" {
		return "", ErrUnsupportedOS
	}

	out, err := c.run(ctx, securityPath, "find-generic-password", "-s", KeychainGitHubService, "-w")
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == itemNotFoundExitCode {
		return "", nil
	}

	if err != nil {
		return "", errors.Join(fmt.Errorf("%w: service=%s", ErrFailedToReadGitHubToken, KeychainGitHubService), err)
	}

	// security ends the password with a newline, and a token pasted at the
	// prompt may carry stray whitespace that would break the header.
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("%w: service=%s", ErrEmptyGitHubToken, KeychainGitHubService)
	}

	return token, nil
}

// ReplaceExecutable makes data the running executable, resolved through
// symlinks, and keeps the previous one next to it with an ".old" suffix, a
// hard link. The new file is written next to the executable and renamed over
// it, so a file is at the path at every moment and the running process keeps
// its own file: overwriting it in place would get the process killed by macOS
// code signing. On failure the executable is left as it was.
func (c *client) ReplaceExecutable(_ context.Context, data []byte) error {
	exe, err := c.executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}

	if err != nil {
		return errors.Join(ErrFailedToFindExecutableToReplace, err)
	}

	f, err := os.CreateTemp(filepath.Dir(exe), "."+filepath.Base(exe)+"-*")
	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateTempExecutable, exe), err)
	}

	// Removing after a successful rename fails harmlessly.
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(0o755)
	}

	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteTempExecutable, exe), err)
	}

	// Link does not replace an existing file, so the backup is linked under the
	// temp name and renamed over the previous one, which stays until then.
	backup := f.Name() + ".old"
	defer func() { _ = os.Remove(backup) }()

	if err := os.Link(exe, backup); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToBackUpExecutable, exe), err)
	}

	if err := os.Rename(backup, exe+".old"); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReplaceBackup, exe), err)
	}

	if err := os.Rename(f.Name(), exe); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToReplaceExecutable, exe), err)
	}

	return nil
}

// InstallLaunchAgent writes the wharf LaunchAgent for the running executable
// to ~/Library/LaunchAgents and loads it, replacing a loaded one. configPath,
// when not empty, becomes its WHARF_CONFIG; output goes to
// ~/Library/Logs/wharf/wharf.log.
func (c *client) InstallLaunchAgent(ctx context.Context, configPath string) error {
	if c.goos != "darwin" {
		return ErrUnsupportedOS
	}

	exe, err := c.executable()
	if err != nil {
		return errors.Join(ErrFailedToFindExecutableToInstall, err)
	}

	if configPath != "" {
		// launchd starts the agent in /, so a relative path would not resolve.
		if configPath, err = filepath.Abs(configPath); err != nil {
			return errors.Join(ErrFailedToResolveConfigPath, err)
		}
	}

	home, err := c.home()
	if err != nil {
		return err
	}

	plistPath := launchAgentPath(home)
	logPath := filepath.Join(home, "Library", "Logs", "wharf", "wharf.log")

	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateLaunchAgentDir, filepath.Dir(plistPath)), err)
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToCreateLogDir, filepath.Dir(logPath)), err)
	}

	var plist bytes.Buffer
	if err := launchAgent.Execute(&plist, map[string]string{
		"Label":      LaunchAgentLabel,
		"Executable": exe,
		"ConfigPath": configPath,
		"LogPath":    logPath,
	}); err != nil {
		return errors.Join(ErrFailedToRenderLaunchAgent, err)
	}

	if err := os.WriteFile(plistPath, plist.Bytes(), 0o644); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToWriteLaunchAgent, plistPath), err)
	}

	// An agent loaded before must be booted out first or bootstrap fails; when
	// none is loaded, bootout fails harmlessly.
	_, _ = c.run(ctx, "launchctl", "bootout", c.launchAgentTarget())

	if _, err := c.run(ctx, "launchctl", "bootstrap", c.launchDomain(), plistPath); err != nil {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToBootstrapLaunchAgent, plistPath), err)
	}

	return nil
}

// UninstallLaunchAgent stops and unloads the wharf LaunchAgent and removes its
// file. Neither a not-loaded agent nor a missing file is an error.
func (c *client) UninstallLaunchAgent(ctx context.Context) error {
	if c.goos != "darwin" {
		return ErrUnsupportedOS
	}

	home, err := c.home()
	if err != nil {
		return err
	}

	plistPath := launchAgentPath(home)

	// bootout fails when the agent is not loaded, which is the wanted state.
	_, _ = c.run(ctx, "launchctl", "bootout", c.launchAgentTarget())

	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToRemoveLaunchAgentConfig, plistPath), err)
	}

	return nil
}

func (c *client) home() (string, error) {
	home, err := c.homeDir()
	if err != nil {
		return "", errors.Join(ErrFailedToFindHomeDir, err)
	}

	return home, nil
}

func launchAgentPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

// launchDomain is the user's GUI session domain.
func (c *client) launchDomain() string {
	return "gui/" + strconv.Itoa(c.uid)
}

func (c *client) launchAgentTarget() string {
	return c.launchDomain() + "/" + LaunchAgentLabel
}

func xmlText(s string) (string, error) {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return "", err
	}

	return b.String(), nil
}
