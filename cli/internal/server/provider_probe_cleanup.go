package server

import (
	"context"
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

const providerProbeCleanupTimeout = 2 * time.Second

var preClaimProviderProbeCleanupInterval = 2 * time.Second
var providerProbeCleanupFollowupDelays = []time.Duration{
	250 * time.Millisecond,
	1 * time.Second,
	3 * time.Second,
}

var reapCurrentProcessProviderProbes = func(scopeDirs ...string) {
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		scopeDirs = append(scopeDirs, cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	_ = agent.ReapRootProviderChildrenInScopes(ctx, os.Getpid(), scopeDirs...)
}

var reapCurrentProcessNonRouteProviderProbes = func(harness, provider, model string, scopeDirs ...string) {
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		scopeDirs = append(scopeDirs, cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	_ = agent.ReapRootNonRouteProviderChildrenInScopes(ctx, os.Getpid(), harness, provider, model, scopeDirs...)
}

func cleanupCurrentProcessProviderProbes(scopeDirs ...string) {
	reapCurrentProcessProviderProbes(scopeDirs...)
	scheduleProviderProbeFollowupCleanup(reapCurrentProcessProviderProbes, scopeDirs...)
}

func cleanupCurrentProcessNonRouteProviderProbes(harness, provider, model string, scopeDirs ...string) {
	reapCurrentProcessNonRouteProviderProbes(harness, provider, model, scopeDirs...)
}

func scheduleProviderProbeFollowupCleanup(cleanup func(...string), scopeDirs ...string) {
	if cleanup == nil || len(providerProbeCleanupFollowupDelays) == 0 {
		return
	}
	copiedScopes := append([]string(nil), scopeDirs...)
	delays := append([]time.Duration(nil), providerProbeCleanupFollowupDelays...)
	go func() {
		for _, delay := range delays {
			if delay > 0 {
				timer := time.NewTimer(delay)
				<-timer.C
			}
			cleanup(copiedScopes...)
		}
	}()
}
