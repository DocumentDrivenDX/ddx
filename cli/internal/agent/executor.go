package agent

import (
	"sort"
	"strings"
)

// envWithOverrides is generic process plumbing shared by DDx's git/worktree
// helpers. It does not select or invoke an agent harness.
func envWithOverrides(base []string, overrides map[string]string) []string {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}

	skip := make(map[string]bool, len(overrides))
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		skip[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		equals := strings.IndexByte(entry, '=')
		if equals < 0 {
			env = append(env, entry)
			continue
		}
		if !skip[entry[:equals]] {
			env = append(env, entry)
		}
	}
	for _, key := range keys {
		env = append(env, key+"="+overrides[key])
	}
	return env
}
