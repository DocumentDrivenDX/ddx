package evidence

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrOversize is returned by hard-fail readers (e.g. user-supplied prompt
// files, FEAT-022 §8). Callers compare with errors.Is to surface a
// distinct error class to the user.
var ErrOversize = errors.New("evidence: input exceeds cap")

// OversizeError carries the input-source name, observed size, and cap
// for actionable error messages (FEAT-022 Non-Functional / Failure mode
// clarity).
type OversizeError struct {
	Source        string
	ObservedBytes int64
	CapBytes      int
}

func (e *OversizeError) Error() string {
	return fmt.Sprintf("evidence: %s exceeds cap: observed %d bytes, cap %d bytes",
		e.Source, e.ObservedBytes, e.CapBytes)
}

func (e *OversizeError) Unwrap() error { return ErrOversize }

// ReadFileClamped reads up to max+1 bytes from path. If the file fits
// within max it returns (content, false, len(content), nil). If the file
// exceeds max it returns the first max bytes plus truncated=true and
// originalBytes set to the on-disk size (as observed via os.Stat). The
// implementation does not fully load oversize files into memory
// (FEAT-022 Non-Functional / Performance).
func ReadFileClamped(path string, max int) (content []byte, truncated bool, originalBytes int64, err error) {
	if max < 0 {
		return nil, false, 0, fmt.Errorf("evidence: negative cap %d", max)
	}
	// nolint:gosec // path is supplied by callers that have already validated it.
	f, err := os.Open(path)
	if err != nil {
		return nil, false, 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, false, 0, err
	}
	originalBytes = st.Size()

	// Read max+1 bytes so we can detect overflow without loading the whole file.
	buf := make([]byte, max+1)
	n, readErr := io.ReadFull(f, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return nil, false, originalBytes, readErr
	}

	if n > max {
		return buf[:max], true, originalBytes, nil
	}
	return buf[:n], false, int64(n), nil
}

// ReadFileHardFail reads path and returns *OversizeError (wrapping
// ErrOversize) if the file exceeds max. Used for user-supplied prompt
// files where silent truncation is forbidden (FEAT-022 §8).
func ReadFileHardFail(path string, max int, source string) ([]byte, error) {
	content, truncated, originalBytes, err := ReadFileClamped(path, max)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, &OversizeError{
			Source:        source,
			ObservedBytes: originalBytes,
			CapBytes:      max,
		}
	}
	return content, nil
}
