package system

import "context"

// NewDesktopWith returns a Desktop that acts as on goos and runs commands with
// run, so tests can stand in for osascript and open.
func NewDesktopWith(goos string, run func(ctx context.Context, name string, args ...string) ([]byte, error)) Desktop {
	return &client{goos: goos, run: run}
}
