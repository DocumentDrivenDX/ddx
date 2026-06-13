//go:build windows

package agent

import "context"

type attemptProcessSnapshot struct{}

func captureAttemptProcessBaseline(context.Context, string) attemptProcessSnapshot {
	return attemptProcessSnapshot{}
}

func cleanupAttemptProcesses(context.Context, string, string, string, string, attemptProcessSnapshot, string) any {
	return nil
}
