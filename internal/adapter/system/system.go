// Package system runs the desktop commands the settings screen needs on the
// machine that runs wharf.
package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrUnsupportedOS       = errors.New("system: desktop commands are supported only on macOS")
	ErrFailedToChooseFile  = errors.New("system: failed to choose file")
	ErrFileChoiceCanceled  = errors.New("system: file choice was canceled")
	ErrFailedToOpenFile    = errors.New("system: failed to open file")
	ErrEmptyChosenFilePath = errors.New("system: chosen file path is empty")
)

// chooseTOMLScript asks for a .toml file and prints its POSIX path.
const chooseTOMLScript = `POSIX path of (choose file of type {"toml"} with prompt "Choose a Wharf config file")`

// userCanceled is in osascript's stderr when the dialog is canceled: it is
// AppleScript error -128.
const userCanceled = "(-128)"

type Desktop interface {
	ChooseTOMLFile(ctx context.Context) (string, error)
	Open(ctx context.Context, path string) error
}

type client struct {
	goos string
	run  func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewDesktop returns a Desktop that runs osascript and open on macOS.
func NewDesktop() Desktop {
	return &client{goos: runtime.GOOS, run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	}}
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
