//go:build !linux

package agent

import "context"

type executionCleanupAttemptProcessUnavailableScanner struct{}

func newExecutionCleanupAttemptProcessScannerImpl() executionCleanupAttemptProcessScanner {
	return executionCleanupAttemptProcessUnavailableScanner{}
}

func (executionCleanupAttemptProcessUnavailableScanner) Scan(context.Context) ([]executionCleanupAttemptProcess, error) {
	return nil, errExecutionCleanupAttemptProcessUnavailable
}

func newExecutionCleanupAttemptProcessKiller() executionCleanupAttemptProcessKiller {
	return executionCleanupAttemptProcessKillerFunc(func(int) error { return nil })
}
