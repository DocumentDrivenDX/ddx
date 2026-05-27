package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// ExecResult holds the raw output of a command execution.
type ExecResult struct {
	Stdout           string
	Stderr           string
	ExitCode         int
	EarlyCancel      bool          // true if execution was cancelled due to detected auth/rate-limit error
	CancelReason     string        // the matched pattern that caused early cancellation
	WallClockTimeout bool          // true if the absolute wall-clock deadline fired (vs the resettable idle timer)
	WallClockElapsed time.Duration // how long the process ran before the wall-clock timer fired; zero when WallClockTimeout is false
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

type executionWallClockKey struct{}
type executionEnvKey struct{}

// withExecutionWallClock attaches an absolute wall-clock deadline to ctx.
// Unlike withExecutionTimeout — which is an idle (inactivity) timer that
// resets on every stream byte or event — this bound fires regardless of
// activity so a chatty provider cannot pin the worker indefinitely.
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
	if wallClock, ok := ctx.Value(executionWallClockKey{}).(time.Duration); ok {
		return wallClock
	}
	return 0
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

	// Lock the OS thread before Start to ensure Pdeathsig is properly
	// associated with this thread. Pdeathsig is sent when the thread dies,
	// not the process, so we must prevent Go runtime from migrating the
	// goroutine to a different thread during the critical exec operation.
	// We unlock immediately after Start succeeds, as the kernel has already
	// bound the child to this thread's PID via Pdeathsig.
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
		wallClockElapsed  atomic.Int64 // nanoseconds; convert via time.Duration on read
		killOnce          sync.Once
	)

	stopProcess := func() {
		killOnce.Do(func() {
			cmdKillProcessGroup(cmd)
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

	// Wall-clock watchdog: fires at an absolute deadline regardless of
	// stream activity. Runs alongside the idle watchdog so a provider that
	// emits heartbeats cannot defeat the overall bound. See RC2 of
	// ddx-0a651925 for the incident that motivated this timer.
	wallClock := executionWallClockFromContext(ctx)
	if wallClock > 0 {
		wallClockStart := time.Now()
		go func() {
			timer := time.NewTimer(wallClock)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				wallClockTimedOut.Store(true)
				wallClockElapsed.Store(int64(time.Since(wallClockStart)))
				cancel()
				stopProcess()
			}
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
		Stdout:           stdoutBuf.String(),
		Stderr:           stderrBuf.String(),
		EarlyCancel:      cancelReason != "",
		CancelReason:     cancelReason,
		WallClockTimeout: wallClockTimedOut.Load(),
		WallClockElapsed: time.Duration(wallClockElapsed.Load()),
	}

	if runErr != nil {
		if cancelReason != "" {
			// We triggered the cancel; report as early-cancel rather than timeout.
			result.ExitCode = -1
			return result, nil
		}
		if wallClockTimedOut.Load() {
			result.ExitCode = -1
			return result, context.DeadlineExceeded
		}
		if timedOut.Load() {
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

func envWithOverrides(base []string, overrides map[string]string) []string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}

	skip := make(map[string]bool, len(overrides))
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		skip[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			env = append(env, kv)
			continue
		}
		if skip[kv[:eq]] {
			continue
		}
		env = append(env, kv)
	}
	for _, key := range keys {
		env = append(env, key+"="+overrides[key])
	}
	return env
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
