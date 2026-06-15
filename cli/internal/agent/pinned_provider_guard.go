package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const pinnedProviderGuardLogEnv = "DDX_PINNED_PROVIDER_GUARD_LOG"

func applyPinnedProviderGuard(env map[string]string, harness, stateDir string) (map[string]string, error) {
	harness = strings.TrimSpace(resolveHarnessAlias(harness))
	if harness == "" {
		return env, nil
	}
	if _, ok := knownSubscriptionHarnesses[harness]; !ok {
		return env, nil
	}
	guardDir := filepath.Join(stateDir, "provider-guard-bin")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return nil, err
	}
	out := cloneStringMap(env)
	logPath := filepath.Join(stateDir, "provider-guard.jsonl")
	out[pinnedProviderGuardLogEnv] = logPath
	for blocked := range knownSubscriptionHarnesses {
		if blocked == harness {
			continue
		}
		if err := writePinnedProviderGuardShim(filepath.Join(guardDir, blocked), blocked, harness); err != nil {
			return nil, err
		}
	}
	path := strings.TrimSpace(out["PATH"])
	if path == "" {
		path = os.Getenv("PATH")
	}
	if path == "" {
		out["PATH"] = guardDir
	} else {
		out["PATH"] = guardDir + string(os.PathListSeparator) + path
	}
	return out, nil
}

func writePinnedProviderGuardShim(path, blocked, requested string) error {
	script := fmt.Sprintf(`#!/bin/sh
log="${%s:-/dev/null}"
printf '{"blocked_harness":%q,"requested_harness":%q,"argv":"%%s","pwd":"%%s"}\n' "$*" "$(pwd)" >> "$log" 2>/dev/null || true
echo "ddx: blocked unrequested provider harness %q during pinned %q execution" >&2
exit 126
`, pinnedProviderGuardLogEnv, blocked, requested, blocked, requested)
	return os.WriteFile(path, []byte(script), 0o755)
}
