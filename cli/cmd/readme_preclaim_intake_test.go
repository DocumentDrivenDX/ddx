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
		"ClassifyReadinessWithMode",
		"warn-only",
		"operator-attention",
		"hard errors",
		"worker idles on a full queue",
		"loop.idle",
		"preClaimIdleEscalationThreshold",
		"ddx-df77e668",
		"--claim-rate-window",
		"--claim-rate-threshold",
	}
	for _, term := range required {
		assert.Contains(t, readme, term, "README.md must document %q", term)
	}
}
