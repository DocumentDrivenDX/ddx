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
		"warn-only",
		"operator-attention",
		"loop.operator_attention",
		"hard errors",
		"worker idles on a full queue",
		"loop.idle",
		"same fingerprint",
		"preClaimIdleEscalationThreshold",
		"ddx-df77e668",
		"--claim-rate-window",
		"--claim-rate-threshold",
		"AR-2026-05-17",
		"ddx-57c40485",
		"ddx-8f2e0ebf",
	}
	for _, term := range required {
		assert.Contains(t, readme, term, "README.md must document %q", term)
	}
}
