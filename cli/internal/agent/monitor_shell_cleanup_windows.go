//go:build windows

package agent

import (
	"context"
	"time"
)

func scanMonitorShellsImpl(_ context.Context, _ int, _ time.Time) ([]monitorShellProcess, error) {
	return nil, nil
}

func terminateMonitorShellImpl(_ int) {}
