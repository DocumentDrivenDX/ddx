package agent

import (
	"fmt"
	"sync"
)

// RunQuorum invokes multiple harnesses and evaluates consensus.
func (r *Runner) RunQuorum(opts QuorumOptions) ([]*Result, error) {
	if len(opts.Harnesses) == 0 {
		return nil, fmt.Errorf("agent: quorum requires at least one harness")
	}

	threshold := effectiveThreshold(opts.Strategy, opts.Threshold, len(opts.Harnesses))
	if threshold < 1 || threshold > len(opts.Harnesses) {
		return nil, fmt.Errorf("agent: invalid quorum threshold %d for %d harnesses", threshold, len(opts.Harnesses))
	}

	// Run all harnesses in parallel
	results := make([]*Result, len(opts.Harnesses))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, name := range opts.Harnesses {
		wg.Add(1)
		go func(idx int, harness string) {
			defer wg.Done()
			runOpts := opts.RunOptions
			runOpts.Harness = harness
			result, err := r.Run(runOpts)
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = result
			mu.Unlock()
		}(i, name)
	}
	wg.Wait()

	return results, firstErr
}

// QuorumMet returns true if enough results succeeded.
func QuorumMet(strategy string, threshold int, results []*Result) bool {
	total := len(results)
	eff := effectiveThreshold(strategy, threshold, total)

	successes := 0
	for _, r := range results {
		if r != nil && r.ExitCode == 0 {
			successes++
		}
	}
	return successes >= eff
}

func effectiveThreshold(strategy string, threshold, total int) int {
	switch strategy {
	case "any":
		return 1
	case "majority":
		return (total / 2) + 1
	case "unanimous":
		return total
	default:
		if threshold > 0 {
			return threshold
		}
		return 1
	}
}
