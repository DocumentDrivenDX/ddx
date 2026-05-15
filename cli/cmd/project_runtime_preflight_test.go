package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectRuntimePreflight_MissingBeadLifecycleReportsCheckedPathsAndRemediation
// verifies that a project with .agents/skills/ddx/SKILL.md but no nested
// bead-lifecycle directory reports the resolved project root, both checked
// nested skill paths, and the remediation commands.
func TestProjectRuntimePreflight_MissingBeadLifecycleReportsCheckedPathsAndRemediation(t *testing.T) {
	projectRoot := t.TempDir()
	// Create .agents/skills/ddx/SKILL.md — parent dir exists but bead-lifecycle is absent.
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".agents", "skills", "ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".agents", "skills", "ddx", "SKILL.md"), []byte("# DDx Skills"), 0o644))

	result := checkProjectRuntimePreflight(projectRoot)

	assert.Equal(t, projectRoot, result.ProjectRoot)
	assert.True(t, result.MissingBeadLifecycle, "bead-lifecycle must be reported missing")
	require.Len(t, result.CheckedPaths, 2, "exactly two paths must be checked")
	assert.Contains(t, result.CheckedPaths[0], filepath.Join(".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"))
	assert.Contains(t, result.CheckedPaths[1], filepath.Join(".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"))

	// Verify warning output contains all required information.
	var buf bytes.Buffer
	emitPreflightWarning(&buf, result)
	out := buf.String()

	assert.Contains(t, out, projectRoot, "output must include resolved project root")
	assert.Contains(t, out, filepath.Join(".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"), "output must include first checked path")
	assert.Contains(t, out, filepath.Join(".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"), "output must include second checked path")
	assert.Contains(t, out, "ddx update --force", "output must include primary remediation command")
	assert.Contains(t, out, "ddx doctor", "output must include secondary remediation command")
}

// TestWorkPreflight_WarnsOnceForMissingLifecycleSkill verifies that ddx work
// emits one preflight warning before dispatch and does not repeat the warning
// across loop iterations in the same process (via sync.Once on the factory).
func TestWorkPreflight_WarnsOnceForMissingLifecycleSkill(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	env := NewTestEnvironment(t)
	// No bead-lifecycle skill in env.Dir.

	factory := NewCommandFactory(env.Dir)

	// First invocation must emit the preflight warning (non-JSON mode so the
	// warning is not suppressed for machine-readable consumers).
	var buf1 bytes.Buffer
	root1 := factory.NewRootCommand()
	root1.SetOut(&buf1)
	root1.SetErr(&buf1)
	root1.SetArgs([]string{"work", "--once"})
	_ = root1.Execute()
	out1 := buf1.String()
	assert.Contains(t, out1, "preflight warning:", "first invocation must emit the preflight warning")
	assert.Contains(t, out1, "ddx update --force")

	// Second invocation on the same factory must not repeat the warning.
	var buf2 bytes.Buffer
	root2 := factory.NewRootCommand()
	root2.SetOut(&buf2)
	root2.SetErr(&buf2)
	root2.SetArgs([]string{"work", "--once"})
	_ = root2.Execute()
	out2 := buf2.String()
	assert.NotContains(t, out2, "preflight warning:", "warning must not repeat across loop iterations in the same process")
}

// TestTryPreflight_WarnsBeforeClaimForMissingLifecycleSkill verifies that
// ddx try runs the preflight before claim/worktree setup and includes the
// remediation text.
func TestTryPreflight_WarnsBeforeClaimForMissingLifecycleSkill(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	beadID := "ddx-preflight-try"
	seedOpenBead(t, env.Dir, beadID)
	// No bead-lifecycle skill installed (NewTestEnvironment does not install it).

	factory := NewCommandFactory(env.Dir)
	// AgentRunnerOverride prevents the lifecycle-hook runners (lint, triage) from
	// making real network calls to fizeau/openrouter during the test.
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(_ context.Context, id string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    id,
			Status:    agent.ExecuteBeadStatusNoChanges,
			BaseRev:   "abc1234",
			ResultRev: "abc1234",
		}, nil
	})

	root := factory.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"try", beadID, "--no-review", "--no-review-i-know-what-im-doing"})
	_ = root.Execute()

	out := buf.String()
	assert.Contains(t, out, "preflight warning:", "preflight warning must appear in output")
	assert.Contains(t, out, "ddx update --force", "remediation must include ddx update --force")
	assert.Contains(t, out, "ddx doctor", "remediation must include ddx doctor")

	// Warning must precede any bead dispatch output.
	warnIdx := strings.Index(out, "preflight warning:")
	beadLine := fmt.Sprintf("bead: %s", beadID)
	beadIdx := strings.Index(out, beadLine)
	if warnIdx >= 0 && beadIdx >= 0 {
		assert.Less(t, warnIdx, beadIdx, "preflight warning must appear before bead dispatch output")
	}
}

