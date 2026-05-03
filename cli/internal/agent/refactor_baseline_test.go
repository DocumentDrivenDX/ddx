package agent

// TestRefactorBaseline_FixturesStable freezes the canonical event-shape for the
// canonical bead lifecycles enumerated in
// /tmp/execute-bead-refactor-proposal.md §6.1 R4. Subsequent execute-bead
// refactor children diff their runtime output against these fixtures to catch
// unintended changes to the {kind, summary, body, source, data} shape of
// bead.result, execute-bead, review, routing, triage-decision, loop-error,
// decomposition-recommendation, land-conflict-*, push-conflict and friends.
//
// Each scenario is defined in code as an ordered sequence of events. The test
// re-encodes each event with the standard library json.Marshal (which sorts
// map keys) and compares the resulting JSONL byte-for-byte against the golden
// fixture under testdata/refactor_baseline/. To regenerate the fixtures after
// an intentional event-shape change, run
//
//	UPDATE_BASELINE=1 go test -run TestRefactorBaseline ./internal/agent/
//
// and commit the updated .jsonl files alongside the code change.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type baselineEvent map[string]any

var refactorBaselineScenarios = map[string][]baselineEvent{
	"merged_success_close.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "merged_success",
			"body":    "result_rev=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nbase_rev=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-merged",
				"status":      "merged_success",
				"detail":      "merged",
				"session_id":  "session-baseline-merged",
				"result_rev":  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"base_rev":    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"duration_ms": 12345,
			},
			"stream": "loop",
		},
	},
	"success_review_approve_close.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "merged_success",
			"body":    "result_rev=cccccccccccccccccccccccccccccccccccccccc\nbase_rev=dddddddddddddddddddddddddddddddddddddddd",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "review",
			"summary": "review: APPROVE",
			"body":    "post-merge review: APPROVE",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-approve",
				"status":      "merged_success",
				"detail":      "review: APPROVE",
				"session_id":  "session-baseline-approve",
				"result_rev":  "cccccccccccccccccccccccccccccccccccccccc",
				"base_rev":    "dddddddddddddddddddddddddddddddddddddddd",
				"duration_ms": 23456,
			},
			"stream": "loop",
		},
	},
	"success_review_block_reopen_triage.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "merged_success",
			"body":    "result_rev=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\nbase_rev=ffffffffffffffffffffffffffffffffffffffff",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "review",
			"summary": "review: BLOCK",
			"body":    "post-merge review: BLOCK (flagged for human)",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "reopen",
			"summary": "review: BLOCK",
		},
		{
			"kind":    "triage-decision",
			"summary": "verdict=needs_human",
			"body":    `{"verdict":"needs_human","reason":"review BLOCK escalated"}`,
			"source":  "ddx triage dispatch",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-block",
				"status":      "review_block",
				"detail":      "review: BLOCK",
				"session_id":  "session-baseline-block",
				"result_rev":  "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
				"base_rev":    "ffffffffffffffffffffffffffffffffffffffff",
				"duration_ms": 34567,
			},
			"stream": "loop",
		},
	},
	"land_conflict_recover.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "land-conflict-auto-recovered",
			"summary": "land conflict auto-recovered",
			"body":    `{"strategy":"rebase","attempts":1,"resolved":true}`,
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "merged_success",
			"body":    "result_rev=1111111111111111111111111111111111111111\nbase_rev=2222222222222222222222222222222222222222",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-landrecover",
				"status":      "merged_success",
				"detail":      "merged after land-conflict auto-recover",
				"session_id":  "session-baseline-landrecover",
				"result_rev":  "1111111111111111111111111111111111111111",
				"base_rev":    "2222222222222222222222222222222222222222",
				"duration_ms": 45678,
			},
			"stream": "loop",
		},
	},
	"no_changes_verified.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "no_changes_verified",
			"body":    "no_changes_rationale: already_satisfied; verification_command exited 0",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":              "ddx-baseline-already",
				"status":               "no_changes_verified",
				"detail":               "already_satisfied",
				"session_id":           "session-baseline-already",
				"no_changes_rationale": "already_satisfied",
				"duration_ms":          5678,
			},
			"stream": "loop",
		},
	},
	"no_changes_unjustified.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "no_changes_unjustified",
			"body":    "no rationale or verification produced",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "reopen",
			"summary": "no_changes_unjustified (cooldown)",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-unjust",
				"status":      "no_changes_unjustified",
				"detail":      "reopened with cooldown",
				"session_id":  "session-baseline-unjust",
				"duration_ms": 6789,
			},
			"stream": "loop",
		},
	},
	"decomposition.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "decomposition-recommendation",
			"summary": "decompose into 3 children",
			"body":    `{"children":["ddx-c1","ddx-c2","ddx-c3"]}`,
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "declined_needs_decomposition",
			"body":    "agent declined: bead too large; decomposed into ddx-c1, ddx-c2, ddx-c3",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-decompose",
				"status":      "declined_needs_decomposition",
				"detail":      "decomposed",
				"session_id":  "session-baseline-decompose",
				"duration_ms": 7890,
			},
			"stream": "loop",
		},
	},
	"push_failed.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "push_failed",
			"body":    "git push failed: remote rejected (auth)",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":      "ddx-baseline-pushfail",
				"status":       "push_failed",
				"detail":       "push failed",
				"session_id":   "session-baseline-pushfail",
				"result_rev":   "3333333333333333333333333333333333333333",
				"base_rev":     "4444444444444444444444444444444444444444",
				"preserve_ref": "refs/ddx/iterations/ddx-baseline-pushfail/attempt-1",
				"duration_ms":  8901,
			},
			"stream": "loop",
		},
	},
	"push_conflict.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "push-conflict",
			"summary": "push rejected: non-fast-forward",
			"body":    `{"strategy":"abort","preserve_ref":"refs/ddx/iterations/ddx-baseline-pushconflict/attempt-1"}`,
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "push_conflict",
			"body":    "push conflict: non-fast-forward; result preserved",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":      "ddx-baseline-pushconflict",
				"status":       "push_conflict",
				"detail":       "push conflict",
				"session_id":   "session-baseline-pushconflict",
				"result_rev":   "5555555555555555555555555555555555555555",
				"base_rev":     "6666666666666666666666666666666666666666",
				"preserve_ref": "refs/ddx/iterations/ddx-baseline-pushconflict/attempt-1",
				"duration_ms":  9012,
			},
			"stream": "loop",
		},
	},
	"preserved_needs_review.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "preserved_needs_review",
			"body":    "result preserved on iteration ref pending human review",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":      "ddx-baseline-preserved",
				"status":       "preserved_needs_review",
				"detail":       "preserved",
				"session_id":   "session-baseline-preserved",
				"result_rev":   "7777777777777777777777777777777777777777",
				"base_rev":     "8888888888888888888888888888888888888888",
				"preserve_ref": "refs/ddx/iterations/ddx-baseline-preserved/attempt-1",
				"duration_ms":  10123,
			},
			"stream": "loop",
		},
	},
	"default_failure.jsonl": {
		{
			"kind":    "routing",
			"summary": "provider=claude model=claude-sonnet-4-6",
			"body":    `{"resolved_provider":"claude","resolved_model":"claude-sonnet-4-6","fallback_chain":[]}`,
			"source":  "ddx agent execute-bead",
			"actor":   "ddx",
		},
		{
			"kind":    "execute-bead",
			"summary": "execution_failed",
			"body":    "agent: provider error: 500 internal",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind":    "loop-error",
			"summary": "execute returned error",
			"body":    "provider error: 500 internal",
			"source":  "ddx agent execute-loop",
			"actor":   "ddx",
		},
		{
			"kind": "bead.result",
			"data": map[string]any{
				"bead_id":     "ddx-baseline-fail",
				"status":      "execution_failed",
				"detail":      "provider error",
				"session_id":  "session-baseline-fail",
				"duration_ms": 11234,
			},
			"stream": "loop",
		},
	},
}

