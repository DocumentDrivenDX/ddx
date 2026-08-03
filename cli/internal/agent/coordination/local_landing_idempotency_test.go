package coordination_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinationContract_LocalLandingIdempotency verifies the coordination
// landing contract against the local implementation using a real temporary
// bead store and git repository (ddx-f7b012d6).
//
// Flow: create a worker commit on a real repo, land it via LocalCoordinator
// wired to agent.NewCoordinationLandBackend(RealLandingGitOps), then replay
// the same idempotency key and observe OutcomeAlreadyApplied without
// call-recording mocks. Production path is agent.Land — not a fake recorder.
func TestCoordinationContract_LocalLandingIdempotency(t *testing.T) {
	projectRoot := t.TempDir()
	// Keep the real landing path off the host-global DDx config so the temp
	// finalization worktree always lands in a writable scratch root.
	scratchRoot := t.TempDir()
	t.Setenv(config.ExecutionWorktreeRootEnv, filepath.Join(scratchRoot, "exec-wt"))
	initGitRepo(t, projectRoot)
	// Production land path expects execution evidence + lock paths gitignored.
	writeFile(t, projectRoot, ".gitignore", strings.Join([]string{
		".ddx/executions/",
		".ddx/.git-tracker.lock",
		".ddx/.git-tracker.lock.*",
		"",
	}, "\n"))
	writeFile(t, projectRoot, "README.md", "# coordination land fixture\n")
	runGit(t, projectRoot, "add", "-A")
	runGit(t, projectRoot, "commit", "-m", "init land fixture")
	baseSHA := strings.TrimSpace(runGitOutput(t, projectRoot, "rev-parse", "HEAD"))

	testutils.MakeInitializedDDxRoot(t, projectRoot)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-land-idempotency-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "coordination landing idempotency fixture",
	}))
	// Seed tracker into git history so the worktree matches a real project.
	runGit(t, projectRoot, "add", "-A")
	runGit(t, projectRoot, "commit", "-m", "seed bead store for land fixture")
	// Re-resolve base after seed commit — worker branches from current tip.
	baseSHA = strings.TrimSpace(runGitOutput(t, projectRoot, "rev-parse", "HEAD"))

	// Worker contribution: commit on base in a throwaway worktree (same
	// pattern as agent land unit tests) so ResultRev is in the object store
	// but not yet on main.
	resultSHA := commitOnBase(t, projectRoot, baseSHA, "feature.txt", "landed-by-coordination\n", "feat: land coordination fixture")

	// Production land backend: real agent.Land + RealLandingGitOps.
	landBackend := agent.NewCoordinationLandBackend(projectRoot, agent.RealLandingGitOps{})
	coord := coordination.NewLocalCoordinatorWithLand(store, landBackend)
	ctx := context.Background()

	const landKey = "land-key-idempotency-1"
	first, err := coord.Land(ctx, coordination.LandRequest{
		ProjectRoot:    projectRoot,
		WorktreeDir:    projectRoot,
		BaseRev:        baseSHA,
		ResultRev:      resultSHA,
		BeadID:         beadID,
		AttemptID:      "20260724T000000-land1",
		TargetBranch:   "main",
		IdempotencyKey: landKey,
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeApplied, first.Code, "first land must apply")
	assert.Equal(t, coordination.LandStatusLanded, first.Status, "first land must land")
	assert.Equal(t, beadID, first.BeadID)
	assert.Equal(t, resultSHA, first.NewTip, "fast-forward tip should be worker result")
	assert.Equal(t, landKey, first.IdempotencyKey)
	assert.False(t, first.Merged, "same-base land should fast-forward")
	assert.Empty(t, first.Reason, "clean land must not synthesize a reason")

	// Durable git truth after applied land.
	mainTip := strings.TrimSpace(runGitOutput(t, projectRoot, "rev-parse", "refs/heads/main"))
	assert.Equal(t, resultSHA, mainTip, "main must advance to ResultRev")
	featurePath := filepath.Join(projectRoot, "feature.txt")
	content, readErr := os.ReadFile(featurePath)
	require.NoError(t, readErr, "land must materialize worker files in the worktree")
	assert.Equal(t, "landed-by-coordination\n", string(content))

	// Replay same idempotency key: already_applied without re-invoking land.
	// Tip must remain stable (no second land attempt).
	replay, err := coord.Land(ctx, coordination.LandRequest{
		ProjectRoot:    projectRoot,
		WorktreeDir:    projectRoot,
		BaseRev:        baseSHA,
		ResultRev:      resultSHA,
		BeadID:         beadID,
		AttemptID:      "20260724T000000-land1",
		TargetBranch:   "main",
		IdempotencyKey: landKey,
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeAlreadyApplied, replay.Code,
		"same-key replay must be already_applied")
	assert.Equal(t, coordination.LandStatusLanded, replay.Status,
		"already_applied echoes the prior land status")
	assert.Equal(t, resultSHA, replay.NewTip,
		"already_applied echoes the prior NewTip")
	assert.Equal(t, beadID, replay.BeadID)
	assert.Equal(t, landKey, replay.IdempotencyKey)
	assert.Empty(t, replay.Reason, "already_applied replay must not synthesize a reason")

	// Git tip unchanged after idempotent replay.
	mainTipAfter := strings.TrimSpace(runGitOutput(t, projectRoot, "rev-parse", "refs/heads/main"))
	assert.Equal(t, resultSHA, mainTipAfter, "idempotent replay must not re-land")
}

// commitOnBase creates a detached commit at baseSHA in a throwaway worktree
// and returns the new commit SHA (reachable in the object store, not on main).
func commitOnBase(t *testing.T, repo, baseSHA, path, content, msg string) string {
	t.Helper()
	wt, err := os.MkdirTemp("", "coord-land-wt-*")
	skipFilesystemAvailabilityError(t, "create landing worktree", err)
	_ = os.RemoveAll(wt)
	runGit(t, repo, "worktree", "add", "--detach", wt, baseSHA)
	defer func() {
		runGit(t, repo, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()

	writeFile(t, wt, path, content)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = wt
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add: %s", out)

	cmd = exec.Command("git",
		"-c", "user.name=Coordination Test",
		"-c", "user.email=coordination@test.local",
		"commit", "-m", msg)
	cmd.Dir = wt
	cmd.Env = cleanGitEnv()
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wt
	cmd.Env = cleanGitEnv()
	out, err = cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func skipFilesystemAvailabilityError(t *testing.T, op string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	if errors.Is(err, fs.ErrPermission) || strings.Contains(msg, "read-only file system") || strings.Contains(msg, "permission denied") {
		t.Skipf("%s unavailable in this environment: %v", op, err)
	}
	require.NoError(t, err, op)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
	return string(out)
}

func cleanGitEnv() []string {
	env := os.Environ()
	cleaned := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		cleaned = append(cleaned, kv)
	}
	return cleaned
}
