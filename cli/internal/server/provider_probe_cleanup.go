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
var providerProbeCleanupSettleQuiet = 1 * time.Second
var providerProbeCleanupSettleDeadline = 5 * time.Second
var providerProbeCleanupSettleInterval = 250 * time.Millisecond

var reapCurrentProcessProviderProbes = func(scopeDirs ...string) int {
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		scopeDirs = append(scopeDirs, cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	return agent.ReapRootProviderChildrenInScopes(ctx, os.Getpid(), scopeDirs...)
}

var reapCurrentProcessNonRouteProviderProbes = func(harness, provider, model string, scopeDirs ...string) int {
	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		scopeDirs = append(scopeDirs, cwd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	return agent.ReapRootNonRouteProviderChildrenInScopes(ctx, os.Getpid(), harness, provider, model, scopeDirs...)
}

func cleanupCurrentProcessProviderProbes(scopeDirs ...string) int {
	reaped := reapCurrentProcessProviderProbes(scopeDirs...)
	scheduleProviderProbeFollowupCleanup(reapCurrentProcessProviderProbes, scopeDirs...)
	return reaped
}

func cleanupCurrentProcessProviderProbesSettled(scopeDirs ...string) int {
	reaped := cleanupCurrentProcessProviderProbes(scopeDirs...)
	quietFor := time.Duration(0)
	deadline := time.Now().Add(providerProbeCleanupSettleDeadline)
	interval := providerProbeCleanupSettleInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for time.Now().Before(deadline) {
		timer := time.NewTimer(interval)
		<-timer.C
		n := reapCurrentProcessProviderProbes(scopeDirs...)
		reaped += n
		if n > 0 {
			quietFor = 0
			continue
		}
		quietFor += interval
		if quietFor >= providerProbeCleanupSettleQuiet {
			break
		}
	}
	return reaped
}

func cleanupCurrentProcessNonRouteProviderProbes(harness, provider, model string, scopeDirs ...string) int {
	return reapCurrentProcessNonRouteProviderProbes(harness, provider, model, scopeDirs...)
}

func scheduleProviderProbeFollowupCleanup(cleanup func(...string) int, scopeDirs ...string) {
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
			_ = cleanup(copiedScopes...)
		}
	}()
}
