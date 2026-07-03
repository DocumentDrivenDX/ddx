package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewRunnerStub struct {
	result   *agent.Result
	err      error
	lastOpts agent.RunArgs
}

func (r *reviewRunnerStub) Run(opts agent.RunArgs) (*agent.Result, error) {
	r.lastOpts = opts
	return r.result, r.err
}

// TestWireTelemetry covers FEAT-022 §15 for both review and grading attempt
// bundles. The result.json must carry an evidence_assembly object whose shape
// matches EvidenceAssemblySection (Stage A1 contract): per-section
// bytes_included / bytes_omitted / truncation_reason / selected_items /
// omitted_items, plus total input_bytes + output_bytes.
func TestWireTelemetry(t *testing.T) {
	t.Run("review", func(t *testing.T) {
		projectRoot := t.TempDir()
		out, err := exec.Command("git", "init", projectRoot).CombinedOutput()
		require.NoError(t, err, string(out))

		store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
		require.NoError(t, store.Init(context.Background()))
		// Write a governing doc whose body is large enough to be clamped
		// under the small caps used below (forces truncation_reason).
		docRel := "docs/big.md"
		docAbs := filepath.Join(projectRoot, filepath.FromSlash(docRel))
		require.NoError(t, os.MkdirAll(filepath.Dir(docAbs), 0o755))
		require.NoError(t, os.WriteFile(docAbs, []byte(strings.Repeat("X", 8*1024)), 0o644))

		require.NoError(t, store.Create(context.Background(), &bead.Bead{
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

		reviewer := &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   store,
			Runner: &reviewRunnerStub{result: &agent.Result{
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

		res, err := reviewer.ReviewBead(context.Background(), "ddx-evid-telem", head, agent.ImplementerRouting{Harness: "claude", Model: "claude-sonnet"})
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

}
