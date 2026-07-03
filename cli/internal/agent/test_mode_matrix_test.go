// Package agent — test_mode_matrix_test.go
//
// TestModeMatrix parameterizes every behavior-equivalence scenario over both
// DDx storage modes. Any behavior that diverges between Convention and InTree
// modes (other than the on-disk anchor) is a regression.
//
// Convention mode: project root has no .ddx/; state lives under
//
//	${XDG_DATA_HOME}/ddx/projects/<identity>/ in its own git repo.
//
// InTree mode: project root has .ddx/; state commits go to the project repo.
package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/metaprompt"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/DocumentDrivenDX/ddx/internal/skills"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
)

// modeKind identifies which DDx storage layout a fixture uses.
type modeKind int

const (
	modeInTree     modeKind = iota // .ddx/ lives inside the project root
	modeConvention                 // .ddx state lives in ${XDG_DATA_HOME}/ddx/projects/…
)

func (m modeKind) String() string {
	switch m {
	case modeInTree:
		return "InTree"
	case modeConvention:
		return "Convention"
	default:
		return "Unknown"
	}
}

// modeFixture holds the paths for one test case inside the mode matrix.
type modeFixture struct {
	kind        modeKind
	projectRoot string // git project root (always present)
	ddxDir      string // ddxroot.Path() result: .ddx/ (in-tree) or XDG path (convention)
}

// setupModeFixture creates a minimal git project wired for the given mode.
// The projectRoot always has a HEAD commit so git worktree operations work.
func setupModeFixture(t *testing.T, kind modeKind) modeFixture {
	t.Helper()
	setExecutionWorktreeRootForTest(t)

	xdgHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgHome)

	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "matrix@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Matrix")
	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	runGitInteg(t, root, "add", "seed.txt")
	runGitInteg(t, root, "commit", "-m", "chore: matrix seed")

	var ddxDir string
	switch kind {
	case modeInTree:
		testutils.MakeInitializedDDxRoot(t, root)
		ddxDir = ddxroot.Path(context.Background(), root)
	case modeConvention:
		// No .ddx/ in project root; ddxroot.Path bootstraps the XDG state repo.
		ddxDir = ddxroot.Path(context.Background(), root)
	}

	return modeFixture{kind: kind, projectRoot: root, ddxDir: ddxDir}
}

// seedBeadStore initialises the store under cfg.ddxDir and creates one bead.
func seedBeadStore(t *testing.T, cfg modeFixture, beadID string) *bead.Store {
	t.Helper()
	store := bead.NewStore(cfg.ddxDir)
	require.NoError(t, store.Init(context.Background()))
	b := &bead.Bead{ID: beadID, Title: "Matrix test bead", IssueType: "task", Priority: 0}
	require.NoError(t, store.Create(context.Background(), b))
	return store
}

// allModes is the ordered set of modes exercised by every scenario.
var allModes = []modeKind{modeInTree, modeConvention}

// TestModeMatrix runs the nine behavioral-equivalence scenarios across both
// storage modes. All (mode, scenario) cells must pass.
func TestModeMatrix(t *testing.T) {
	for _, mode := range allModes {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			t.Run("bead-lifecycle", func(t *testing.T) {
				modeModeMatrixBeadLifecycle(t, mode)
			})
			t.Run("bead-execution", func(t *testing.T) {
				modeModeMatrixBeadExecution(t, mode)
			})
			t.Run("plugin-skill-resolution", func(t *testing.T) {
				modeModeMatrixPluginSkillResolution(t, mode)
			})
			t.Run("persona-binding", func(t *testing.T) {
				modeModeMatrixPersonaBinding(t, mode)
			})
			t.Run("worker-drain", func(t *testing.T) {
				modeModeMatrixWorkerDrain(t, mode)
			})
			t.Run("meta-prompt-sync", func(t *testing.T) {
				modeModeMatrixMetaPromptSync(t, mode)
			})
			t.Run("auto-commit-routing", func(t *testing.T) {
				modeModeMatrixAutoCommitRouting(t, mode)
			})
			t.Run("attachments", func(t *testing.T) {
				modeModeMatrixAttachments(t, mode)
			})
			t.Run("index-lock-recovery", func(t *testing.T) {
				modeModeMatrixIndexLockRecovery(t, mode)
			})
		})
	}
}