// expectedKindsInOrder asserts each scenario emits the headline event kinds
// referenced by /tmp/execute-bead-refactor-proposal.md §6.1 R4 in canonical
// order. This is the structural contract refactor children must preserve.
var refactorBaselineExpectedKinds = map[string][]string{
	"merged_success_close.jsonl":               {"routing", "execute-bead", "bead.result"},
	"success_review_approve_close.jsonl":       {"routing", "execute-bead", "review", "bead.result"},
	"success_review_block_reopen_triage.jsonl": {"routing", "execute-bead", "review", "reopen", "triage-decision", "bead.result"},
	"land_conflict_recover.jsonl":              {"routing", "land-conflict-auto-recovered", "execute-bead", "bead.result"},
	"no_changes_verified.jsonl":                {"routing", "execute-bead", "bead.result"},
	"no_changes_unjustified.jsonl":             {"routing", "execute-bead", "reopen", "bead.result"},
	"decomposition.jsonl":                      {"routing", "decomposition-recommendation", "execute-bead", "bead.result"},
	"push_failed.jsonl":                        {"routing", "execute-bead", "bead.result"},
	"push_conflict.jsonl":                      {"routing", "push-conflict", "execute-bead", "bead.result"},
	"preserved_needs_review.jsonl":             {"routing", "execute-bead", "bead.result"},
	"default_failure.jsonl":                    {"routing", "execute-bead", "loop-error", "bead.result"},
}

func TestRefactorBaseline_FixturesStable(t *testing.T) {
	dir := filepath.Join("testdata", "refactor_baseline")
	update := os.Getenv("UPDATE_BASELINE") == "1"

	names := make([]string, 0, len(refactorBaselineScenarios))
	for name := range refactorBaselineScenarios {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		events := refactorBaselineScenarios[name]
		t.Run(name, func(t *testing.T) {
			var canonical bytes.Buffer
			gotKinds := make([]string, 0, len(events))
			for _, ev := range events {
				if k, _ := ev["kind"].(string); k != "" {
					gotKinds = append(gotKinds, k)
				}
				line, err := json.Marshal(ev)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				canonical.Write(line)
				canonical.WriteByte('\n')
			}

			want, ok := refactorBaselineExpectedKinds[name]
			if !ok {
				t.Fatalf("scenario %q missing expected-kinds entry", name)
			}
			if !equalStringSlices(gotKinds, want) {
				t.Fatalf("scenario %q: kinds=%v want=%v", name, gotKinds, want)
			}

			path := filepath.Join(dir, name)
			if update {
				if err := os.WriteFile(path, canonical.Bytes(), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				return
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture %s: %v (run UPDATE_BASELINE=1 to regenerate)", path, err)
			}
			if !bytes.Equal(got, canonical.Bytes()) {
				t.Fatalf("fixture %s drifted from canonical baseline; run UPDATE_BASELINE=1 to regenerate after intentional event-shape changes", path)
			}
		})
	}

	t.Run("no_orphan_fixtures", func(t *testing.T) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
				continue
			}
			if _, ok := refactorBaselineScenarios[e.Name()]; !ok {
				t.Errorf("orphan baseline fixture %s has no scenario in refactorBaselineScenarios", e.Name())
			}
		}
	})
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
