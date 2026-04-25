package agent

// execute_bead_env_isolation_test.go — Regression test for "Bug A":
// when execute-bead is invoked with hook-contaminated GIT_DIR /
// GIT_WORK_TREE / GIT_INDEX_FILE in the environment, every git call
// inside the agent path must operate on its target dir (the project
// repo or its worktree), NOT on the inherited outer repo. Any callsite
// that reverts to bare exec.Command("git", ...) without scrubbing the
// env will mutate the outer bare repo and fail this test.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

// repoSnapshot records every file under root by relative path → sha256(content).
// Symlinks are recorded by their link target. Used to assert byte-for-byte
// equivalence of an outer bare repo before/after a contaminated execute-bead run.
type repoSnapshot struct {
	files map[string]string // relPath -> sha256-hex of contents (or "L:"+target for symlinks)
}

func snapshotDir(t *testing.T, root string) repoSnapshot {
	t.Helper()
	snap := repoSnapshot{files: map[string]string{}}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.Type()&fs.ModeSymlink != 0 {
			tgt, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			snap.files[rel] = "L:" + tgt
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		snap.files[rel] = hex.EncodeToString(sum[:])
		return nil
	}))
	return snap
}

func (s repoSnapshot) diff(other repoSnapshot) []string {
	var diffs []string
	seen := map[string]bool{}
	for k, v := range s.files {
		seen[k] = true
		ov, ok := other.files[k]
		if !ok {
			diffs = append(diffs, "removed: "+k)
			continue
		}
		if ov != v {
			diffs = append(diffs, "modified: "+k)
		}
	}
	for k := range other.files {
		if !seen[k] {
			diffs = append(diffs, "added: "+k)
		}
	}
	sort.Strings(diffs)
	return diffs
}

// noopRunner is a minimal AgentRunner that creates a single file in the
// worktree (so SynthesizeCommit fires) and exits 0.
type envIsoRunner struct{}

func (envIsoRunner) Run(opts RunOptions) (*Result, error) {
	if opts.WorkDir != "" {
		path := filepath.Join(opts.WorkDir, "iso_marker.txt")
		_ = os.WriteFile(path, []byte("iso\n"), 0o644)
	}
	return &Result{ExitCode: 0}, nil
}

// TestExecuteBead_GitDirContaminatedEnv_LeavesOuterBareRepoUntouched is the
// regression test for Bug A: in-process execute-bead end-to-end (CommitTracker,
// WorktreeAdd, agent run, SynthesizeCommit, LandBeadResult preserve path) MUST
// NOT touch the outer bare repo named by inherited GIT_DIR / GIT_WORK_TREE /
// GIT_INDEX_FILE. Every git callsite in the agent path must scrub those vars
// (via internal/git.Command). If any callsite reverts to bare
// exec.Command("git", ...), this test will detect the mutation.
func TestExecuteBead_GitDirContaminatedEnv_LeavesOuterBareRepoUntouched(t *testing.T) {
	// 1. Build the project working repo + bead store with scrubbed env (the
	//    package-wide TestMain has already cleared GIT_*). newScriptHarnessRepo
	//    seeds one bead and returns the project root.
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	// 2. Build the fake outer bare repo that will play the role of the
	//    hook-inherited GIT_DIR. Use a sibling tempdir so a buggy git invocation
	//    that resolves the inherited GIT_DIR will land here, not in projectRoot.
	outerBase := t.TempDir()
	bareDir := filepath.Join(outerBase, "outer-bare.git")
	require.NoError(t, os.MkdirAll(bareDir, 0o755))
	bareInit := runGitInteg(t, outerBase, "init", "--bare", "outer-bare.git")
	_ = bareInit
	fakeWork := filepath.Join(outerBase, "fake-work")
	require.NoError(t, os.MkdirAll(fakeWork, 0o755))
	fakeIndex := filepath.Join(outerBase, "fake-index")
	// Touch the index file so a buggy callsite that tries to read it can; we
	// still expect the byte-identical assertion to catch any mutation.
	require.NoError(t, os.WriteFile(fakeIndex, []byte{}, 0o644))

	// 3. Snapshot the bare repo BEFORE the contaminated execute-bead run.
	before := snapshotDir(t, bareDir)
	require.NotEmpty(t, before.files, "bare repo snapshot must include files (config, HEAD, etc.)")

	// 4. Contaminate the env: set GIT_DIR / GIT_WORK_TREE / GIT_INDEX_FILE to
	//    the outer bare repo. t.Setenv restores the prior (unset) state on
	//    test exit. From this point on, any bare exec.Command("git", ...) in
	//    production code will operate on bareDir instead of cmd.Dir.
	t.Setenv("GIT_DIR", bareDir)
	t.Setenv("GIT_WORK_TREE", fakeWork)
	t.Setenv("GIT_INDEX_FILE", fakeIndex)

	// 5. Drive in-process execute-bead end-to-end. We do this directly (not via
	//    scriptHarnessExecutor) so we can pass NoMerge: true to LandBeadResult,
	//    exercising the preserve-ref path (UpdateRef in projectRoot) without
	//    requiring a remote.
	gitOps := &RealGitOps{}
	orchGitOps := &RealOrchestratorGitOps{}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "script",
	}).Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: envIsoRunner{},
	}, gitOps)
	require.NoError(t, err, "ExecuteBead should not fail under contaminated env")
	require.NotNil(t, res)

	landing, lerr := LandBeadResult(projectRoot, res, orchGitOps, BeadLandingOptions{
		NoMerge: true,
	})
	require.NoError(t, lerr, "LandBeadResult should not fail under contaminated env")
	require.NotNil(t, landing)

	// Sanity: the run should have either preserved (NoMerge with commits) or
	// reported no-changes. Either way, the bare repo must be untouched.
	switch landing.Outcome {
	case "preserved", "no-changes":
		// ok
	default:
		t.Fatalf("unexpected landing outcome %q (reason=%q); test cannot prove env isolation", landing.Outcome, landing.Reason)
	}

	// 6. Snapshot the bare repo AFTER and assert byte-for-byte equivalence.
	after := snapshotDir(t, bareDir)
	if diffs := before.diff(after); len(diffs) > 0 {
		t.Fatalf("outer bare repo was mutated by execute-bead under contaminated env "+
			"(this means a git callsite in the agent path inherited GIT_DIR=%s and "+
			"operated on the outer repo instead of cmd.Dir):\n  %s",
			bareDir, strings.Join(diffs, "\n  "))
	}

	// 7. Defence-in-depth: the bare repo's config bytes specifically must be
	//    byte-identical (the most common Bug A symptom is core.bare flipping or
	//    stray core.worktree entries appearing).
	cfgPath := filepath.Join(bareDir, "config")
	cfgBefore := before.files["config"]
	cfgAfter := after.files["config"]
	if cfgBefore == "" {
		t.Fatalf("bare repo config not captured in snapshot (path=%s)", cfgPath)
	}
	if cfgBefore != cfgAfter {
		raw, _ := os.ReadFile(cfgPath)
		t.Fatalf("bare repo config bytes mutated\n  path: %s\n  contents now:\n%s", cfgPath, string(raw))
	}
}
