package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseReviewVerdict covers the strict JSON contract: every accepted
// shape decodes to a ReviewVerdict, every rejected shape returns
// ErrReviewVerdictUnparseable. This replaces the regex-based markdown
// extractor that silently mis-parsed `### Verdict: APPROVE` outputs whenever
// the model echoed back the prompt's options-header line.
func TestParseReviewVerdict(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		wantVerdict Verdict
		wantSummary string
	}{
		{
			name:        "raw JSON object — APPROVE",
			input:       `{"schema_version":1,"verdict":"APPROVE","summary":"all good"}`,
			wantVerdict: VerdictApprove,
			wantSummary: "all good",
		},
		{
			name:        "fenced JSON block — REQUEST_CHANGES",
			input:       "```json\n{\"schema_version\":1,\"verdict\":\"REQUEST_CHANGES\",\"summary\":\"fix tests\"}\n```",
			wantVerdict: VerdictRequestChanges,
			wantSummary: "fix tests",
		},
		{
			name:        "fenced JSON block — BLOCK",
			input:       "Some preamble.\n\n```json\n{\"verdict\":\"BLOCK\",\"summary\":\"AC#3 missing\"}\n```\n\ntrailing prose",
			wantVerdict: VerdictBlock,
			wantSummary: "AC#3 missing",
		},
		{
			name: "mixed prose + multiple fences — last json fence wins",
			input: "```\nignore this generic block\n```\n\n```json\n" +
				`{"verdict":"APPROVE","summary":"only the last json fence counts"}` +
				"\n```\n",
			wantVerdict: VerdictApprove,
			wantSummary: "only the last json fence counts",
		},
		{
			name:        "tolerates unknown extra fields",
			input:       `{"verdict":"APPROVE","summary":"ok","custom_field":42}`,
			wantVerdict: VerdictApprove,
			wantSummary: "ok",
		},
		{
			name:    "rejects unknown verdict value",
			input:   `{"verdict":"OK"}`,
			wantErr: true,
		},
		{
			name:    "rejects missing verdict field",
			input:   `{"summary":"no verdict here"}`,
			wantErr: true,
		},
		{
			name:    "rejects truncated JSON",
			input:   `{"verdict":"APPROVE","summary":"unclosed`,
			wantErr: true,
		},
		{
			name:    "rejects empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "rejects pure markdown with no JSON",
			input:   "### Verdict: APPROVE\n\nNo JSON anywhere here.",
			wantErr: true,
		},
		{
			name:    "rejects lowercase verdict value (case-sensitive enum)",
			input:   `{"verdict":"approve"}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReviewVerdict([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrReviewVerdictUnparseable),
					"unparseable input must surface a typed error so the loop classifies as review-error (retryable), not as a silent BLOCK")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantVerdict, got.Verdict)
			if tc.wantSummary != "" {
				assert.Equal(t, tc.wantSummary, got.Summary)
			}
		})
	}
}

// TestReviewerJSONHappyPath verifies the call site at DefaultBeadReviewer:
// when the reviewer emits a valid JSON contract, ReviewBead returns a
// ReviewResult with the parsed verdict.
func TestReviewerJSONHappyPath(t *testing.T) {
	projectRoot, head, store := newReviewArtifactsFixture(t)

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner: &reviewRunnerStub{result: &Result{
			Harness:        "claude",
			Model:          "claude-opus-4-6",
			Output:         "```json\n{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"all good\"}\n```",
			DurationMS:     10,
			AgentSessionID: "native-review-happy",
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-happy", head, "claude", "claude-sonnet")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, VerdictApprove, res.Verdict)
	assert.Equal(t, "all good", res.Rationale)
	assert.Empty(t, res.Error, "happy path must not record a review-error class")
}

// TestReviewerEmitsJSONContract is the regression for the upstream report:
// when the reviewer returns markdown wrapped output (e.g. `### Verdict:
// APPROVE`), the call site must REJECT it as unparseable rather than
// silently treating the verdict as BLOCK (or any other value extracted from
// the prompt's options-header line). This confirms the markdown extractor
// path is gone.
func TestReviewerEmitsJSONContract(t *testing.T) {
	projectRoot, head, store := newReviewArtifactsFixture(t)

	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner: &reviewRunnerStub{result: &Result{
			Harness: "claude",
			Model:   "claude-opus-4-6",
			Output: "## Review: ddx-review-md iter 1\n\n" +
				"### Verdict: APPROVE\n\n" +
				"### Findings\n- everything looks great",
			DurationMS:     10,
			AgentSessionID: "native-review-md",
		}},
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-happy", head, "claude", "claude-sonnet")
	require.Error(t, err, "markdown wrapped output must be rejected — the markdown extractor is gone")
	require.NotNil(t, res)
	assert.Equal(t, evidence.OutcomeReviewUnparseable, res.Error,
		"call site must surface the unparseable class so the loop classifies it as review-error (retryable)")
}

// TestReviewerNoOptionsHeaderRegression is the precise regression for the
// upstream report's silent failure mode: when the reviewer echoes back the
// prompt template's options header (e.g. literal text like `APPROVE |
// REQUEST_CHANGES | BLOCK` or `### Verdict: APPROVE | REQUEST_CHANGES |
// BLOCK`) and emits NO valid JSON object, the parser must reject it. The
// pre-fix markdown extractor pulled "BLOCK" from this header line and
// silently logged BLOCK for actual APPROVEs.
func TestReviewerNoOptionsHeaderRegression(t *testing.T) {
	transcripts := []string{
		// Bare options-header echo.
		"### Verdict: APPROVE | REQUEST_CHANGES | BLOCK",
		// The template's literal options listing without JSON.
		"verdict must be one of: APPROVE, REQUEST_CHANGES, BLOCK",
		// Markdown with a parenthetical listing options.
		"## Review\n### Verdict: (APPROVE / REQUEST_CHANGES / BLOCK)",
	}
	for _, tx := range transcripts {
		t.Run(tx[:min(40, len(tx))], func(t *testing.T) {
			got, err := ParseReviewVerdict([]byte(tx))
			require.Error(t, err, "options-header echo must be rejected as unparseable, NOT silently parsed as BLOCK")
			assert.True(t, errors.Is(err, ErrReviewVerdictUnparseable))
			assert.Empty(t, string(got.Verdict))
		})
	}
}

// TestReviewPromptRequestsJSON asserts the reviewer prompt template
// instructs the model to emit a JSON object with the schema version, the
// verdict enum, and a summary field — and includes a fenced ```json …```
// example. The regex-based markdown contract this replaces did not specify
// JSON output and so silently allowed echo-style mis-parses.
func TestReviewPromptRequestsJSON(t *testing.T) {
	mustContain := []string{
		"schema_version",
		"verdict",
		"summary",
		"findings",
		"```json",
		"APPROVE",
		"REQUEST_CHANGES",
		"BLOCK",
	}
	for _, want := range mustContain {
		assert.Contains(t, beadReviewInstructions, want,
			"reviewer prompt must request JSON contract — missing required marker %q", want)
	}
	// And it must not instruct the legacy markdown table contract: the
	// `### AC Grades` heading was the seam that the old markdown extractor
	// keyed on. Its presence in the new template would invite the model to
	// fall back to that shape.
	assert.NotContains(t, beadReviewInstructions, "### AC Grades",
		"prompt must not still request the legacy markdown AC-grades table")
}

// newReviewArtifactsFixture sets up a bare git repository with a single
// committed file and a bead, suitable for calls into
// DefaultBeadReviewer.ReviewBead.
func newReviewArtifactsFixture(t *testing.T) (projectRoot, head string, store *bead.Store) {
	t.Helper()
	projectRoot = t.TempDir()
	cmd := exec.Command("git", "init", projectRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	store = bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# review test\n"), 0o644))
	require.NoError(t, store.Create(&bead.Bead{
		ID:          "ddx-review-happy",
		Title:       "Review JSON contract test",
		Description: "Ensure the JSON contract is enforced.",
		Acceptance:  "1. AC one\n2. AC two\n3. AC three",
	}))
	out, err = exec.Command("git", "-C", projectRoot, "add", "README.md", ".ddx/beads.jsonl").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))
	headRaw, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head = strings.TrimSpace(string(headRaw))
	return projectRoot, head, store
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
