package agent

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// ExecResult holds the raw output of a command execution.
type ExecResult struct {
	Stdout       string
	Stderr       string
	ExitCode     int
	EarlyCancel  bool   // true if execution was cancelled due to detected auth/rate-limit error
	CancelReason string // the matched pattern that caused early cancellation
}

// Executor abstracts command execution for testability.
type Executor interface {
	Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error)
	// ExecuteInDir runs the command in the specified directory.
	// Falls back to Execute if dir is empty.
	ExecuteInDir(ctx context.Context, binary string, args []string, stdin, dir string) (*ExecResult, error)
}

// authCancelPatterns are regexps matched against lowercased stderr lines that indicate
// the subprocess will never succeed and should be cancelled immediately.
// Numeric HTTP codes use word-boundary anchors to avoid false positives (e.g. "port 4290").
// Overly broad patterns like "credential" and "please run" are intentionally excluded.
var authCancelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b429\b`),
	regexp.MustCompile(`\b401\b`),
	regexp.MustCompile(`\b403\b`),
	regexp.MustCompile(`rate limit`),
	regexp.MustCompile(`ratelimit`),
	regexp.MustCompile(`quota exceeded`),
	regexp.MustCompile(`quota_exceeded`),
	regexp.MustCompile(`not logged in`),
	regexp.MustCompile(`not signed in`),
	regexp.MustCompile(`no credentials`),
	regexp.MustCompile(`authentication required`),
	regexp.MustCompile(`authentication failed`),
	regexp.MustCompile(`unauthorized`),
	regexp.MustCompile(`unauthenticated`),
	regexp.MustCompile(`invalid api key`),
	regexp.MustCompile(`invalid_api_key`),
	regexp.MustCompile(`api key not found`),
	regexp.MustCompile(`insufficient credits`),
	regexp.MustCompile(`insufficient_credits`),
	regexp.MustCompile(`login required`),
	regexp.MustCompile(`api key expired`),
	regexp.MustCompile(`token expired`),
}

// matchesCancelPattern returns the matched pattern string if line matches a known
// auth/rate-limit pattern that warrants early cancellation, empty string otherwise.
func matchesCancelPattern(line string) string {
	lower := strings.ToLower(line)
	for _, p := range authCancelPatterns {
		if p.MatchString(lower) {
			return p.String()
		}
	}
	return ""
}

// OSExecutor executes commands via os/exec.
type OSExecutor struct{}

// Execute runs a command and captures output.
func (e *OSExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	return e.ExecuteInDir(ctx, binary, args, stdin, "")
}

// ExecuteInDir runs a command in the specified directory and captures output.
// It monitors stderr in real time and cancels the subprocess immediately if
// a known auth/rate-limit error pattern is detected.
func (e *OSExecutor) ExecuteInDir(ctx context.Context, binary string, args []string, stdin, dir string) (*ExecResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Stream stderr: collect it and watch for cancel patterns.
	var stderrBuf bytes.Buffer
	var cancelReason string
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line)
			stderrBuf.WriteByte('\n')
			if cancelReason == "" {
				if reason := matchesCancelPattern(line); reason != "" {
					cancelReason = reason
					cancel() // kill the subprocess immediately
				}
			}
		}
		// Drain any remaining bytes not caught by the scanner.
		_, _ = io.Copy(&stderrBuf, stderrPipe)
	}()

	<-stderrDone
	runErr := cmd.Wait()

	result := &ExecResult{
		Stdout:       stdout.String(),
		Stderr:       stderrBuf.String(),
		EarlyCancel:  cancelReason != "",
		CancelReason: cancelReason,
	}

	if runErr != nil {
		if cancelReason != "" {
			// We triggered the cancel; report as early-cancel rather than timeout.
			result.ExitCode = -1
			return result, nil
		}
		if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			return result, runErr
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil // non-zero exit is not an execution error
		}
		return result, runErr
	}
	return result, nil
}

// LookPathFunc abstracts binary discovery for testability.
type LookPathFunc func(file string) (string, error)

// DefaultLookPath is the production implementation.
var DefaultLookPath LookPathFunc = exec.LookPath
