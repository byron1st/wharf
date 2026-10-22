package tool

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
)

type Tool interface {
	Run(opt RunOption) (string, error)
}

var (
	ErrFailedToParseExecCmdPath = errors.New("failed to parse exec cmd path")
	ErrFailedToParseCwdPath     = errors.New("failed to parse cwd path")
	ErrFailedToExecuteCmd       = errors.New("failed to execute cmd")
)

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

	return &tool{execCmd: c}, nil
}

func (t *tool) Run(opt RunOption) (string, error) {
	cwdPath, err := filepath.Abs(opt.Cwd)
	if err != nil {
		return "", errors.Join(ErrFailedToParseCwdPath, err)
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(t.execCmd, append([]string{opt.Cmd}, opt.Args...)...)
	cmd.Dir = cwdPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", errors.Join(fmt.Errorf("%w: stderr=%s", ErrFailedToExecuteCmd, stderr.String()), err)
	}

	return stdout.String(), nil
}
