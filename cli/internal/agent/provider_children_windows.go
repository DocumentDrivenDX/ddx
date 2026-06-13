//go:build windows

package agent

import (
	"context"
	"time"
)

func scanProviderChildProcessesImpl(context.Context, int, time.Time) ([]providerChildProcess, error) {
	return nil, nil
}

func terminateProviderChildImpl(int) {}
