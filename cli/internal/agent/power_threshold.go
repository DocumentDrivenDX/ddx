package agent

import (
	"sort"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// TopNPowerThreshold returns the minimum power value that, when used as
// MinPower, restricts routing to models within the top-n distinct power
// levels present in the catalog. Pass n=1 to allow only the highest-power
// models; n=2 to include the two strongest tiers, and so on.
//
// Returns 0 when models is empty or every entry carries a zero power value.
//
// The result depends only on power values; model IDs, provider names, and
// harness names are never inspected.
func TopNPowerThreshold(models []agentlib.ModelInfo, n int) int {
	if len(models) == 0 || n <= 0 {
		return 0
	}

	// Collect non-zero power values (zero means unrated/unknown).
	powers := make([]int, 0, len(models))
	for _, m := range models {
		if m.Power > 0 {
			powers = append(powers, m.Power)
		}
	}
	if len(powers) == 0 {
		return 0
	}

	// Sort descending so distinct-level extraction is a single pass.
	sort.Sort(sort.Reverse(sort.IntSlice(powers)))

	// Build a deduplicated list of distinct power levels in descending order.
	distinct := make([]int, 0, len(powers))
	for _, p := range powers {
		if len(distinct) == 0 || p != distinct[len(distinct)-1] {
			distinct = append(distinct, p)
		}
	}

	// Return the n-th level (1-indexed). When fewer than n distinct levels
	// exist, return the lowest level so the threshold remains satisfiable.
	if n >= len(distinct) {
		return distinct[len(distinct)-1]
	}
	return distinct[n-1]
}
