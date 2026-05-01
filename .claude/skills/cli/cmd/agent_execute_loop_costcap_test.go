package cmd

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

// These tests exercise the cost-cap and infrastructure-failure wiring used
// by runAgentExecuteLoop's executor closure. The closure itself is built
// inline (so it cannot be unit-tested directly), but we exercise the same
// composition pattern here against a fake inner attempt to assert:
//
//  1. When accumulated cost exceeds the cap, the next executor invocation
//     returns the cap-tripped error WITHOUT invoking the inner attempt.
//  2. Infrastructure failures do not escalate to a smart tier — the tier
//     loop short-circuits with a deferred RetryAfter.
//
// The patterns mirror the closures in cli/cmd/agent_cmd.go (ddx-785d02f7).

// fakeAttempt records calls and returns a configured sequence of reports.
type fakeAttempt struct {
	calls   atomic.Int32
	reports []agent.ExecuteBeadReport
}

func (f *fakeAttempt) next(_ string) agent.ExecuteBeadReport {
	idx := f.calls.Add(1) - 1
	if int(idx) >= len(f.reports) {
		return f.reports[len(f.reports)-1]
	}
	return f.reports[idx]
}

// buildCostCappedExecutor mirrors the outer-executor wrapping done in
// runAgentExecuteLoop: cap check at the top, inner attempt, accumulate,
// cap check on the way out. Used by the tests below.
func buildCostCappedExecutor(tracker *escalation.CostCapTracker, inner func(beadID string) agent.ExecuteBeadReport) agent.ExecuteBeadExecutorFunc {
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

// TestExecuteLoopCostCap_ShortCircuitsAfterCap asserts the cost cap halts
// the queue: once accumulated billed cost reaches the cap, the next
// executor call returns the cap-tripped status without invoking the
// inner attempt.
func TestExecuteLoopCostCap_ShortCircuitsAfterCap(t *testing.T) {
	tracker := escalation.NewCostCapTracker(10.0, func(string) bool { return true })

	fake := &fakeAttempt{
		reports: []agent.ExecuteBeadReport{
			{BeadID: "ddx-1", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 6.0},
			{BeadID: "ddx-2", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 6.0},
			// Should never be reached.
			{BeadID: "ddx-3", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 1.0},
		},
	}
	exec := buildCostCappedExecutor(tracker, fake.next)

	// First call: $6 < $10 cap — succeeds and accumulates.
	r1, err := exec(context.Background(), "ddx-1")
	if err != nil {
		t.Fatalf("call 1 err = %v", err)
	}
	if r1.Status != agent.ExecuteBeadStatusSuccess {
		t.Fatalf("call 1 status = %s, want success", r1.Status)
	}

	// Second call: accumulates to $12 >= $10 cap. Returns success but
	// cap is now tripped — but wait, the post-attempt cap check fires,
	// returning the cap-tripped report instead of the success report.
	r2, err := exec(context.Background(), "ddx-2")
	if err != nil {
		t.Fatalf("call 2 err = %v", err)
	}
	if r2.Status != agent.ExecuteBeadStatusExecutionFailed {
		t.Fatalf("call 2 should have tripped post-attempt cap; status = %s", r2.Status)
	}
	if !strings.Contains(r2.Detail, "cost cap reached") {
		t.Fatalf("call 2 detail missing cost-cap message: %q", r2.Detail)
	}

	callsBefore := fake.calls.Load()

	// Third call: cap-on-entry check trips IMMEDIATELY, inner attempt
	// is NOT invoked.
	r3, err := exec(context.Background(), "ddx-3")
	if err != nil {
		t.Fatalf("call 3 err = %v", err)
	}
	if r3.Status != agent.ExecuteBeadStatusExecutionFailed {
		t.Fatalf("call 3 should be cap-tripped; status = %s", r3.Status)
	}
	if r3.BeadID != "ddx-3" {
		t.Fatalf("call 3 beadID = %s, want ddx-3", r3.BeadID)
	}
	if got := fake.calls.Load(); got != callsBefore {
		t.Fatalf("cap-on-entry must NOT invoke inner attempt; calls went %d -> %d", callsBefore, got)
	}
}

// TestExecuteLoopCostCap_LocalProvidersDoNotCount asserts that local
// and subscription providers' CostUSD never accumulates against the
// cap, even if they report a non-zero cost field.
func TestExecuteLoopCostCap_LocalProvidersDoNotCount(t *testing.T) {
	// Lookup says only "openrouter" counts.
	tracker := escalation.NewCostCapTracker(10.0, func(name string) bool {
		return name == "openrouter"
	})

	fake := &fakeAttempt{
		reports: []agent.ExecuteBeadReport{
			{BeadID: "ddx-a", Harness: "claude", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 100.0},
			{BeadID: "ddx-b", Harness: "qwen-local", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 100.0},
			{BeadID: "ddx-c", Harness: "openrouter", Status: agent.ExecuteBeadStatusSuccess, CostUSD: 1.0},
		},
	}
	exec := buildCostCappedExecutor(tracker, fake.next)

	for _, id := range []string{"ddx-a", "ddx-b", "ddx-c"} {
		r, err := exec(context.Background(), id)
		if err != nil {
			t.Fatalf("call %s err = %v", id, err)
		}
		if r.Status != agent.ExecuteBeadStatusSuccess {
			t.Fatalf("call %s status = %s, want success (local/sub providers must not trip the cap)", id, r.Status)
		}
	}
	if got := tracker.Spent(); got != 1.0 {
		t.Fatalf("Spent = %.2f, want 1.0 — only openrouter should contribute", got)
	}
}

// TestInfrastructureFailureDoesNotEscalate asserts that the
// infrastructure-failure check used by the tier-escalation loop in
// runAgentExecuteLoop short-circuits without consuming escalation
// budget. We verify the contract that:
//   - IsInfrastructureFailure(execution_failed, "502 Bad Gateway") is true,
//     so the loop body's infrastructure branch fires (deferred + RetryAfter).
//   - IsInfrastructureFailure(execution_failed, "TestFoo failed") is false,
//     so the loop falls through to mark the harness unhealthy and try the
//     next tier.
//
// This is a contract test — the actual tier-escalation closure is built
// inline in runAgentExecuteLoop. The two-arm decision in that closure
// (defer-vs-escalate) is driven entirely by IsInfrastructureFailure.
func TestInfrastructureFailureDoesNotEscalate(t *testing.T) {
	cases := []struct {
		name   string
		status string
		detail string
		// wantInfra=true means "defer with retry-after, do not escalate";
		// wantInfra=false means "escalate to next tier".
		wantInfra bool
	}{
		{"502 from provider triggers defer", agent.ExecuteBeadStatusExecutionFailed, "POST /v1/chat: 502 Bad Gateway", true},
		{"connection refused triggers defer", agent.ExecuteBeadStatusExecutionFailed, "dial tcp: connection refused", true},
		{"401 unauthorized triggers defer", agent.ExecuteBeadStatusExecutionFailed, "401 Unauthorized", true},
		{"plain test failure does NOT defer (escalates)", agent.ExecuteBeadStatusExecutionFailed, "TestFoo: assertion failed", false},
		{"structural failure escalates but is not infrastructure", "structural_validation_failed", "anything 502", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escalation.IsInfrastructureFailure(tc.status, tc.detail)
			if got != tc.wantInfra {
				t.Fatalf("IsInfrastructureFailure(%q, %q) = %v, want %v", tc.status, tc.detail, got, tc.wantInfra)
			}
			// Sanity: an infra failure on an escalatable status should
			// also have ShouldEscalate=true (so the loop body must
			// explicitly check infra BEFORE escalating).
			if tc.wantInfra && !escalation.ShouldEscalate(tc.status) {
				t.Fatalf("infrastructure-failure case %q has !ShouldEscalate; loop ordering invariant broken", tc.name)
			}
		})
	}
}
