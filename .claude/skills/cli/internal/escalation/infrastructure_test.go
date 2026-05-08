package escalation

import (
	"strings"
	"sync"
	"testing"
)

func TestIsInfrastructureFailure(t *testing.T) {
	tests := []struct {
		name   string
		status string
		detail string
		want   bool
	}{
		{"structural validation is not infrastructure", "structural_validation_failed", "anything 502", false},
		{"escalatable + empty detail returns false", "execution_failed", "", false},
		{"escalatable + plain test failure is not infrastructure", "execution_failed", "TestFoo failed: assertion error", false},
		{"escalatable + 502 from provider is infrastructure", "execution_failed", `provider error: POST "http://bragi:1234/v1/chat/completions": 502 Bad Gateway`, true},
		{"escalatable + 503 service unavailable is infrastructure", "execution_failed", "503 Service Unavailable", true},
		{"escalatable + connection refused is infrastructure", "execution_failed", "dial tcp 127.0.0.1:1234: connect: connection refused", true},
		{"escalatable + no such host is infrastructure", "execution_failed", "dial tcp: lookup vidar: no such host", true},
		{"escalatable + context deadline is infrastructure", "execution_failed", "context deadline exceeded", true},
		{"escalatable + rate limit is infrastructure (operator-fixable)", "execution_failed", "429 Too Many Requests: rate limit reached", true},
		{"escalatable + auth failure is infrastructure", "execution_failed", "401 Unauthorized: invalid api key", true},
		{"escalatable + binary missing is infrastructure", "execution_failed", `exec: "claude": executable file not found in $PATH`, true},
		{"case-insensitive match", "execution_failed", "BAD GATEWAY", true},
		{"no_changes never infrastructure regardless of detail", "no_changes", "agent ran fine, no edits", false},
		{"post_run_check_failed: gate failure is not infrastructure", "post_run_check_failed", "gate verify-tests exited with status 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInfrastructureFailure(tt.status, tt.detail); got != tt.want {
				t.Errorf("IsInfrastructureFailure(%q, %q) = %v, want %v",
					tt.status, tt.detail, got, tt.want)
			}
		})
	}
}

func TestCountsTowardCostCap(t *testing.T) {
	tests := []struct {
		name           string
		isLocal        bool
		isSubscription bool
		costClass      string
		want           bool
	}{
		{"local provider never counts", true, false, "free", false},
		{"subscription provider never counts", false, true, "subscription", false},
		{"both flags set: never counts", true, true, "anything", false},
		{"billed provider counts", false, false, "expensive", true},
		{"unknown costClass with no flags: count by default (safe)", false, false, "", true},
		{"costClass=free with no flags: don't count", false, false, "free", false},
		{"costClass=subscription with no flags: don't count", false, false, "subscription", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountsTowardCostCap(tt.isLocal, tt.isSubscription, tt.costClass); got != tt.want {
				t.Errorf("CountsTowardCostCap(%v,%v,%q) = %v, want %v",
					tt.isLocal, tt.isSubscription, tt.costClass, got, tt.want)
			}
		})
	}
}

// TestCostCapTracker_DisabledWhenMaxZero asserts MaxUSD<=0 is a kill
// switch — no Add ever trips Tripped, even at huge accumulated values.
func TestCostCapTracker_DisabledWhenMaxZero(t *testing.T) {
	tr := NewCostCapTracker(0, func(string) bool { return true })
	tr.Add("openrouter", 1000.0)
	if _, capped := tr.Tripped(); capped {
		t.Fatalf("Tripped should always be false when MaxUSD=0")
	}
}

// TestCostCapTracker_AddRespectsLookup asserts that Add ignores cost
// reports from harnesses whose Lookup returns false, e.g. local /
// subscription providers — they must never push the tracker over the
// cap.
func TestCostCapTracker_AddRespectsLookup(t *testing.T) {
	tr := NewCostCapTracker(10.0, func(name string) bool {
		return name == "openrouter"
	})
	tr.Add("claude", 100.0) // subscription — must not count
	tr.Add("local-llama", 100.0)
	if got := tr.Spent(); got != 0 {
		t.Fatalf("Spent after non-billed adds = %.2f, want 0", got)
	}
	if _, capped := tr.Tripped(); capped {
		t.Fatalf("non-billed adds must not trip the cap")
	}
	tr.Add("openrouter", 5.0)
	if got := tr.Spent(); got != 5.0 {
		t.Fatalf("Spent after billed add = %.2f, want 5.0", got)
	}
	if _, capped := tr.Tripped(); capped {
		t.Fatalf("Tripped at $5 with $10 cap should be false")
	}
	tr.Add("openrouter", 6.0)
	if got := tr.Spent(); got != 11.0 {
		t.Fatalf("Spent after second billed add = %.2f, want 11.0", got)
	}
	detail, capped := tr.Tripped()
	if !capped {
		t.Fatalf("Tripped at $11 with $10 cap should be true")
	}
	if !strings.Contains(detail, "$11.00") || !strings.Contains(detail, "$10.00") {
		t.Fatalf("Tripped detail missing dollar values: %q", detail)
	}
}

// TestCostCapTracker_LookupCached asserts that the per-harness lookup
// is invoked at most once per harness — the tracker must not call
// Lookup on every Add (the lookup may make a network call).
func TestCostCapTracker_LookupCached(t *testing.T) {
	var calls int
	tr := NewCostCapTracker(100.0, func(string) bool {
		calls++
		return true
	})
	tr.Add("openrouter", 1.0)
	tr.Add("openrouter", 1.0)
	tr.Add("openrouter", 1.0)
	if calls != 1 {
		t.Fatalf("Lookup called %d times for one harness, want 1", calls)
	}
}

// TestCostCapTracker_NilLookupCountsByDefault asserts that a nil
// Lookup is treated as "count by default" — the safe option for
// callers that have not wired up harness-info resolution.
func TestCostCapTracker_NilLookupCountsByDefault(t *testing.T) {
	tr := NewCostCapTracker(5.0, nil)
	tr.Add("anything", 6.0)
	if _, capped := tr.Tripped(); !capped {
		t.Fatalf("nil Lookup should count by default; cap should trip")
	}
}

// TestCostCapTracker_ConcurrentSafe runs many goroutines hammering
// Add/Tripped/Spent concurrently — the test must not race (run with
// -race) and the final spent must be the deterministic sum.
func TestCostCapTracker_ConcurrentSafe(t *testing.T) {
	tr := NewCostCapTracker(10000.0, func(string) bool { return true })
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				tr.Add("openrouter", 0.5)
				_, _ = tr.Tripped()
				_ = tr.Spent()
			}
		}()
	}
	wg.Wait()
	want := 50.0 * 20.0 * 0.5
	if got := tr.Spent(); got != want {
		t.Fatalf("Spent after concurrent Adds = %.2f, want %.2f", got, want)
	}
}
