package server

import (
	"context"
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

const providerProbeCleanupTimeout = 2 * time.Second

var reapCurrentProcessProviderProbes = func(scopeDirs ...string) {
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		scopeDirs = append(scopeDirs, cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	_ = agent.ReapRootProviderChildrenInScopes(ctx, os.Getpid(), scopeDirs...)
}

func cleanupCurrentProcessProviderProbes(scopeDirs ...string) {
	reapCurrentProcessProviderProbes(scopeDirs...)
}
