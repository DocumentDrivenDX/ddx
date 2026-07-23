package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func readSkillCopies(t *testing.T) map[string]string {
	t.Helper()

	root := testRepoRoot(t)
	paths := map[string]string{
		"library":  filepath.Join(root, "library", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		"cli":      filepath.Join(root, "cli", "internal", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		"embedded": filepath.Join(root, "cli", "internal", "registry", "defaultplugin", "library", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		"agents":   filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		"claude":   filepath.Join(root, ".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
	}
	out := make(map[string]string, len(paths))
	for name, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err, path)
		out[name] = string(data)
	}
	return out
}

func skillSection(t *testing.T, body, heading string) string {
	t.Helper()

	start := strings.Index(body, heading)
	require.NotEqual(t, -1, start, "missing section %q", heading)
	return body[start:]
}

func TestBeadLifecycleSkillReadinessDocumentsRewriteContract(t *testing.T) {
	skills := readSkillCopies(t)
	for name, body := range skills {
		t.Run(name, func(t *testing.T) {
			readiness := skillSection(t, body, "## READINESS MODE")
			assert.Contains(t, body, "Use the exact readiness classifications:")
			for _, want := range []string{"`ready`", "`needs_refine`", "`needs_split`", "`operator_required`", "`system_unready`"} {
				assert.Contains(t, body, want)
			}
			for _, want := range []string{"`ambiguous`", "`safely_refinable`", "`split`", "`rewritten`", "`needs_human`"} {
				assert.Contains(t, body, want)
			}
			assert.Contains(t, body, "`suggested_fixes` are")
			assert.Contains(t, body, "advisory diagnostics for the author or operator")
			assert.Contains(t, body, "`rewrite` is the machine-")
			assert.Contains(t, body, "consumable replacement contract DDx may apply before claim")
			assert.Contains(t, body, "`rewrite.changed_fields` is required")
			assert.Contains(t, body, "`rewrite.description` / `rewrite.acceptance` must be strings, not arrays")
			assert.Contains(t, body, "`readiness_checks` MUST be a JSON array")
			assert.Contains(t, body, "`waivers_applied` SHOULD be emitted as a JSON array")
			assert.Contains(t, body, "legacy flat string arrays or a scalar string")
			assert.Contains(t, body, "`score` is metadata only for the readiness rubric summary")
			assert.Contains(t, body, "power-selection, or ordering input")
			assert.Contains(t, body, "every entry MUST")
			assert.Contains(t, readiness, `"classification": "ready|needs_refine|needs_split|operator_required|system_unready"`)
			assert.Contains(t, readiness, `"tractability": "tractable|too_large|ambiguous|blocked|unknown"`)
			assert.Contains(t, readiness, `"score": 0`)
			assert.Contains(t, readiness, `"rationale": "brief evidence-grounded explanation"`)
			assert.Contains(t, readiness, `"changed_fields": ["description", "acceptance"]`)
			assert.Contains(t, readiness, `"acceptance": "1. TestFoo`)
			assert.Contains(t, body, "Stale-blocker precedence")
			assert.Contains(t, body, "supersedes an older blocker prior attempt")
		})
	}
}

func TestBeadLifecycleSkillLintDocumentsRationaleShape(t *testing.T) {
	skills := readSkillCopies(t)
	for name, body := range skills {
		t.Run(name, func(t *testing.T) {
			lint := skillSection(t, body, "## LINT MODE")
			assert.Contains(t, lint, "`LintResult.rationale` is a single string summary.")
			assert.Contains(t, lint, `"rationale": "brief evidence-grounded explanation"`)
			assert.Contains(t, lint, `"suggested_fixes": [`)
			assert.Contains(t, lint, `"specific amendment to make"`)
			assert.Contains(t, lint, `"waivers_applied": [`)
			assert.Contains(t, lint, `"doc-only"`)
			assert.NotContains(t, lint, `"rationale": [`)
			assert.NotContains(t, lint, `"criterion": "a|b|c|d|e|f|g|h"`)
		})
	}
}

func TestPreClaimReadiness_DecodesRewriteAcceptanceString(t *testing.T) {
	payload := `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}],"rewrite":{"changed_fields":["description","acceptance"],"description":"PROBLEM\nmissing verification\n\nROOT CAUSE\nno gate\n\nPROPOSED FIX\nadd tests\n\nNON-SCOPE\nrouting\n","acceptance":"1. TestPreClaimReadiness_DecodesRewriteAcceptanceString\n2. cd cli && go test ./internal/agent/... -run \"TestPreClaimReadiness_.*|TestLintHook_RejectsMalformedRationaleShape\" -count=1\n3. lefthook run pre-commit"}}`

	got, err := decodePreClaimIntakePayloadResultWithMode(payload, config.BeadQualityModeWarnOnly)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableButRewritten, got.Outcome)
	assert.Equal(t, []string{"description", "acceptance"}, got.Rewrite.ChangedFields)
	assert.Contains(t, got.Rewrite.Description, "PROBLEM")
	assert.Equal(t, strings.Join([]string{
		"1. TestPreClaimReadiness_DecodesRewriteAcceptanceString",
		`2. cd cli && go test ./internal/agent/... -run "TestPreClaimReadiness_.*|TestLintHook_RejectsMalformedRationaleShape" -count=1`,
		"3. lefthook run pre-commit",
	}, "\n"), got.Rewrite.Acceptance)
}

func TestPreClaimReadiness_DecodesRewriteAcceptanceList(t *testing.T) {
	payload := `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}],"rewrite":{"changed_fields":["description","acceptance"],"description":"PROBLEM\nmissing verification\n\nROOT CAUSE\nno gate\n\nPROPOSED FIX\nadd tests\n\nNON-SCOPE\nrouting\n","acceptance":[" 1. TestPreClaimReadiness_DecodesRewriteAcceptanceList ","","2. cd cli && go test ./internal/agent/... -run \"TestPreClaimReadiness_.*|TestLintHook_RejectsMalformedRationaleShape\" -count=1","   ","3. lefthook run pre-commit "]}}`

	got, err := decodePreClaimIntakePayloadResultWithMode(payload, config.BeadQualityModeWarnOnly)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableButRewritten, got.Outcome)
	assert.Equal(t, []string{"description", "acceptance"}, got.Rewrite.ChangedFields)
	assert.Equal(t, strings.Join([]string{
		"1. TestPreClaimReadiness_DecodesRewriteAcceptanceList",
		`2. cd cli && go test ./internal/agent/... -run "TestPreClaimReadiness_.*|TestLintHook_RejectsMalformedRationaleShape" -count=1`,
		"3. lefthook run pre-commit",
	}, "\n"), got.Rewrite.Acceptance)
}

