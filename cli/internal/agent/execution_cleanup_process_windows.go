//go:build windows

package agent

func defaultExecutionCleanupProcessScanner() ExecutionCleanupProcessScanner {
	return nil
}

func defaultExecutionCleanupProcessKiller() ExecutionCleanupProcessKiller {
	return nil
}
