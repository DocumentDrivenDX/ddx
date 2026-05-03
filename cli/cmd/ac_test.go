package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// initRepoWithBead creates a fresh git repo with one committed file, an
// initialized .ddx bead store containing one bead, and (optionally) writes
// the given check YAML under .ddx/checks/.
func initRepoWithBead(t *testing.T, beadID string, labels []string, checkYAML string) string {
	t.Helper()
	root := t.TempDir()

	gitInit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitInit("init", "-q", "-b", "main")
	gitInit("config", "user.email", "t@example.com")
	gitInit("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	gitInit("add", ".")
	gitInit("commit", "-q", "-m", "init")

	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "ac test", Labels: labels}))

	if checkYAML != "" {
		checksDir := filepath.Join(ddxDir, "checks")
		require.NoError(t, os.MkdirAll(checksDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(checksDir, "fixture.yaml"), []byte(checkYAML), 0o644))
	}

	return root
}

func TestAcRun_PassExitsZero(t *testing.T) {
	beadID := "ddx-ac-pass"
	yaml := `
name: passer
command: 'printf ''{"status":"pass","message":"ok"}'' > "$EVIDENCE_DIR/$CHECK_NAME.json"'
when: pre_merge
`
	root := initRepoWithBead(t, beadID, nil, yaml)

	out, err := executeCommand(NewCommandFactory(root).NewRootCommand(), "ac", "run", beadID)
	require.NoError(t, err)
	require.Contains(t, out, "[PASS] passer")
}

func TestAcRun_BlockExitsNonZero(t *testing.T) {
	beadID := "ddx-ac-block"
	yaml := `
name: blocker
command: 'printf ''{"status":"block","message":"nope"}'' > "$EVIDENCE_DIR/$CHECK_NAME.json"'
when: pre_merge
`
	root := initRepoWithBead(t, beadID, nil, yaml)

	out, err := executeCommand(NewCommandFactory(root).NewRootCommand(), "ac", "run", beadID)
	require.Error(t, err)
	require.Contains(t, out, "[BLOCK] blocker")
}

func TestAcRun_ErrorOnMissingResultFile(t *testing.T) {
	beadID := "ddx-ac-err"
	yaml := `
name: silent
command: 'true'
when: pre_merge
`
	root := initRepoWithBead(t, beadID, nil, yaml)

	out, err := executeCommand(NewCommandFactory(root).NewRootCommand(), "ac", "run", beadID)
	require.Error(t, err)
	require.Contains(t, out, "[ERROR] silent")
}

func TestAcRun_AppliesToFiltersByLabel(t *testing.T) {
	beadID := "ddx-ac-filter"
	yaml := `
name: skipped
command: 'printf ''{"status":"block"}'' > "$EVIDENCE_DIR/$CHECK_NAME.json"'
when: pre_merge
applies_to:
  labels: [area:other]
`
	// Bead has label area:foo, check filters on area:other → should be skipped.
	root := initRepoWithBead(t, beadID, []string{"area:foo"}, yaml)

	out, err := executeCommand(NewCommandFactory(root).NewRootCommand(), "ac", "run", beadID)
	require.NoError(t, err, "filter should skip the only check, leaving zero failures")
	require.NotContains(t, out, "skipped")
	// Header line is still printed.
	require.Contains(t, out, "ac run")
}

func TestAcRun_OnlyFlag(t *testing.T) {
	beadID := "ddx-ac-only"
	root := initRepoWithBead(t, beadID, nil, "")
	checksDir := filepath.Join(root, ".ddx", "checks")
	require.NoError(t, os.MkdirAll(checksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(checksDir, "a.yaml"), []byte(`
name: alpha
command: 'printf ''{"status":"pass"}'' > "$EVIDENCE_DIR/$CHECK_NAME.json"'
when: pre_merge
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(checksDir, "b.yaml"), []byte(`
name: beta
command: 'printf ''{"status":"block"}'' > "$EVIDENCE_DIR/$CHECK_NAME.json"'
when: pre_merge
`), 0o644))

	out, err := executeCommand(NewCommandFactory(root).NewRootCommand(),
		"ac", "run", beadID, "--check", "alpha")
	require.NoError(t, err)
	require.Contains(t, out, "[PASS] alpha")
	require.NotContains(t, out, "beta")
}

func TestAcRun_UnknownBeadFails(t *testing.T) {
	root := initRepoWithBead(t, "ddx-ac-known", nil, "")
	_, err := executeCommand(NewCommandFactory(root).NewRootCommand(),
		"ac", "run", "ddx-does-not-exist")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "load bead") || strings.Contains(err.Error(), "not found"),
		"unexpected error: %v", err)
}
