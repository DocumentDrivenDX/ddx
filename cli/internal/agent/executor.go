package agent

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

// ExecResult holds the raw output of a command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Executor abstracts command execution for testability.
type Executor interface {
	Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error)
}

// OSExecutor executes commands via os/exec.
type OSExecutor struct{}

// Execute runs a command and captures output.
func (e *OSExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	err := cmd.Run()
	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			return result, err
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil // non-zero exit is not an execution error
		}
		return result, err
	}
	return result, nil
}

// LookPathFunc abstracts binary discovery for testability.
type LookPathFunc func(file string) (string, error)

// DefaultLookPath is the production implementation.
var DefaultLookPath LookPathFunc = exec.LookPath
