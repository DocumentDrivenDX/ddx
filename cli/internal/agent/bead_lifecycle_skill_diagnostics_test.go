package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMissingBeadLifecycleProjectRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project-root-file")
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o644))
	abs, err := filepath.Abs(root)
	require.NoError(t, err)
	return abs
}

func assertMissingBeadLifecycleDiagnostic(t *testing.T, msg, projectRoot string) {
	t.Helper()
	assert.Contains(t, msg, "skill missing: bead-lifecycle")
	assert.Contains(t, msg, "project_root="+projectRoot)
	assert.Contains(t, msg, filepath.Join(projectRoot, ".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"))
	assert.Contains(t, msg, filepath.Join(projectRoot, ".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"))
	assert.Contains(t, msg, "ddx update --force")
	assert.Contains(t, msg, "ddx doctor")
}

func TestHasBeadLifecycleSkillDiagnostics_FindsAgentsOrClaudePath(t *testing.T) {
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		filepath.Join(".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
	} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("skill"), 0o644))

			ok, diag := HasBeadLifecycleSkillDiagnostics(root)
			require.True(t, ok)
			assert.Equal(t, path, diag.FoundPath)
			require.Len(t, diag.CheckedPaths, 2)
		})
	}
}

func TestPostAttemptTriageHook_MissingSkillErrorIncludesProjectRootPathsAndRemediation(t *testing.T) {
	storeRoot := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, storeRoot)
	projectRoot := newMissingBeadLifecycleProjectRoot(t)

	hook := NewPostAttemptTriageHook(projectRoot, store, triageHookTestConfig(), nil, &triageHookRunnerStub{}, nil)
	_, err := hook(context.Background(), b.ID, ExecuteBeadReport{Status: ExecuteBeadStatusNoChanges})

	require.Error(t, err)
	assertMissingBeadLifecycleDiagnostic(t, err.Error(), projectRoot)
}

func TestRunPostAttemptTriage_MissingSkillStillFailOpen(t *testing.T) {
	storeRoot := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, storeRoot)
	projectRoot := newMissingBeadLifecycleProjectRoot(t)
	report := ExecuteBeadReport{
		BeadID:    b.ID,
		Status:    ExecuteBeadStatusNoChanges,
		Detail:    "zero diff",
		BaseRev:   "same",
		ResultRev: "same",
	}
	var log bytes.Buffer
	worker := &ExecuteBeadWorker{Store: store}
	hook := NewPostAttemptTriageHook(projectRoot, store, triageHookTestConfig(), nil, &triageHookRunnerStub{}, nil)

	got := worker.runPostAttemptTriage(context.Background(), *b, report, ExecuteBeadLoopRuntime{
		Log:                   &log,
		PostAttemptTriageHook: hook,
	}, "worker", time.Now)

	assert.Equal(t, report, got)
	assertMissingBeadLifecycleDiagnostic(t, log.String(), projectRoot)
}

func TestPreDispatchLintHook_MissingSkillUsesSameDiagnostic(t *testing.T) {
	projectRoot := newMissingBeadLifecycleProjectRoot(t)
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	hook := NewPreDispatchLintHook(projectRoot, nil, rcfg, nil, nil)

	_, err := hook(context.Background(), "ddx-missing-skill")

	require.Error(t, err)
	var lintErr *LintHookError
	require.ErrorAs(t, err, &lintErr)
	assert.Equal(t, LintHookErrorKindMissingSkill, lintErr.Kind)
	assertMissingBeadLifecycleDiagnostic(t, err.Error(), projectRoot)
}
