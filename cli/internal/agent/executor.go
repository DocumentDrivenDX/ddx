package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
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

type executionTimeoutKey struct{}

func withExecutionTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, executionTimeoutKey{}, timeout)
}

func executionTimeoutFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	if timeout, ok := ctx.Value(executionTimeoutKey{}).(time.Duration); ok {
		return timeout
	}
	return 0
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

	cmd := exec.Command(binary, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
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

	var (
		stdoutBuf    bytes.Buffer
		stderrBuf    bytes.Buffer
		cancelReason string
		timedOut     bool
		killOnce     sync.Once
	)

	stopProcess := func() {
		killOnce.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}

	activity := make(chan struct{}, 1)
	pulse := func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}

	idleTimeout := executionTimeoutFromContext(ctx)
	if idleTimeout > 0 {
		go func() {
			timer := time.NewTimer(idleTimeout)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					stopProcess()
					return
				case <-activity:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(idleTimeout)
				case <-timer.C:
					timedOut = true
					stopProcess()
					return
				}
			}
		}()
	} else {
		go func() {
			<-ctx.Done()
			stopProcess()
		}()
	}

	// Stream stdout to collect it and count any write as progress.
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		_, _ = io.Copy(&activityWriter{Buffer: &stdoutBuf, OnWrite: pulse}, stdoutPipe)
	}()

	// Stream stderr: collect it, count progress, and watch for cancel patterns.
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line)
			stderrBuf.WriteByte('\n')
			pulse()
			if cancelReason == "" {
				if reason := matchesCancelPattern(line); reason != "" {
					cancelReason = reason
					cancel()
					stopProcess()
				}
			}
		}
		// Drain any remaining bytes not caught by the scanner.
		_, _ = io.Copy(&activityWriter{Buffer: &stderrBuf, OnWrite: pulse}, stderrPipe)
	}()

	<-stdoutDone
	<-stderrDone
	runErr := cmd.Wait()

	result := &ExecResult{
		Stdout:       stdoutBuf.String(),
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
		if timedOut {
			result.ExitCode = -1
			return result, context.DeadlineExceeded
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ExitCode = -1
			return result, ctx.Err()
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil // non-zero exit is not an execution error
		}
		return result, runErr
	}
	return result, nil
}

type activityWriter struct {
	Buffer  *bytes.Buffer
	OnWrite func()
}

func (w *activityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 && w.OnWrite != nil {
		w.OnWrite()
	}
	return w.Buffer.Write(p)
}

// LookPathFunc abstracts binary discovery for testability.
type LookPathFunc func(file string) (string, error)

// DefaultLookPath is the production implementation.
var DefaultLookPath LookPathFunc = exec.LookPath
