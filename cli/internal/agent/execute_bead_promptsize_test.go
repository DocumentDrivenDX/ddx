package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// representativePromptBead returns the deterministic bead used by the
// prompt-size fixture and the prompt_sha test. Holding it constant means
// rendered prompt sha and word/byte counts only change when the static
// instructions change — which is exactly what we want to surface.
func representativePromptBead() *bead.Bead {
	return &bead.Bead{
		ID:    "ddx-promptsize-fixture",
		Title: "Representative bead for prompt-size measurement",
		Description: "Static fixture used by Story 12 prompt-size report and " +
			"prompt_sha analytics. Description body is deliberately ordinary " +
			"prose so word/byte counts reflect the static instructions, not " +
			"the bead body.",
		Acceptance: "AC1: prompt renders deterministically.\n" +
			"AC2: word and byte counts are reported per (variant, contextBudget).",
		Labels: []string{"phase:2", "story:12", "tier:cheap"},
		Extra:  map[string]any{"spec-id": "FEAT-022"},
	}
}

// renderPromptForSize renders a prompt for the given harness selector and
// contextBudget using a fixed in-memory artifact bundle. It does not touch
// the filesystem so the fixture is hermetic.
func renderPromptForSize(t *testing.T, harness, contextBudget string) []byte {
	t.Helper()
	const attemptID = "20260101T000000-promptsize"
	arts := &executeBeadArtifacts{
		DirRel:      ".ddx/executions/" + attemptID,
		PromptRel:   ".ddx/executions/" + attemptID + "/prompt.md",
		ManifestRel: ".ddx/executions/" + attemptID + "/manifest.json",
		ResultRel:   ".ddx/executions/" + attemptID + "/result.json",
		ChecksRel:   ".ddx/executions/" + attemptID + "/checks.json",
		UsageRel:    ".ddx/executions/" + attemptID + "/usage.json",
	}
	b := representativePromptBead()
	// Empty refs to keep the fixture hermetic — buildPrompt will emit the
	// missing-governing fallback note for the non-minimal budget, which
	// matches the AC for that case.
	content, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeefdeadbeef", "", harness, contextBudget)
	if err != nil {
		t.Fatalf("buildPrompt(%s, %q): %v", harness, contextBudget, err)
	}
	return content
}

// TestPromptSizeReport renders the execute-bead prompt for both variants
// (Claude harness, Agent harness) at each contextBudget label ("" full,
// "minimal") and writes a deterministic word/byte report. CI uploads the
// report path named by DDX_PROMPT_SIZE_REPORT as an artifact so accidental
// prompt bloat shows up as a diff in the artifact across runs.
func TestPromptSizeReport(t *testing.T) {
	type row struct {
		Variant       string `json:"variant"`
		Harness       string `json:"harness"`
		ContextBudget string `json:"context_budget"`
		Words         int    `json:"words"`
		Bytes         int    `json:"bytes"`
		PromptSHA     string `json:"prompt_sha"`
	}

	// Selector at execute_bead.go routes (agent|fiz|embedded) to the Agent
	// variant; everything else (claude/codex/opencode/unknown) to the Claude
	// variant. One representative harness per variant is enough.
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	budgets := []string{"", "minimal"}

	rows := make([]row, 0, len(cases)*len(budgets))
	for _, c := range cases {
		for _, budget := range budgets {
			content := renderPromptForSize(t, c.harness, budget)
			sum := sha256.Sum256(content)
			rows = append(rows, row{
				Variant:       c.variant,
				Harness:       c.harness,
				ContextBudget: budget,
				Words:         len(strings.Fields(string(content))),
				Bytes:         len(content),
				PromptSHA:     hex.EncodeToString(sum[:]),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Variant != rows[j].Variant {
			return rows[i].Variant < rows[j].Variant
		}
		return rows[i].ContextBudget < rows[j].ContextBudget
	})

	var report strings.Builder
	report.WriteString("# execute-bead prompt size report\n")
	report.WriteString("# variant\tharness\tcontext_budget\twords\tbytes\tprompt_sha\n")
	for _, r := range rows {
		budget := r.ContextBudget
		if budget == "" {
			budget = "full"
		}
		fmt.Fprintf(&report, "%s\t%s\t%s\t%d\t%d\t%s\n",
			r.Variant, r.Harness, budget, r.Words, r.Bytes, r.PromptSHA)
	}

	// Always log the report to test output — that alone makes CI surface
	// it via the existing test-log capture even if the artifact step is
	// not configured.
	t.Logf("\n%s", report.String())

	// Sanity: every row produced non-empty output.
	for _, r := range rows {
		if r.Words == 0 || r.Bytes == 0 {
			t.Errorf("empty rendered prompt for variant=%s budget=%q", r.Variant, r.ContextBudget)
		}
	}
	// Minimal must be strictly smaller than full for both variants — the
	// fixture's reason for existing is to catch regressions on the cheap-tier
	// path.
	byKey := map[string]row{}
	for _, r := range rows {
		byKey[r.Variant+"|"+r.ContextBudget] = r
	}
	for _, v := range []string{"claude", "agent"} {
		full := byKey[v+"|"]
		min := byKey[v+"|minimal"]
		if min.Bytes >= full.Bytes {
			t.Errorf("variant %s: minimal bytes %d not smaller than full bytes %d",
				v, min.Bytes, full.Bytes)
		}
	}

	// Write the human-readable report alongside a JSON sibling for
	// machine consumers. CI sets DDX_PROMPT_SIZE_REPORT to a path under the
	// workspace and uploads it as an artifact; locally the test falls back
	// to t.TempDir() so it never pollutes the working tree.
	outPath := strings.TrimSpace(os.Getenv("DDX_PROMPT_SIZE_REPORT"))
	if outPath == "" {
		outPath = filepath.Join(t.TempDir(), "prompt-size-report.txt")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}
	if err := os.WriteFile(outPath, []byte(report.String()), 0o644); err != nil {
		t.Fatalf("writing prompt-size report: %v", err)
	}
	jsonPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".json"
	jb, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatalf("marshal rows: %v", err)
	}
	if err := os.WriteFile(jsonPath, append(jb, '\n'), 0o644); err != nil {
		t.Fatalf("writing prompt-size report json: %v", err)
	}
	t.Logf("prompt-size report written to %s (and %s)", outPath, jsonPath)
}

// TestPromptSHA_DeterministicAcrossRenders asserts that the helper
// promptSHA used to populate manifest.prompt_sha is stable across renders
// of the same inputs. Without this, analytics grouping by prompt_sha would
// fragment across attempts that share a prompt.
func TestPromptSHA_DeterministicAcrossRenders(t *testing.T) {
	a := renderPromptForSize(t, "claude", "")
	b := renderPromptForSize(t, "claude", "")
	if promptSHA(a) != promptSHA(b) {
		t.Fatalf("promptSHA not deterministic: %s vs %s", promptSHA(a), promptSHA(b))
	}
	c := renderPromptForSize(t, "agent", "")
	if promptSHA(a) == promptSHA(c) {
		t.Fatalf("promptSHA collided across variants — selector is not influencing the prompt body")
	}
}