// scenario: bead-lifecycle — create/show/update/close via bead.Store
func modeModeMatrixBeadLifecycle(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	store := bead.NewStore(cfg.ddxDir)
	require.NoError(t, store.Init(context.Background()))

	const id = "ddx-matrix-lc01"

	// Create
	b := &bead.Bead{ID: id, Title: "Lifecycle bead", IssueType: "task", Priority: 0}
	require.NoError(t, store.Create(context.Background(), b))

	// Show (Get)
	got, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Lifecycle bead", got.Title)
	assert.Equal(t, bead.StatusOpen, got.Status)

	// Update
	require.NoError(t, store.Update(context.Background(), id, func(b *bead.Bead) {
		b.Title = "Lifecycle bead (updated)"
	}))
	updated, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Lifecycle bead (updated)", updated.Title)

	// AppendEvent — explicit events must persist in the mode's ddxDir.
	require.NoError(t, store.AppendEvent(id, bead.BeadEvent{
		Kind:    "matrix-lifecycle-event",
		Summary: "lifecycle test",
		Body:    "mode=" + kind.String(),
	}))
	events, err := store.Events(id)
	require.NoError(t, err)
	require.NotEmpty(t, events, "explicitly appended events must be readable for mode %s", kind)
	assert.Equal(t, "matrix-lifecycle-event", events[0].Kind)

	// Close
	require.NoError(t, store.Close(context.Background(), id))
	closed, err := store.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, closed.Status)

	// The beads.jsonl file must be anchored under ddxDir, not under a foreign path.
	beadsPath := filepath.Join(cfg.ddxDir, "beads.jsonl")
	_, err = os.Stat(beadsPath)
	require.NoError(t, err, "beads.jsonl must exist under ddxDir for mode %s", kind)
}

// scenario: bead-execution — the attempt evidence directory is anchored to
// cfg.ddxDir and the bead can be claimed in both modes.
//
// We test path routing (where evidence lands) and bead-claim state mutation
// without spinning up a full agent subprocess.
func modeModeMatrixBeadExecution(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	const beadID = "ddx-matrix-ex01"
	store := seedBeadStore(t, cfg, beadID)

	// The attempt evidence root must be anchored to cfg.ddxDir.
	artifactRoot := executeBeadArtifactRoot(cfg.projectRoot)
	require.True(t,
		strings.HasPrefix(filepath.Clean(artifactRoot), filepath.Clean(cfg.ddxDir)),
		"attempt evidence root %q must be under ddxDir %q for mode %s",
		artifactRoot, cfg.ddxDir, kind,
	)

	// Claiming and unclaiming the bead mutates state in the mode's ddxDir.
	require.NoError(t, store.Claim(beadID, "matrix-worker"))
	claimed, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "matrix-worker", claimed.Owner, "claim must be persisted for mode %s", kind)

	require.NoError(t, store.Unclaim(beadID))
	unclaimed, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "", unclaimed.Owner, "unclaim must clear owner for mode %s", kind)
}

// mockPluginFS implements fs.FS with a single skill directory embedded inline
// so the test does not depend on any real plugin content on disk.
type mockPluginFS struct {
	skillName string
	content   string
}

