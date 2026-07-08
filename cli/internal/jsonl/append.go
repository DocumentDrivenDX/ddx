package jsonl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const DefaultLockTimeout = 5 * time.Second

var errLockHeld = errors.New("jsonl lock held")

type appendConfig struct {
	lockTimeout  time.Duration
	beforeAppend func() error
}

// Option configures AppendJSONL.
type Option func(*appendConfig)

// WithLockTimeout overrides the default lock wait budget.
func WithLockTimeout(timeout time.Duration) Option {
	return func(cfg *appendConfig) {
		if timeout > 0 {
			cfg.lockTimeout = timeout
		}
	}
}

// WithBeforeAppend runs fn after the lock is acquired and before the row is
// appended. It is intended for callers that need to mutate or rotate the target
// file while holding the shared append lock.
func WithBeforeAppend(fn func() error) Option {
	return func(cfg *appendConfig) {
		cfg.beforeAppend = fn
	}
}

// TimeoutError reports that the append could not acquire the file lock within
// the configured wait budget.
type TimeoutError struct {
	Path     string
	LockPath string
	Timeout  time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("jsonl append timeout waiting for %s (lock %s) after %s", e.Path, e.LockPath, e.Timeout)
}

func (e *TimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}

// WriteError reports that the append lock was acquired but the row could not be
// persisted completely.
type WriteError struct {
	Path     string
	LockPath string
	Err      error
}

func (e *WriteError) Error() string {
	return fmt.Sprintf("jsonl append failed for %s (lock %s): %v", e.Path, e.LockPath, e.Err)
}

func (e *WriteError) Unwrap() error {
	return e.Err
}

var lockMutexes sync.Map

func mutexFor(lockPath string) *sync.Mutex {
	if v, ok := lockMutexes.Load(lockPath); ok {
		return v.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	actual, _ := lockMutexes.LoadOrStore(lockPath, mu)
	return actual.(*sync.Mutex)
}

// AppendJSONL appends one JSON object as a newline-terminated row to path.
// It creates parent directories, serializes same-process callers with a mutex,
// and uses a cross-process advisory lock at path+".lock" before writing.
func AppendJSONL(ctx context.Context, path string, row any, opts ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := appendConfig{lockTimeout: DefaultLockTimeout}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}

	mu := mutexFor(lockPath)
	mu.Lock()
	defer mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close() //nolint:errcheck

	deadline := time.Now().Add(cfg.lockTimeout)
	for {
		if err := tryLockFile(lockFile); err == nil {
			break
		} else if !errors.Is(err, errLockHeld) {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return &TimeoutError{Path: path, LockPath: lockPath, Timeout: cfg.lockTimeout}
		}

		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer unlockFile(lockFile) //nolint:errcheck

	if cfg.beforeAppend != nil {
		if err := cfg.beforeAppend(); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	before := fi.Size()

	n, err := f.Write(data)
	if err != nil || n != len(data) {
		if truncErr := f.Truncate(before); truncErr != nil && err == nil {
			err = truncErr
		}
		if err == nil {
			err = io.ErrShortWrite
		}
		return &WriteError{Path: path, LockPath: lockPath, Err: err}
	}
	return nil
}