// TestServerPreflight_MissingLifecycleSkillStartsDegraded verifies that
// ddx server surfaces degraded startup diagnostics without blocking server
// construction/startup solely because bead-lifecycle is missing.
func TestServerPreflight_MissingLifecycleSkillStartsDegraded(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot := t.TempDir()
	// No bead-lifecycle skill installed.

	factory := NewCommandFactory(projectRoot)
	// Override ListenAndServeTLS so the server returns immediately without binding.
	factory.serverListenAndServeOverride = func(cert, key string) error { return nil }

	root := factory.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"server", "--tsnet=false"})
	err := root.Execute()

	// Missing bead-lifecycle must not cause the server to error.
	assert.NoError(t, err, "ddx server must not fail because bead-lifecycle is absent")

	out := buf.String()
	// Degraded startup diagnostics must be surfaced.
	assert.Contains(t, out, "bead-lifecycle", "degraded diagnostics must mention bead-lifecycle")
	assert.Contains(t, out, "ddx update --force", "degraded diagnostics must include primary remediation")
}

// TestProjectRuntimePreflight_IgnoresNonDDxProjectSkillSymlink verifies that
// project-owned skill symlinks do not degrade runtime preflight when the DDx
// skill layout itself is healthy.
func TestProjectRuntimePreflight_IgnoresNonDDxProjectSkillSymlink(t *testing.T) {
	projectRoot := t.TempDir()

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "bead-lifecycle"),
		filepath.Join(".claude", "skills", "ddx", "bead-lifecycle"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, rel), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, rel, "SKILL.md"), []byte("# bead lifecycle"), 0o644))
	}
	require.NoError(t, os.Symlink("../../skills/helix", filepath.Join(projectRoot, ".agents", "skills", "helix")))
	require.NoError(t, os.Symlink("../../skills/helix", filepath.Join(projectRoot, ".claude", "skills", "helix")))

	result := checkProjectRuntimePreflight(projectRoot)

	assert.False(t, result.MissingBeadLifecycle, "bead-lifecycle must be found")
	assert.Empty(t, result.LegacySymlinkDirs, "non-DDx project skill symlinks must be ignored")

	var buf bytes.Buffer
	emitPreflightWarning(&buf, result)
	assert.Empty(t, buf.String(), "healthy DDx layout must not emit a preflight warning")
}

// TestProjectRuntimePreflight_LegacyDDxSkillSymlinkSuggestsUpdateForce verifies
// that legacy DDx skill symlinks reuse the FEAT-015 remediation path:
// ddx update --force first, then ddx doctor.
func TestProjectRuntimePreflight_LegacyDDxSkillSymlinkSuggestsUpdateForce(t *testing.T) {
	projectRoot := t.TempDir()
	legacyDDxTarget := filepath.Join(projectRoot, "legacy-skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(legacyDDxTarget, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDDxTarget, "SKILL.md"), []byte("# bead lifecycle"), 0o644))

	agentSkillsDir := filepath.Join(projectRoot, ".agents", "skills")
	require.NoError(t, os.MkdirAll(agentSkillsDir, 0o755))
	require.NoError(t, os.Symlink("../../legacy-skills/ddx", filepath.Join(agentSkillsDir, "ddx")))

	result := checkProjectRuntimePreflight(projectRoot)

	assert.NotEmpty(t, result.LegacySymlinkDirs, "legacy DDx skill symlink must be detected")

	var buf bytes.Buffer
	emitPreflightWarning(&buf, result)
	out := buf.String()

	// ddx update --force must appear before ddx doctor (primary remediation first).
	updateIdx := strings.Index(out, "ddx update --force")
	doctorIdx := strings.Index(out, "ddx doctor")
	assert.GreaterOrEqual(t, updateIdx, 0, "output must contain ddx update --force")
	assert.GreaterOrEqual(t, doctorIdx, 0, "output must contain ddx doctor")
	assert.Less(t, updateIdx, doctorIdx, "ddx update --force must appear before ddx doctor")
}