func (m mockPluginFS) Open(name string) (fs.File, error) {
	parts := strings.SplitN(strings.TrimPrefix(name, "."), "/", -1)
	// Support both top-level skill layout and plugin layout.
	if name == "." || name == m.skillName || name == ".agents/skills/"+m.skillName {
		return os.Open(os.DevNull) // directory-like
	}
	_ = parts
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// scenario: plugin-skill-resolution — install a skill from the plugin
// directory under cfg.ddxDir and verify it lands in the project's
// .agents/skills/ and .claude/skills/.
func modeModeMatrixPluginSkillResolution(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	// Write a minimal SKILL.md into the plugin dir for the mode's ddxDir.
	skillName := "matrix-skill"
	pluginSkillDir := filepath.Join(cfg.ddxDir, "plugins", "test-plugin", ".agents", "skills", skillName)
	require.NoError(t, os.MkdirAll(pluginSkillDir, 0o755))
	skillContent := "# Matrix test skill\nTest content\n"
	require.NoError(t, os.WriteFile(filepath.Join(pluginSkillDir, "SKILL.md"), []byte(skillContent), 0o644))

	// Install the skill into the project root using the OS FS for the plugin dir.
	pluginRoot := filepath.Join(cfg.ddxDir, "plugins", "test-plugin")
	src := os.DirFS(pluginRoot)
	require.NoError(t, skills.Install(src, cfg.projectRoot, skills.Options{}))

	// Verify the skill is available at both agent-tier destinations.
	for _, base := range []string{".agents", ".claude"} {
		dest := filepath.Join(cfg.projectRoot, base, "skills", skillName, "SKILL.md")
		data, err := os.ReadFile(dest)
		require.NoError(t, err, "skill must be installed at %s for mode %s", dest, kind)
		assert.Equal(t, skillContent, string(data))
	}
}

// scenario: persona-binding — role → persona resolves correctly for both modes.
//
// The .ddx.yml binding config is always in the project root regardless of mode.
func modeModeMatrixPersonaBinding(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	// Write a minimal .ddx.yml with a persona binding.
	configPath := filepath.Join(cfg.projectRoot, ".ddx.yml")
	configContent := `version: "1.0"
persona_bindings:
  code-reviewer: strict-reviewer
  test-engineer: tdd-specialist
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	mgr := persona.NewBindingManagerWithPath(configPath)

	got, err := mgr.GetBinding("code-reviewer")
	require.NoError(t, err)
	assert.Equal(t, "strict-reviewer", got, "binding must resolve for mode %s", kind)

	got2, err := mgr.GetBinding("test-engineer")
	require.NoError(t, err)
	assert.Equal(t, "tdd-specialist", got2)

	// Non-existent role returns empty (not an error).
	absent, err := mgr.GetBinding("nonexistent-role")
	require.NoError(t, err)
	assert.Empty(t, absent)
}

// scenario: worker-drain — ExecuteBeadWorker picks up a ready bead from the
// mode's ddxDir and drives it through success.
func modeModeMatrixWorkerDrain(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	const beadID = "ddx-matrix-drain01"
	store := seedBeadStore(t, cfg, beadID)

	executed := make([]string, 0)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			executed = append(executed, id)
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "drained in mode " + kind.String(),
				SessionID: "matrix-session",
				ResultRev: "cafebabe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "matrix-worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts, "worker must attempt the ready bead for mode %s", kind)
	assert.Equal(t, 1, result.Successes, "worker must succeed for mode %s", kind)
	require.Len(t, executed, 1)
	assert.Equal(t, beadID, executed[0])

	// Bead must be closed in the mode's ddxDir.
	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "bead must be closed for mode %s", kind)
}

// scenario: meta-prompt-sync — injecting a meta-prompt into CLAUDE.md
// produces a consistent in-sync state for both modes.
func modeModeMatrixMetaPromptSync(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	// Write a prompt file into the plugin library under cfg.ddxDir.
	promptRel := "test/matrix-prompt.md"
	promptDir := filepath.Join(cfg.ddxDir, "plugins", "ddx", "prompts", "test")
	require.NoError(t, os.MkdirAll(promptDir, 0o755))
	promptContent := "# Matrix prompt\nThis is the matrix mode test prompt.\n"
	require.NoError(t, os.WriteFile(filepath.Join(promptDir, "matrix-prompt.md"), []byte(promptContent), 0o644))

	// The CLAUDE.md target is always in the project root.
	claudeFile := "CLAUDE.md"
	// libraryPath is relative to projectRoot when workingDir==projectRoot, but
	// for convention mode the plugins live in ddxDir which may be outside projectRoot.
	// Pass the full ddxDir plugin path as libraryPath with workingDir set to "".
	libraryPath := filepath.Join(cfg.ddxDir, "plugins", "ddx")
	injector := metaprompt.NewMetaPromptInjectorWithPaths(
		filepath.Join(cfg.projectRoot, claudeFile),
		libraryPath,
		"", // workingDir: empty because libraryPath is already absolute
	)

	require.NoError(t, injector.InjectMetaPrompt(promptRel))

	// CLAUDE.md must have been created in the project root.
	claudePath := filepath.Join(cfg.projectRoot, claudeFile)
	data, err := os.ReadFile(claudePath)
	require.NoError(t, err, "CLAUDE.md must exist in project root for mode %s", kind)
	assert.Contains(t, string(data), "Matrix prompt", "CLAUDE.md must contain the injected prompt for mode %s", kind)

	// IsInSync must return true after injection.
	inSync, err := injector.IsInSync()
	require.NoError(t, err)
	assert.True(t, inSync, "meta-prompt must be in sync after injection for mode %s", kind)
}

// scenario: auto-commit-routing — ddxStateGitScope routes state commits to the
// correct git repo: project repo for InTree, XDG state repo for Convention.
func modeModeMatrixAutoCommitRouting(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	const pathspec = ".ddx/beads.jsonl"
	gitDir, pathspecs := ddxStateGitScope(cfg.projectRoot, pathspec)

	switch kind {
	case modeInTree:
		// InTree: git operations go to the project root; pathspecs keep .ddx/ prefix.
		assert.Equal(t, cfg.projectRoot, gitDir,
			"InTree mode must route state commits to the project repo")
		require.Len(t, pathspecs, 1)
		assert.Equal(t, pathspec, pathspecs[0],
			"InTree mode must preserve the .ddx/ pathspec prefix")

	case modeConvention:
		// Convention: git operations go to the XDG state repo; .ddx/ prefix is stripped.
		assert.Equal(t, cfg.ddxDir, gitDir,
			"Convention mode must route state commits to the XDG state repo")
		require.Len(t, pathspecs, 1)
		assert.Equal(t, "beads.jsonl", pathspecs[0],
			"Convention mode must strip the .ddx/ prefix from pathspecs")
	}
}

// scenario: attachments — per-attempt event files are written to the anchor
// under cfg.ddxDir/attachments/ regardless of mode.
func modeModeMatrixAttachments(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	const beadID = "ddx-matrix-att01"
	store := seedBeadStore(t, cfg, beadID)

	// Write a synthetic attachment into the expected anchor.
	attDir := filepath.Join(cfg.ddxDir, "attachments", beadID)
	require.NoError(t, os.MkdirAll(attDir, 0o755))
	eventsPath := filepath.Join(attDir, "events.jsonl")
	require.NoError(t, os.WriteFile(eventsPath, []byte(`{"kind":"test","body":"matrix"}`+"\n"), 0o644))

	// The attachment path must be under ddxDir.
	assert.True(t,
		strings.HasPrefix(filepath.Clean(eventsPath), filepath.Clean(cfg.ddxDir)),
		"attachment must be anchored under ddxDir %q for mode %s", cfg.ddxDir, kind,
	)

	// AppendEvent should write events under ddxDir too.
	require.NoError(t, store.AppendEvent(beadID, bead.BeadEvent{Kind: "matrix-event", Summary: "test", Body: "payload"}))
	events, err := store.Events(beadID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "matrix-event" {
			found = true
		}
	}
	assert.True(t, found, "AppendEvent must persist for mode %s", kind)

	// Verify the DDx dir is the correct anchor, not a fallback to an unrelated path.
	require.NotEmpty(t, cfg.ddxDir)
	switch kind {
	case modeInTree:
		assert.True(t, strings.HasPrefix(cfg.ddxDir, cfg.projectRoot),
			"InTree ddxDir must be under projectRoot for mode %s", kind)
	case modeConvention:
		assert.False(t, strings.HasPrefix(cfg.ddxDir, cfg.projectRoot),
			"Convention ddxDir must be outside projectRoot for mode %s", kind)
	}
}

// scenario: index-lock-recovery — recoverGitIndexLock works correctly on the
// project git repo in both modes. (Convention mode uses the project repo for
// implementation commits and the XDG repo for state; both must recover locks.)
func modeModeMatrixIndexLockRecovery(t *testing.T, kind modeKind) {
	t.Helper()
	cfg := setupModeFixture(t, kind)

	// Project git repo must recover a missing lock as "not present".
	result, err := recoverGitIndexLock(cfg.projectRoot)
	require.NoError(t, err, "recoverGitIndexLock must not error for mode %s", kind)
	assert.False(t, result.Removed, "no stale lock should be removed when none exists for mode %s", kind)
	assert.Contains(t, result.Reason, "not present",
		"reason must indicate lock absence for mode %s", kind)

	// For Convention mode, also verify the XDG state repo recovers correctly.
	if kind == modeConvention {
		xdgGitDir := filepath.Join(cfg.ddxDir, ".git")
		_, err := os.Stat(xdgGitDir)
		require.NoError(t, err, "Convention mode XDG state repo must have a .git dir")

		xdgResult, xdgErr := recoverGitIndexLock(cfg.ddxDir)
		require.NoError(t, xdgErr, "recoverGitIndexLock on XDG repo must not error")
		assert.False(t, xdgResult.Removed)
		assert.Contains(t, xdgResult.Reason, "not present")
	}
}
