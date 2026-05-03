package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvidenceAssemblyTelemetry covers FEAT-022 §15 for both review and
// grading attempt bundles. The result.json must carry an evidence_assembly
// object whose shape matches EvidenceAssemblySection (Stage A1 contract):
// per-section bytes_included / bytes_omitted / truncation_reason /
// selected_items / omitted_items, plus total input_bytes + output_bytes.
func TestEvidenceAssemblyTelemetry(t *testing.T) {
	t.Run("review", func(t *testing.T) {
		projectRoot := t.TempDir()
		out, err := exec.Command("git", "init", projectRoot).CombinedOutput()
		require.NoError(t, err, string(out))

		store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
		require.NoError(t, store.Init())
		// Write a governing doc whose body is large enough to be clamped
		// under the small caps used below (forces truncation_reason).
		docRel := "docs/big.md"
		docAbs := filepath.Join(projectRoot, filepath.FromSlash(docRel))
		require.NoError(t, os.MkdirAll(filepath.Dir(docAbs), 0o755))
		require.NoError(t, os.WriteFile(docAbs, []byte(strings.Repeat("X", 8*1024)), 0o644))

		require.NoError(t, store.Create(&bead.Bead{
			ID:          "ddx-evid-telem",
			Title:       "evidence telemetry review fixture",
			Description: "exercise governing doc clamp + diff section",
			Acceptance:  "AC#1: foo handler\nAC#2: bar coverage",
			Notes:       "see docs/big.md",
			Extra:       map[string]any{"spec-id": docRel},
		}))
		out, err = exec.Command("git", "-C", projectRoot, "add", "README.md", "docs/big.md", ".ddx/beads.jsonl").CombinedOutput()
		// README.md may not exist; ignore and just commit what's staged.
		_ = err
		_ = out
		out, err = exec.Command("git", "-C", projectRoot, "add", "-A").CombinedOutput()
		require.NoError(t, err, string(out))
		out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init").CombinedOutput()
		require.NoError(t, err, string(out))
		headRaw, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
		require.NoError(t, err)
		head := strings.TrimSpace(string(headRaw))

		reviewer := &DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   store,
			Runner: &reviewRunnerStub{result: &Result{
				Harness:        "claude",
				Model:          "claude-opus-4-6",
				Output:         "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}\n```",
				DurationMS:     17,
				AgentSessionID: "sess-evid-1",
			}},
			Caps: evidence.Caps{
				MaxPromptBytes:       64 * 1024,
				MaxInlinedFileBytes:  4 * 1024,
				MaxDiffBytes:         16 * 1024,
				MaxGoverningDocBytes: 1024, // forces clamp on the 8KiB doc
			},
		}

		res, err := reviewer.ReviewBead(context.Background(), "ddx-evid-telem", head, ImplementerRouting{Harness: "claude", Model: "claude-sonnet"})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotEmpty(t, res.ExecutionDir)

		resultPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "result.json")
		raw, err := os.ReadFile(resultPath)
		require.NoError(t, err)

		var doc map[string]any
		require.NoError(t, json.Unmarshal(raw, &doc))
		ea, ok := doc["evidence_assembly"].(map[string]any)
		require.True(t, ok, "result.json must contain an evidence_assembly object: %s", string(raw))
		// total bytes
		assert.Greater(t, int(ea["input_bytes"].(float64)), 0, "input_bytes must be populated")
		assert.Greater(t, int(ea["output_bytes"].(float64)), 0, "output_bytes must be populated")

		// per-section records
		secsRaw, ok := ea["sections"].([]any)
		require.True(t, ok, "evidence_assembly.sections must be an array")
		require.NotEmpty(t, secsRaw, "expect at least one section record (bead, governing, diff)")

		// At least one section should carry a truncation_reason (the clamped
		// governing doc) and the EvidenceAssemblySection field shape.
		foundClampedGoverning := false
		for _, raw := range secsRaw {
			s, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			// Required field shape: bytes_included / bytes_omitted /
			// truncation_reason / selected_items / omitted_items / name.
			_, hasName := s["name"]
			_, hasIncluded := s["bytes_included"]
			assert.True(t, hasName, "section must carry name")
			assert.True(t, hasIncluded, "section must carry bytes_included")
			if reason, ok := s["truncation_reason"].(string); ok && reason == "governing_doc_cap" {
				foundClampedGoverning = true
			}
		}
		assert.True(t, foundClampedGoverning, "expected a section with truncation_reason=governing_doc_cap")

		// jq-style assertion: evidence_assembly is an object.
		assert.IsType(t, map[string]any{}, doc["evidence_assembly"])

		// Manifest mirrors the same key.
		manifestPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "manifest.json")
		mraw, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		var mdoc map[string]any
		require.NoError(t, json.Unmarshal(mraw, &mdoc))
		_, ok = mdoc["evidence_assembly"].(map[string]any)
		assert.True(t, ok, "manifest.json must also carry evidence_assembly")
	})

	t.Run("grade", func(t *testing.T) {
		mock := &mockExecutor{output: `{"arms":[{"arm":"agent","score":8,"max_score":10,"pass":true,"rationale":"ok"}]}`}
		r := newTestRunner(mock)

		// Force per-arm clamp by feeding a large arm output against a tight
		// per-arm cap so a section carries truncation_reason=per_arm_cap.
		bigOutput := strings.Repeat("A", 4*1024)
		record := &ComparisonRecord{
			ID:     "cmp-evid",
			Prompt: "do the thing",
			Arms: []ComparisonArm{
				{Harness: "agent", Model: "m1", Output: bigOutput},
			},
		}
		artifactDir := t.TempDir()
		_, err := GradeFn(r, record, GradeOptions{
			Grader: "codex",
			Caps: evidence.Caps{
				MaxPromptBytes:      4 * 1024 * 1024,
				MaxInlinedFileBytes: 256, // forces per-arm clamp
				MaxDiffBytes:        1024,
			},
			ArtifactDir: artifactDir,
		})
		require.NoError(t, err)

		raw, err := os.ReadFile(filepath.Join(artifactDir, "result.json"))
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(raw, &doc))
		ea, ok := doc["evidence_assembly"].(map[string]any)
		require.True(t, ok, "grade result.json must contain evidence_assembly: %s", string(raw))
		assert.Greater(t, int(ea["input_bytes"].(float64)), 0)
		assert.Greater(t, int(ea["output_bytes"].(float64)), 0)
		secs, ok := ea["sections"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, secs)

		foundClampedArm := false
		for _, rs := range secs {
			s := rs.(map[string]any)
			if reason, _ := s["truncation_reason"].(string); reason == "per_arm_cap" {
				foundClampedArm = true
			}
		}
		assert.True(t, foundClampedArm, "expected an arm section with truncation_reason=per_arm_cap")

		// Manifest mirrors.
		mraw, err := os.ReadFile(filepath.Join(artifactDir, "manifest.json"))
		require.NoError(t, err)
		var mdoc map[string]any
		require.NoError(t, json.Unmarshal(mraw, &mdoc))
		_, ok = mdoc["evidence_assembly"].(map[string]any)
		assert.True(t, ok, "grade manifest.json must also carry evidence_assembly")
	})
}