func TestPreClaimReadiness_RejectsMalformedRewriteAcceptance(t *testing.T) {
	malformedCases := []struct {
		name     string
		accept   string
		wantFrag []string
	}{
		{
			name:   "object",
			accept: `{"value":"bad"}`,
			wantFrag: []string{
				"rewrite.acceptance",
				"string or string array",
			},
		},
		{
			name:   "number",
			accept: `42`,
			wantFrag: []string{
				"rewrite.acceptance",
				"string or string array",
			},
		},
		{
			name:   "non_string_array",
			accept: `["1. TestFoo",42]`,
			wantFrag: []string{
				"rewrite.acceptance",
				"cannot unmarshal",
			},
		},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}],"rewrite":{"changed_fields":["acceptance"],"acceptance":` + tc.accept + `}}`
			_, err := decodePreClaimIntakePayloadResultWithMode(payload, config.BeadQualityModeWarnOnly)
			require.Error(t, err)
			for _, frag := range tc.wantFrag {
				assert.Contains(t, err.Error(), frag)
			}
		})
	}

	t.Run("all_blank_array_rejected_by_validation", func(t *testing.T) {
		payload := `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}],"rewrite":{"changed_fields":["acceptance"],"acceptance":["   ",""," \t "]}}`
		got, err := decodePreClaimIntakePayloadResultWithMode(payload, config.BeadQualityModeWarnOnly)
		require.NoError(t, err)
		assert.Equal(t, PreClaimIntakeActionableButRewritten, got.Outcome)
		assert.Empty(t, got.Rewrite.Acceptance)

		original := &bead.Bead{
			ID:          "ddx-test-blank-acceptance",
			Description: "PROBLEM\nmissing verification\n\nROOT CAUSE\nno gate\n",
			Acceptance:  "1. keep original acceptance\n2. run tests\n",
		}
		_, _, _, _, err = validateAndApplyPreClaimIntakeRewrite(original, got.Rewrite)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "acceptance rewrite missing")
	})
}

func TestPreClaimReadiness_RejectsUnsupportedClassification(t *testing.T) {
	_, err := decodePreClaimIntakePayloadResultWithMode(`{"classification":"totally_new","rationale":"needs rewrite","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}]}`, config.BeadQualityModeWarnOnly)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown classification")
	for _, want := range []string{
		"ready",
		"needs_refine",
		"needs_split",
		"operator_required",
		"system_unready",
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLintHook_RejectsMalformedRationaleShape(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{
			ExitCode: 0,
			Output:   `{"score":7,"rationale":[{"criterion":"a","verdict":"fail"}],"suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLintHookBadJSON)
	assert.Contains(t, err.Error(), "rationale")
	assert.Contains(t, err.Error(), "string")
}

func TestBeadLifecycleSkillFilesStayInSyncOnContractTerms(t *testing.T) {
	skills := readSkillCopies(t)
	library := strings.TrimSpace(skills["library"])
	cli := strings.TrimSpace(skills["cli"])
	embedded := strings.TrimSpace(skills["embedded"])
	agents := strings.TrimSpace(skills["agents"])
	claude := strings.TrimSpace(skills["claude"])
	assert.Equal(t, library, cli, "skill copies should stay in sync for the documented lifecycle contract")
	assert.Equal(t, library, embedded, "embedded default-plugin skill copy should stay in sync with the source skill")
	assert.Equal(t, library, agents, ".agents skill copy should stay in sync with the source skill")
	assert.Equal(t, library, claude, ".claude skill copy should stay in sync with the source skill")
}
