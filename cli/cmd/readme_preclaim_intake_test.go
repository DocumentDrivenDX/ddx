package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoRootForReadmeContractTest(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestReadmeDocumentsPreClaimIntakeContract(t *testing.T) {
	root := repoRootForReadmeContractTest(t)

	readmePath := filepath.Join(root, "README.md")
	data, err := os.ReadFile(readmePath)
	require.NoError(t, err)

	readme := string(data)
	required := []string{
		"Pre-claim Intake and Silent-Idle Diagnosis",
		"readiness_checks",
		"bool `true` -> `pass`",
		"bool `false` -> `fail`",
		"null",
		"absent",
		"ClassifyReadinessWithMode",
		"| Intake classification | Worker behavior | Operator signal |",
		"`needs_refine` in warn-only mode",
		"`needs_refine` in block/factory mode",
		"warn-only",
		"operator-attention",
		"loop.operator_attention",
		"hard errors",
		"worker idles on a full queue",
		"loop.idle",
		"same fingerprint",
		"preClaimIdleEscalationThreshold",
		"`5` consecutive `loop.idle` cycles",
		"`--preclaim-warn-threshold` defaults to `5`",
		"ddx-df77e668",
		"ddx work status --json",
		".ddx/agent-logs/agent-loop-*.jsonl",
		"preclaim_systemic",
		"preclaim_tracker_contention",
		"ddx bead operator-attention --json",
		"--claim-rate-window",
		"`--claim-rate-window` defaults to `10`",
		"--claim-rate-threshold",
		"`--claim-rate-threshold` defaults to `0.0`",
		"AR-2026-05-17",
		"ddx-57c40485",
		"ddx-8f2e0ebf",
	}
	for _, term := range required {
		assert.Contains(t, readme, term, "README.md must document %q", term)
	}
}
