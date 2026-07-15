package agent

// This subprocess executor exists only for legacy unit tests of the retired
// DDx-owned harness runner. Production harness execution belongs to Fizeau.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

type LookPathFunc func(file string) (string, error)

func DefaultLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type ExecResult struct {
	Stdout           string
	Stderr           string
	ExitCode         int
	EarlyCancel      bool
	CancelReason     string
	WallClockTimeout bool
	WallClockElapsed time.Duration
}

type Executor interface {
	Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error)
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
	timeout, _ := ctx.Value(executionTimeoutKey{}).(time.Duration)
	return timeout
}

type executionWallClockKey struct{}
type executionEnvKey struct{}

func withExecutionWallClock(ctx context.Context, wallClock time.Duration) context.Context {
	if wallClock <= 0 {
		return ctx
	}
	return context.WithValue(ctx, executionWallClockKey{}, wallClock)
}

func executionWallClockFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	wallClock, _ := ctx.Value(executionWallClockKey{}).(time.Duration)
	return wallClock
}

func withExecutionEnv(ctx context.Context, env map[string]string) context.Context {
	if len(env) == 0 {
		return ctx
	}
	return context.WithValue(ctx, executionEnvKey{}, cloneStringMap(env))
}

func executionEnvFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	env, ok := ctx.Value(executionEnvKey{}).(map[string]string)
	if !ok || len(env) == 0 {
		return nil
	}
	return cloneStringMap(env)
}

var authCancelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b429\b`), regexp.MustCompile(`\b401\b`), regexp.MustCompile(`\b403\b`),
	regexp.MustCompile(`rate limit`), regexp.MustCompile(`ratelimit`),
	regexp.MustCompile(`quota exceeded`), regexp.MustCompile(`quota_exceeded`),
	regexp.MustCompile(`not logged in`), regexp.MustCompile(`not signed in`),
	regexp.MustCompile(`no credentials`), regexp.MustCompile(`authentication required`),
	regexp.MustCompile(`authentication failed`), regexp.MustCompile(`unauthorized`),
	regexp.MustCompile(`unauthenticated`), regexp.MustCompile(`invalid api key`),
	regexp.MustCompile(`invalid_api_key`), regexp.MustCompile(`api key not found`),
	regexp.MustCompile(`insufficient credits`), regexp.MustCompile(`insufficient_credits`),
	regexp.MustCompile(`login required`), regexp.MustCompile(`api key expired`),
	regexp.MustCompile(`token expired`),
}

func matchesCancelPattern(line string) string {
	lower := strings.ToLower(line)
	for _, pattern := range authCancelPatterns {
		if pattern.MatchString(lower) {
			return pattern.String()
		}
	}
	return ""
}

type OSExecutor struct{}

func (e *OSExecutor) Execute(ctx context.Context, binary string, args []string, stdin string) (*ExecResult, error) {
	return e.ExecuteInDir(ctx, binary, args, stdin, "")
}

func (e *OSExecutor) ExecuteInDir(ctx context.Context, binary string, args []string, stdin, dir string) (*ExecResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.Command(binary, args...)
	cmdSetProcessGroup(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = envWithOverrides(internalgit.CleanEnv(), executionEnvFromContext(ctx))
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
	runtime.LockOSThread()
	err = cmd.Start()
	runtime.UnlockOSThread()
	if err != nil {
		return nil, err
	}

	var (
		stdoutBuf         bytes.Buffer
		stderrBuf         bytes.Buffer
		cancelReason      string
		timedOut          atomic.Bool
		wallClockTimedOut atomic.Bool
		wallClockElapsed  atomic.Int64
		killOnce          sync.Once
	)
	stopProcess := func() { killOnce.Do(func() { cmdKillProcessGroup(cmd) }) }
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
					timedOut.Store(true)
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

	wallClock := executionWallClockFromContext(ctx)
	if wallClock > 0 {
		wallClockStart := time.Now()
		go func() {
			timer := time.NewTimer(wallClock)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
				wallClockTimedOut.Store(true)
				wallClockElapsed.Store(int64(time.Since(wallClockStart)))
				cancel()
				stopProcess()
			}
		}()
	}

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		_, _ = io.Copy(&activityWriter{Buffer: &stdoutBuf, OnWrite: pulse}, stdoutPipe)
	}()
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
		_, _ = io.Copy(&activityWriter{Buffer: &stderrBuf, OnWrite: pulse}, stderrPipe)
	}()
	<-stdoutDone
	<-stderrDone
	runErr := cmd.Wait()

	result := &ExecResult{
		Stdout: stdoutBuf.String(), Stderr: stderrBuf.String(), EarlyCancel: cancelReason != "",
		CancelReason: cancelReason, WallClockTimeout: wallClockTimedOut.Load(),
		WallClockElapsed: time.Duration(wallClockElapsed.Load()),
	}
	if runErr != nil {
		if cancelReason != "" {
			result.ExitCode = -1
			return result, nil
		}
		if wallClockTimedOut.Load() || timedOut.Load() {
			result.ExitCode = -1
			return result, context.DeadlineExceeded
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ExitCode = -1
			return result, ctx.Err()
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, runErr
	}
	return result, nil
}

type activityWriter struct {
	Buffer  *bytes.Buffer
	OnWrite func()
}

func (w *activityWriter) Write(data []byte) (int, error) {
	if len(data) > 0 && w.OnWrite != nil {
		w.OnWrite()
	}
	return w.Buffer.Write(data)
}
