package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Tool interface {
	Run(ctx context.Context, opt RunOption) (string, error)
}

var (
	ErrFailedToParseExecCmdPath = errors.New("tool: failed to parse exec cmd path")
	ErrFailedToStatExecCmd      = errors.New("tool: failed to stat exec cmd")
	ErrExecCmdIsNotExecutable   = errors.New("tool: exec cmd is not an executable file")
	ErrFailedToParseCwdPath     = errors.New("tool: failed to parse cwd path")
	ErrFailedToExecuteCmd       = errors.New("tool: failed to execute cmd")
)

// waitDelay bounds how long Run waits for output pipes after the process is
// killed on context done, since children of the process may keep them open.
const waitDelay = time.Second

type RunOption struct {
	Cmd  string
	Args []string
	Cwd  string
}

type tool struct {
	execCmd string
}

func NewTool(execCmd string) (Tool, error) {
	c, err := filepath.Abs(execCmd)
	if err != nil {
		return nil, errors.Join(ErrFailedToParseExecCmdPath, err)
	}

	info, err := os.Stat(c)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: path=%s", ErrFailedToStatExecCmd, c), err)
	}

	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("%w: path=%s, mode=%s", ErrExecCmdIsNotExecutable, c, info.Mode())
	}

	return &tool{execCmd: c}, nil
}

func (t *tool) Run(ctx context.Context, opt RunOption) (string, error) {
	cwdPath, err := filepath.Abs(opt.Cwd)
	if err != nil {
		return "", errors.Join(ErrFailedToParseCwdPath, err)
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, t.execCmd, append([]string{opt.Cmd}, opt.Args...)...)
	cmd.Dir = cwdPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = waitDelay

	if err := cmd.Run(); err != nil {
		return "", errors.Join(fmt.Errorf("%w: stderr=%s", ErrFailedToExecuteCmd, stderr.String()), err, ctx.Err())
	}

	return stdout.String(), nil
}
