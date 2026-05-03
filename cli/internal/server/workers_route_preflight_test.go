package server

import (
	"testing"
)

// TestWorker_NoPreflight_WhenNoOperatorPin is the regression guard for
// ddx-9e4c238d: when operator pins NOTHING (no Profile, no Model) the
// worker must NOT install a strict ResolveRoute preflight. Previously the
// preflight ran with Profile="" + Harness="codex" + Model="", which made
// fizeau's ResolveRoute pick the harness's default catalog model and reject
// it as exact_pin_only — causing workers to spin emitting "no viable
// routing candidate for pins harness=codex: 1 candidates rejected".
//
// The auto-route case must pass through to fizeau's lenient Execute path.
func TestWorker_NoPreflight_WhenNoOperatorPin(t *testing.T) {
	cases := []struct {
		name string
		spec ExecuteLoopWorkerSpec
	}{
		{
			name: "bare drain — no harness, no model, no profile",
			spec: ExecuteLoopWorkerSpec{},
		},
		{
			name: "harness pin only — operator pinned harness=codex, nothing else",
			spec: ExecuteLoopWorkerSpec{Harness: "codex"},
		},
		{
			name: "provider pin only — provider without profile or model",
			spec: ExecuteLoopWorkerSpec{Provider: "openai"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pre := buildRoutePreflight(t.TempDir(), tc.spec, false /* hasBeadWorkerFactory */)
			if pre != nil {
				t.Fatalf("expected nil preflight for auto-route spec, got non-nil")
			}
		})
	}
}

// TestWorker_PreflightStillRuns_WhenOperatorPinsModel guards against
// over-reach in the ddx-9e4c238d fix: when the operator explicitly pins a
// Model or a Profile, the preflight should still be installed so an
// incompatible (harness,model) combo is rejected up-front (FEAT-006 D3).
func TestWorker_PreflightStillRuns_WhenOperatorPinsModel(t *testing.T) {
	cases := []struct {
		name string
		spec ExecuteLoopWorkerSpec
	}{
		{
			name: "model pin",
			spec: ExecuteLoopWorkerSpec{Model: "gpt-5.4-mini"},
		},
		{
			name: "profile pin",
			spec: ExecuteLoopWorkerSpec{Profile: "fast"},
		},
		{
			name: "harness + model pin",
			spec: ExecuteLoopWorkerSpec{Harness: "codex", Model: "gpt-5.4-mini"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pre := buildRoutePreflight(t.TempDir(), tc.spec, false /* hasBeadWorkerFactory */)
			if pre == nil {
				t.Fatalf("expected non-nil preflight when operator pinned Model/Profile, got nil")
			}
		})
	}
}

// TestWorker_NoPreflight_WhenOpaquePassthrough preserves existing behavior
// for the `ddx work` opaque-passthrough path.
func TestWorker_NoPreflight_WhenOpaquePassthrough(t *testing.T) {
	spec := ExecuteLoopWorkerSpec{
		Harness:           "codex",
		Model:             "gpt-5.4-mini",
		OpaquePassthrough: true,
	}
	if pre := buildRoutePreflight(t.TempDir(), spec, false); pre != nil {
		t.Fatalf("expected nil preflight under OpaquePassthrough, got non-nil")
	}
}

// TestWorker_NoPreflight_WhenBeadWorkerFactoryInjected preserves existing
// behavior for the test-injection path.
func TestWorker_NoPreflight_WhenBeadWorkerFactoryInjected(t *testing.T) {
	spec := ExecuteLoopWorkerSpec{Harness: "codex", Model: "gpt-5.4-mini"}
	if pre := buildRoutePreflight(t.TempDir(), spec, true /* hasBeadWorkerFactory */); pre != nil {
		t.Fatalf("expected nil preflight when BeadWorkerFactory is injected, got non-nil")
	}
}
