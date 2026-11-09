package system

import (
	"context"
	"os"
)

// NewDesktopWith returns a Desktop that acts as on goos and runs commands with
// run, so tests can stand in for osascript and open.
func NewDesktopWith(goos string, run func(ctx context.Context, name string, args ...string) ([]byte, error)) Desktop {
	return &client{goos: goos, run: run, executable: os.Executable, homeDir: os.UserHomeDir, uid: os.Getuid()}
}

// NewDesktopFor returns a Desktop like NewDesktopWith whose executable is exe,
// home dir is home and user ID is uid.
func NewDesktopFor(
	goos string,
	run func(ctx context.Context, name string, args ...string) ([]byte, error),
	exe, home string,
	uid int,
) Desktop {
	return &client{
		goos:       goos,
		run:        run,
		executable: func() (string, error) { return exe, nil },
		homeDir:    func() (string, error) { return home, nil },
		uid:        uid,
	}
}
