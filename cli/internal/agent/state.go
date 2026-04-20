package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// quotaPattern matches lines like:
//
//	"83% of 5h limit"
//	"75% of 7 day limit, resets April 12"
//	"83% of 5h limit (resets April 12)"
var quotaPattern = regexp.MustCompile(
	`(\d+)%\s+of\s+([\w\s]+?)\s+limit(?:[,\s]+resets?\s+([\w\s]+\d+))?`,
)

// ParseQuotaOutput parses the text output of a harness quota command.
// It extracts percent_used, limit_window, and reset_date.
// Returns nil if no quota data is found.
func ParseQuotaOutput(output string) *QuotaInfo {
	// Normalize whitespace for matching
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		m := quotaPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pct, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		info := &QuotaInfo{
			PercentUsed: pct,
			LimitWindow: strings.TrimSpace(m[2]),
		}
		if m[3] != "" {
			info.ResetDate = strings.TrimSpace(m[3])
		}
		return info
	}
	return nil
}

// ProbeHarnessState evaluates the routing-relevant state of a harness.
// It checks installed, reachable, authenticated, and quota status.
// The timeout applies to quota probing; binary lookup is always fast.
//
// Provider-native routing signals (codex/claude/openrouter/lmstudio) are
// sourced from the upstream agent service (ddx-7bc0c8d5). This function
// only exercises the local CLI quota-command path.
func (r *Runner) ProbeHarnessState(harnessName string, timeout time.Duration) HarnessState {
	state := HarnessState{
		LastChecked: time.Now(),
		PolicyOK:    true, // default: no known policy restriction
	}

	harness, ok := r.Registry.Get(harnessName)
	if !ok {
		state.Error = fmt.Sprintf("unknown harness: %s", harnessName)
		return state
	}

	// Embedded harnesses are always installed, always reachable.
	if harness.IsLocal || harnessName == "virtual" || harnessName == "agent" {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		return state
	}

	// HTTP-only providers: reachability is probed by the upstream service.
	// Without a live probe here, mark optimistically as installed; the
	// full picture comes from svc.ListHarnesses in cmd/server callers.
	if harness.IsHTTPProvider {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		state.QuotaState = "unknown"
		return state
	}

	// Check binary on PATH.
	if _, err := r.LookPath(harness.Binary); err != nil {
		state.Installed = false
		state.Error = fmt.Sprintf("%s not found in PATH", harness.Binary)
		return state
	}
	state.Installed = true

	// If there's a quota command, drive it to get quota data.
	if harness.QuotaCommand != "" {
		quota, probeErr := r.probeQuota(harnessName, harness, timeout)
		if probeErr != nil {
			// Binary found but invocation failed — treat as degraded/unreachable.
			state.Reachable = false
			state.Degraded = true
			state.Error = fmt.Sprintf("quota probe failed: %v", probeErr)
			return state
		}
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		if quota != nil {
			state.Quota = quota
			// Treat >= 95% used as quota-blocked for routing purposes.
			if quota.PercentUsed >= 95 {
				state.QuotaOK = false
			}
			if state.QuotaState == "" {
				state.QuotaState = quotaStateFromUsedPercent(quota.PercentUsed)
			}
			r.recordQuotaSnapshot(harnessName, harness, quota, "async-probe")
		}
		if state.QuotaState == "" {
			state.QuotaState = "ok"
		}
		return state
	}

	// No quota command: fall back to TUI slash command if available.
	state.Reachable = true
	state.Authenticated = true
	if state.QuotaState == "" {
		state.QuotaState = "unknown"
	}
	if state.QuotaState == "unknown" && harness.TUIQuotaCommand != "" {
		quota, _ := r.probeQuotaWithArgs(harnessName, harness, strings.Fields(harness.TUIQuotaCommand), timeout)
		if quota != nil {
			state.Quota = quota
			state.QuotaState = quotaStateFromUsedPercent(quota.PercentUsed)
			if quota.PercentUsed >= 95 {
				state.QuotaOK = false
			}
			r.recordQuotaSnapshot(harnessName, harness, quota, "tui-probe")
		}
	}
	if state.QuotaState != "blocked" {
		state.QuotaOK = true
	}
	return state
}

// probeQuota invokes the harness binary with its QuotaCommand CLI args and parses the output.
func (r *Runner) probeQuota(harnessName string, harness Harness, timeout time.Duration) (*QuotaInfo, error) {
	return r.probeQuotaWithArgs(harnessName, harness, strings.Fields(harness.QuotaCommand), timeout)
}

// probeQuotaWithArgs invokes the harness binary with explicit args and parses quota output.
// Used both for QuotaCommand (non-interactive CLI subcommands) and TUIQuotaCommand (slash
// commands sent as a prompt via the binary's print/non-interactive mode).
func (r *Runner) probeQuotaWithArgs(harnessName string, harness Harness, args []string, timeout time.Duration) (*QuotaInfo, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := r.Executor.ExecuteInDir(ctx, harness.Binary, args, "", "")
	if err != nil {
		return nil, fmt.Errorf("invoke %s: %w", harness.Binary, err)
	}

	combined := result.Stdout
	if result.Stderr != "" {
		combined += "\n" + result.Stderr
	}

	return ParseQuotaOutput(combined), nil
}

// quotaStateFromUsedPercent maps a usage percentage to a quota state string.
func quotaStateFromUsedPercent(usedPercent int) string {
	if usedPercent >= 95 {
		return "blocked"
	}
	if usedPercent >= 0 {
		return "ok"
	}
	return "unknown"
}
