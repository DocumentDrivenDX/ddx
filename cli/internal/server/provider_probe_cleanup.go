package server

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

const providerProbeCleanupTimeout = 2 * time.Second

var providerProbeCleanupMu sync.RWMutex

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

var reapCurrentProcessProviderProbesUnscoped = func() int {
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeCleanupTimeout)
	defer cancel()
	return agent.ReapRootProviderChildrenInScope(ctx, os.Getpid(), "")
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
	cleanup := currentReapCurrentProcessProviderProbes()
	reaped := cleanup(scopeDirs...)
	scheduleProviderProbeFollowupCleanup(cleanup, scopeDirs...)
	return reaped
}

func cleanupCurrentProcessProviderProbesSettled(scopeDirs ...string) int {
	reaped := cleanupCurrentProcessProviderProbes(scopeDirs...)
	cleanup := currentReapCurrentProcessProviderProbes()
	quietFor := time.Duration(0)
	quiet, settleDeadline, interval := providerProbeCleanupSettleConfig()
	deadline := time.Now().Add(settleDeadline)
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for time.Now().Before(deadline) {
		timer := time.NewTimer(interval)
		<-timer.C
		n := cleanup(scopeDirs...)
		reaped += n
		if n > 0 {
			quietFor = 0
			continue
		}
		quietFor += interval
		if quietFor >= quiet {
			break
		}
	}
	return reaped
}

func cleanupCurrentProcessProviderProbesUnscopedSettled() int {
	cleanup := currentReapCurrentProcessProviderProbesUnscoped()
	reaped := cleanup()
	quietFor := time.Duration(0)
	quiet, settleDeadline, interval := providerProbeCleanupSettleConfig()
	deadline := time.Now().Add(settleDeadline)
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for time.Now().Before(deadline) {
		timer := time.NewTimer(interval)
		<-timer.C
		n := cleanup()
		reaped += n
		if n > 0 {
			quietFor = 0
			continue
		}
		quietFor += interval
		if quietFor >= quiet {
			break
		}
	}
	return reaped
}

func cleanupCurrentProcessNonRouteProviderProbes(harness, provider, model string, scopeDirs ...string) int {
	return currentReapCurrentProcessNonRouteProviderProbes()(harness, provider, model, scopeDirs...)
}

func scheduleProviderProbeFollowupCleanup(cleanup func(...string) int, scopeDirs ...string) {
	delays := currentProviderProbeCleanupFollowupDelays()
	if cleanup == nil || len(delays) == 0 {
		return
	}
	copiedScopes := append([]string(nil), scopeDirs...)
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

func currentReapCurrentProcessProviderProbes() func(...string) int {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return reapCurrentProcessProviderProbes
}

func currentReapCurrentProcessProviderProbesUnscoped() func() int {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return reapCurrentProcessProviderProbesUnscoped
}

func currentReapCurrentProcessNonRouteProviderProbes() func(string, string, string, ...string) int {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return reapCurrentProcessNonRouteProviderProbes
}

func currentProviderProbeCleanupFollowupDelays() []time.Duration {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return append([]time.Duration(nil), providerProbeCleanupFollowupDelays...)
}

func providerProbeCleanupSettleConfig() (quiet, deadline, interval time.Duration) {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return providerProbeCleanupSettleQuiet, providerProbeCleanupSettleDeadline, providerProbeCleanupSettleInterval
}

func currentPreClaimProviderProbeCleanupInterval() time.Duration {
	providerProbeCleanupMu.RLock()
	defer providerProbeCleanupMu.RUnlock()
	return preClaimProviderProbeCleanupInterval
}

func setReapCurrentProcessProviderProbesForTest(fn func(...string) int) func() {
	providerProbeCleanupMu.Lock()
	old := reapCurrentProcessProviderProbes
	reapCurrentProcessProviderProbes = fn
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		reapCurrentProcessProviderProbes = old
		providerProbeCleanupMu.Unlock()
	}
}

func setReapCurrentProcessProviderProbesUnscopedForTest(fn func() int) func() {
	providerProbeCleanupMu.Lock()
	old := reapCurrentProcessProviderProbesUnscoped
	reapCurrentProcessProviderProbesUnscoped = fn
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		reapCurrentProcessProviderProbesUnscoped = old
		providerProbeCleanupMu.Unlock()
	}
}

func setReapCurrentProcessNonRouteProviderProbesForTest(fn func(string, string, string, ...string) int) func() {
	providerProbeCleanupMu.Lock()
	old := reapCurrentProcessNonRouteProviderProbes
	reapCurrentProcessNonRouteProviderProbes = fn
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		reapCurrentProcessNonRouteProviderProbes = old
		providerProbeCleanupMu.Unlock()
	}
}

func setPreClaimProviderProbeCleanupIntervalForTest(interval time.Duration) func() {
	providerProbeCleanupMu.Lock()
	old := preClaimProviderProbeCleanupInterval
	preClaimProviderProbeCleanupInterval = interval
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		preClaimProviderProbeCleanupInterval = old
		providerProbeCleanupMu.Unlock()
	}
}

func setProviderProbeCleanupFollowupDelaysForTest(delays []time.Duration) func() {
	providerProbeCleanupMu.Lock()
	old := append([]time.Duration(nil), providerProbeCleanupFollowupDelays...)
	providerProbeCleanupFollowupDelays = append([]time.Duration(nil), delays...)
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		providerProbeCleanupFollowupDelays = old
		providerProbeCleanupMu.Unlock()
	}
}

func setProviderProbeCleanupSettleTimingsForTest(quiet, deadline, interval time.Duration) func() {
	providerProbeCleanupMu.Lock()
	oldQuiet := providerProbeCleanupSettleQuiet
	oldDeadline := providerProbeCleanupSettleDeadline
	oldInterval := providerProbeCleanupSettleInterval
	providerProbeCleanupSettleQuiet = quiet
	providerProbeCleanupSettleDeadline = deadline
	providerProbeCleanupSettleInterval = interval
	providerProbeCleanupMu.Unlock()
	return func() {
		providerProbeCleanupMu.Lock()
		providerProbeCleanupSettleQuiet = oldQuiet
		providerProbeCleanupSettleDeadline = oldDeadline
		providerProbeCleanupSettleInterval = oldInterval
		providerProbeCleanupMu.Unlock()
	}
}