// TestReviewEventBodySummary covers FEAT-022 §16: review, review-error, and
// compare-result event bodies all carry the compact summary fields
// (input_bytes, output_bytes, elapsed_ms, harness, model).
func TestReviewEventBodySummary(t *testing.T) {
	summary := EventBodySummary{
		Harness:     "claude",
		Model:       "claude-opus-4-6",
		InputBytes:  12345,
		OutputBytes: 678,
		ElapsedMS:   42,
	}

	t.Run("review", func(t *testing.T) {
		body := AppendEventSummary(ReviewEventBody("APPROVE", "ok", "path/to/log"), summary)
		assertSummaryFields(t, body, summary)
	})

	t.Run("review-error", func(t *testing.T) {
		body := AppendEventSummary(
			ReviewErrorEventBody(evidence.OutcomeReviewTransport, 1, "abc123", "boom"),
			summary,
		)
		assertSummaryFields(t, body, summary)
		// Crucial: section detail does NOT land on the event body.
		assert.NotContains(t, body, "bytes_included",
			"event body must not carry per-section detail (lives only on the artifact)")
	})

	t.Run("compare-result", func(t *testing.T) {
		body := compareResultEventBody(summary)
		assertSummaryFields(t, body, summary)
	})
}

func assertSummaryFields(t *testing.T, body string, s EventBodySummary) {
	t.Helper()
	assert.Contains(t, body, "harness="+s.Harness)
	assert.Contains(t, body, "model="+s.Model)
	assert.Contains(t, body, "input_bytes=12345")
	assert.Contains(t, body, "output_bytes=678")
	assert.Contains(t, body, "elapsed_ms=42")
}

// TestReviewEventBodyCap covers FEAT-022 §16's cap clause: even with maximum-
// sized summary fields, the event body must still respect the bead store cap
// at cli/internal/bead/store.go:747 (capFieldBytes / MaxFieldBytes = 65535).
func TestReviewEventBodyCap(t *testing.T) {
	storeDir := t.TempDir()
	store := bead.NewStore(storeDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-cap-1", Title: "cap test"}))

	// Build maximum-sized summary fields. Harness/Model strings of 100KB each
	// alone overshoot the 65,535-byte cap; the cap path inside AppendEvent
	// must clamp via capFieldBytes regardless of caller intent.
	bigStr := strings.Repeat("X", 100*1024)
	summary := EventBodySummary{
		Harness:     bigStr,
		Model:       bigStr,
		InputBytes:  1<<31 - 1,
		OutputBytes: 1<<31 - 1,
		ElapsedMS:   1<<31 - 1,
	}
	bigRationale := strings.Repeat("R", 100*1024)
	body := AppendEventSummary(ReviewEventBody("BLOCK", bigRationale, "log"), summary)

	require.NoError(t, store.AppendEvent("ddx-cap-1", bead.BeadEvent{
		Kind:      "review",
		Summary:   "BLOCK",
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}))

	events, err := store.Events("ddx-cap-1")
	require.NoError(t, err)
	require.NotEmpty(t, events)
	stored := events[len(events)-1]
	assert.LessOrEqual(t, len(stored.Body), bead.MaxFieldBytes,
		"event body must respect MaxFieldBytes after AppendEvent's capFieldBytes")
}
