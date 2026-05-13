//go:build !linux

package workerstatus

import "context"

// newSystemScanner returns a stub scanner for non-Linux platforms. DDx
// workers run on Linux in practice; macOS developer machines fall back to
// the server-backed worker registry exposed by `ddx server workers list`.
func newSystemScanner() Scanner {
	return stubScanner{}
}

type stubScanner struct{}

func (stubScanner) Scan(_ context.Context) ([]LiveWorker, error) {
	return nil, nil
}
