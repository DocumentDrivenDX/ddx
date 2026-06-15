package graphql

import (
	"context"
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

const providerProbeCleanupTimeout = 2 * time.Second

func cleanupCurrentProcessProviderProbes() {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	_ = agent.ReapRootProviderChildrenInScope(ctx, os.Getpid(), cwd)
}
