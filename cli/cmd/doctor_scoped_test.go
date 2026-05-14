package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeWorkDirWithLegacySymlink creates a temp workdir containing a legacy symlink
// under .agents/skills/, which causes full-scope doctor to exit non-zero.
func makeWorkDirWithLegacySymlink(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	agentSkillsDir := filepath.Join(workDir, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentSkillsDir, 0o755))
	require.NoError(t, os.Symlink("/nonexistent/old-global-path/helix-align", filepath.Join(agentSkillsDir, "helix-align")))
	return workDir
}

// TestDoctorScopedToConfigChange asserts that `ddx doctor --paths .ddx.yml`
// only runs config-related checks and exits 0 when other repo state (e.g.
// legacy skill symlinks) would have failed a full-scope doctor. [ddx-777f4a09 AC1]
func TestDoctorScopedToConfigChange(t *testing.T) {
	workDir := makeWorkDirWithLegacySymlink(t)

	factory := NewCommandFactory(workDir)
	_, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--paths", ".ddx.yml")
	require.NoError(t, err, "doctor --paths .ddx.yml must exit 0 even when legacy symlinks are present")
}

// TestDoctorScopedToResourceChange asserts that `ddx doctor --paths templates/foo`
// only runs resource-related checks and exits 0 when legacy skill symlinks would
// have failed a full-scope doctor. [ddx-777f4a09 AC2]
func TestDoctorScopedToResourceChange(t *testing.T) {
	workDir := makeWorkDirWithLegacySymlink(t)

	for _, path := range []string{
		"templates/example.md",
		"patterns/coding-style.md",
		"prompts/review.md",
	} {
		t.Run(path, func(t *testing.T) {
			factory := NewCommandFactory(workDir)
			_, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--paths", path)
			require.NoError(t, err, "doctor --paths %s must exit 0 even when legacy symlinks are present", path)
		})
	}
}

// TestDoctorFullScopeStillFails is a regression guard: `ddx doctor` with no
// --paths argument must still surface the legacy-symlink finding. [ddx-777f4a09 AC3]
func TestDoctorFullScopeStillFails(t *testing.T) {
	workDir := makeWorkDirWithLegacySymlink(t)

	factory := NewCommandFactory(workDir)
	cmd := factory.NewRootCommand()
	cmd.SetArgs([]string{"doctor"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit from full-scope doctor with legacy symlinks, got nil")
	}
}

// TestDoctorHookPathsDoesNotInvokeFullScope simulates the lefthook ddx-validate
// pre-commit hook passing a space-separated staged path list to --paths. Confirms
// that mixed staged files (config + resource) do not trigger the full-scope
// legacy-symlink check. [ddx-777f4a09 AC4]
func TestDoctorHookPathsDoesNotInvokeFullScope(t *testing.T) {
	workDir := makeWorkDirWithLegacySymlink(t)

	// Simulate the hook passing staged paths including both .ddx.yml and a template.
	factory := NewCommandFactory(workDir)
	_, err := executeWithStdoutCapture(t, factory.NewRootCommand(),
		"doctor", "--paths", ".ddx.yml templates/example.md")
	require.NoError(t, err, "hook-style --paths with multiple staged files must not invoke full-scope doctor")
}

// TestCategorizeStagedPaths_Config verifies that .ddx.yml paths map to the config category.
func TestCategorizeStagedPaths_Config(t *testing.T) {
	cats := categorizeStagedPaths([]string{".ddx.yml"})
	if !cats["config"] {
		t.Error("expected config category for .ddx.yml")
	}
	if cats["resource"] {
		t.Error("did not expect resource category for .ddx.yml")
	}
}

// TestCategorizeStagedPaths_Resource verifies that templates/, patterns/, prompts/ paths
// map to the resource category.
func TestCategorizeStagedPaths_Resource(t *testing.T) {
	for _, p := range []string{"templates/foo.md", "patterns/bar.md", "prompts/baz.md"} {
		cats := categorizeStagedPaths([]string{p})
		if !cats["resource"] {
			t.Errorf("expected resource category for %s", p)
		}
		if cats["config"] {
			t.Errorf("did not expect config category for %s", p)
		}
	}
}

// TestCategorizeStagedPaths_Unrecognised verifies that unrecognised paths produce no category.
func TestCategorizeStagedPaths_Unrecognised(t *testing.T) {
	cats := categorizeStagedPaths([]string{"cli/cmd/foo.go", "README.md", ""})
	if len(cats) != 0 {
		t.Errorf("expected no categories for unrecognised paths, got %v", cats)
	}
}
