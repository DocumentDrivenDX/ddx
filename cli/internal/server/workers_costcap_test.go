package server

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// These tests exercise the cost-cap and infrastructure-failure wiring
// used by runWorker's executor closure (see workers.go around the
// `executor := agent.ExecuteBeadExecutorFunc(...)` block).
//
// The closure itself is built inline (so it cannot be unit-tested
// directly), but we exercise the same composition pattern here against
// a fake inner attempt to assert:
//
//  1. When accumulated cost exceeds the cap, the next executor call
//     returns the cap-tripped error WITHOUT invoking the inner attempt.
//  2. Infrastructure failures are detected by IsInfrastructureFailure
//     and short-circuit before the next-tier escalation step.
//
// Patterns mirror the closures in cli/internal/server/workers.go
// (ddx-785d02f7).

type fakeServerAttempt struct {
	calls   atomic.Int32
	reports []agent.ExecuteBeadReport
}

func (f *fakeServerAttempt) next(_ string) agent.ExecuteBeadReport {
	idx := f.calls.Add(1) - 1
	if int(idx) >= len(f.reports) {
		return f.reports[len(f.reports)-1]
	}
	return f.reports[idx]
}

func wrapServerExecutor(tracker *escalation.CostCapTracker, inner func(beadID string) agent.ExecuteBeadReport) agent.ExecuteBeadExecutorFunc {
	return func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		if detail, capped := tracker.Tripped(); capped {
			return agent.ExecuteBeadReport{
				BeadID: beadID,
				Status: agent.ExecuteBeadStatusExecutionFailed,
				Detail: detail,
			}, nil
		}
		report := inner(beadID)
		tracker.Add(report.Harness, report.CostUSD)
		if detail, capped := tracker.Tripped(); capped {
			return agent.ExecuteBeadReport{
				BeadID: beadID,
				Status: agent.ExecuteBeadStatusExecutionFailed,
				Detail: detail,
			}, nil
		}
		return report, nil
	}
}

// TestWorkerExecutorCostCap_StopsAfterCap asserts the server worker's
// cost-cap logic halts further bead claiming once accumulated billed
// spend reaches the cap.
func TestWorkerExecutorCostCap_StopsAfterCap(t *testing.T) {
	tracker := escalation.NewCostCapTracker(5.0, func(string) bool { return true })

	fake := &fakeServerAttempt{
		reports: []agent.ExecuteBeadReport{
			{BeadID: "ddx-1", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 3.0},
			{BeadID: "ddx-2", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 3.0},
			{BeadID: "ddx-3", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 1.0},
		},
	}
	exec := wrapServerExecutor(tracker, fake.next)

	// First call: $3 < $5 cap.
	if r, err := exec(context.Background(), "ddx-1"); err != nil || r.Status != agent.ExecuteBeadStatusSuccess {
		t.Fatalf("call 1: status=%s err=%v", r.Status, err)
	}
	// Second call: $6 >= $5; post-attempt cap fires.
	r2, _ := exec(context.Background(), "ddx-2")
	if r2.Status != agent.ExecuteBeadStatusExecutionFailed {
		t.Fatalf("call 2 should trip cap; status=%s", r2.Status)
	}
	if !strings.Contains(r2.Detail, "cost cap reached") {
		t.Fatalf("call 2 detail = %q", r2.Detail)
	}

	callsBefore := fake.calls.Load()
	// Third call: cap-on-entry blocks before invoking inner.
	r3, _ := exec(context.Background(), "ddx-3")
	if r3.Status != agent.ExecuteBeadStatusExecutionFailed {
		t.Fatalf("call 3 should be cap-tripped on entry; status=%s", r3.Status)
	}
	if r3.BeadID != "ddx-3" {
		t.Fatalf("cap-tripped report missing beadID: %q", r3.BeadID)
	}
	if got := fake.calls.Load(); got != callsBefore {
		t.Fatalf("inner attempt was invoked despite cap; calls %d -> %d", callsBefore, got)
	}
}

// TestWorkerExecutorCostCap_SubscriptionDoesNotCount asserts that
// subscription/local-provider costs do not contribute to the cap, even
// at huge reported CostUSD values.
func TestWorkerExecutorCostCap_SubscriptionDoesNotCount(t *testing.T) {
	tracker := escalation.NewCostCapTracker(10.0, func(name string) bool {
		// Only "openrouter" counts; everything else is local/subscription.
		return name == "openrouter"
	})

	fake := &fakeServerAttempt{
		reports: []agent.ExecuteBeadReport{
			{BeadID: "ddx-x", Harness: "claude", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 1000.0},
			{BeadID: "ddx-y", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 1.0},
		},
	}
	exec := wrapServerExecutor(tracker, fake.next)

	for _, id := range []string{"ddx-x", "ddx-y"} {
		r, _ := exec(context.Background(), id)
		if r.Status != agent.ExecuteBeadStatusSuccess {
			t.Fatalf("call %s: subscription/billed mix should not trip; status=%s", id, r.Status)
		}
	}
	if got := tracker.Spent(); got != 1.0 {
		t.Fatalf("Spent = %.2f, want 1.0 (only openrouter counts)", got)
	}
}

// TestWorkerInfrastructureFailureDoesNotEscalate asserts the contract
// the server tier-loop relies on: IsInfrastructureFailure flags
// transient provider failures (so the loop defers with retry-after)
// without consuming escalation budget, while real model-capability
// failures fall through to the next-tier path.
func TestWorkerInfrastructureFailureDoesNotEscalate(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		detail    string
		wantInfra bool
	}{
		{"503 from provider triggers defer", agent.ExecuteBeadStatusExecutionFailed, "503 service unavailable", true},
		{"i/o timeout triggers defer", agent.ExecuteBeadStatusExecutionFailed, "Get http://x: i/o timeout", true},
		{"missing binary triggers defer", agent.ExecuteBeadStatusExecutionFailed, `exec: "claude": executable file not found in $PATH`, true},
		{"build failure does NOT defer (escalates)", agent.ExecuteBeadStatusExecutionFailed, "build error: missing import", false},
		{"non-escalatable structural never infra", "structural_validation_failed", "503 service unavailable", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escalation.IsInfrastructureFailure(tc.status, tc.detail)
			if got != tc.wantInfra {
				t.Fatalf("IsInfrastructureFailure(%q, %q) = %v, want %v", tc.status, tc.detail, got, tc.wantInfra)
			}
			// The loop ordering invariant: when IsInfrastructureFailure
			// returns true, ShouldEscalate must also be true (otherwise
			// the infra branch is unreachable from the tier loop).
			if tc.wantInfra && !escalation.ShouldEscalate(tc.status) {
				t.Fatalf("infra-failure case %q has !ShouldEscalate; loop ordering invariant broken", tc.name)
			}
		})
	}
}
