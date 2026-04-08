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
	if harness.IsLocal || harnessName == "virtual" || harnessName == "forge" {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
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
		}
		return state
	}

	// No quota command: mark reachable based on binary presence only.
	state.Reachable = true
	state.Authenticated = true
	state.QuotaOK = true
	return state
}

// probeQuota invokes the harness binary with its quota CLI args and parses the output.
// QuotaCommand must be a whitespace-separated list of CLI arguments for non-interactive
// quota introspection (e.g. "usage" for a "binary usage" subcommand). It must NOT be
// an interactive slash command like "/usage" — those only work in interactive sessions
// and would be passed as an LLM prompt, consuming tokens without returning quota data.
func (r *Runner) probeQuota(harnessName string, harness Harness, timeout time.Duration) (*QuotaInfo, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Invoke the binary directly with QuotaCommand as CLI args.
	// This is a non-LLM invocation: no LLM flags, no prompt, no API call.
	args := strings.Fields(harness.QuotaCommand)

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
