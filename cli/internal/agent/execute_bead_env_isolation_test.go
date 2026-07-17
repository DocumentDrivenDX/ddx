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
	"github.com/DocumentDrivenDX/ddx/internal/gitrepohealth"
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

func (envIsoRunner) Run(opts RunArgs) (*Result, error) {
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
	orchGitOps := &RealGitOps{}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "script"})
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

func TestExecuteBeadWorkerCannotMutatePrimaryGitConfig(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	runGitInteg(t, projectRoot, "config", "extensions.worktreeConfig", "true")

	directivePath := filepath.Join(t.TempDir(), "mutate-git-config.txt")
	writeDirectiveFile(t, directivePath, []string{
		"run git config --local core.bare true",
		"run git config --local user.email fixture@ddx.test",
		"run git config --local user.name DDxFixture",
		"run git config --local core.worktree $PWD/redirected-worktree",
		"run wt_gitdir=$(sed -n 's/^gitdir: //p' .git); GIT_DIR=\"$wt_gitdir\" GIT_WORK_TREE=\"$PWD\" GIT_INDEX_FILE=\"$wt_gitdir/index\" git config --local user.email nested@ddx.test",
		"set-exit 0",
	})

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Model: directivePath,
	}).Resolve(config.CLIOverrides{Harness: "script", Model: directivePath})

	gitOps := &RealGitOps{}
	orchGitOps := &RealGitOps{}
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{}, gitOps)
	require.NoError(t, err)
	require.NotNil(t, res)

	landing, lerr := LandBeadResult(projectRoot, res, orchGitOps, BeadLandingOptions{NoMerge: true})
	require.NoError(t, lerr)
	require.NotNil(t, landing)

	_, statusErr := runGitIntegOutput(projectRoot, "status", "--short")
	require.NoError(t, statusErr, "primary checkout must remain a valid worktree after execute-bead contamination attempt")

	bareOut, bareErr := runGitIntegOutput(projectRoot, "config", "--local", "--get", "core.bare")
	if bareErr == nil && strings.TrimSpace(bareOut) == "true" {
		t.Fatalf("primary checkout leaked core.bare=true")
	}

	worktreeOut, worktreeErr := runGitIntegOutput(projectRoot, "config", "--local", "--get", "core.worktree")
	require.Error(t, worktreeErr, "primary checkout must not retain core.worktree")
	require.Empty(t, strings.TrimSpace(worktreeOut))

	configPaths := []string{
		filepath.Join(projectRoot, ".git", "config"),
		filepath.Join(projectRoot, ".git", "config.worktree"),
	}
	for _, path := range configPaths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			t.Fatalf("read primary git config %s: %v", path, readErr)
		}
		text := string(data)
		for _, banned := range []string{"fixture@ddx.test", "nested@ddx.test", "DDxFixture"} {
			if strings.Contains(text, banned) {
				t.Fatalf("primary git config leaked %q into %s:\n%s", banned, path, text)
			}
		}
	}
}

// TestAgentGitConfigFixturesDoNotLeakToPrimaryLinkedWorktree proves the test
// fixture boundary itself, rather than only ExecuteBead's harness wrapper.
// The parent process deliberately points Git at a real primary repository;
// runGitInteg/newScriptHarnessRepo must scrub that selection and make the
// fixture repair only its own temporary repository.
func TestAgentGitConfigFixturesDoNotLeakToPrimaryLinkedWorktree(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	runGitInteg(t, filepath.Dir(primary), "init", "-b", "main", primary)
	runGitInteg(t, primary, "config", "user.name", "Primary Fixture Guard")
	runGitInteg(t, primary, "config", "user.email", "primary-fixture-guard@ddx.test")
	require.NoError(t, os.WriteFile(filepath.Join(primary, "seed.txt"), []byte("seed\n"), 0o644))
	runGitInteg(t, primary, "add", "seed.txt")
	runGitInteg(t, primary, "commit", "-m", "chore: seed primary fixture guard")
	runGitInteg(t, primary, "config", "extensions.worktreeConfig", "true")
	runGitInteg(t, primary, "config", "--worktree", "user.name", "Primary Worktree Guard")

	linked := filepath.Join(t.TempDir(), "linked")
	runGitInteg(t, primary, "worktree", "add", "-b", "fixture-linked", linked, "HEAD")
	t.Cleanup(func() { _ = runGitInteg(t, primary, "worktree", "remove", "--force", linked) })

	primaryConfig := filepath.Join(primary, ".git", "config")
	primaryWorktreeConfig := filepath.Join(primary, ".git", "config.worktree")
	beforeConfig, err := os.ReadFile(primaryConfig)
	require.NoError(t, err)
	beforeWorktreeConfig, err := os.ReadFile(primaryWorktreeConfig)
	require.NoError(t, err)
	beforeValues := fixtureConfigValues(t, primary)

	// This is the failure mode observed on the shared checkout: a caller's Git
	// repository selection survives into an agent fixture. If a fixture helper
	// ever stops using fixtureGitEnvInteg, its `git config core.bare true` below
	// targets primary and the byte-identity checks fail.
	t.Setenv("GIT_DIR", filepath.Join(primary, ".git"))
	t.Setenv("GIT_WORK_TREE", primary)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(primary, ".git", "index"))
	runCoreBareRepairFixture(t)

	afterConfig, err := os.ReadFile(primaryConfig)
	require.NoError(t, err)
	afterWorktreeConfig, err := os.ReadFile(primaryWorktreeConfig)
	require.NoError(t, err)
	require.Equal(t, beforeConfig, afterConfig, "fixture must not rewrite primary .git/config")
	require.Equal(t, beforeWorktreeConfig, afterWorktreeConfig, "fixture must not rewrite primary .git/config.worktree")
	require.Equal(t, beforeValues, fixtureConfigValues(t, primary), "fixture must preserve primary core/user config values")

	_, primaryStatusErr := runGitIntegOutput(primary, "status", "--short")
	require.NoError(t, primaryStatusErr, "primary checkout must remain usable")
	_, linkedStatusErr := runGitIntegOutput(linked, "status", "--short")
	require.NoError(t, linkedStatusErr, "linked worktree must remain usable")
}

func fixtureConfigValues(t *testing.T, repo string) map[string]string {
	t.Helper()
	values := make(map[string]string)
	for _, key := range []string{"core.bare", "core.worktree", "user.name", "user.email"} {
		out, err := runGitIntegOutput(repo, "config", "--get", key)
		if err == nil {
			values[key] = out
		}
	}
	return values
}

func runCoreBareRepairFixture(t *testing.T) {
	t.Helper()
	fixture, _ := newScriptHarnessRepo(t, 0)
	runGitInteg(t, fixture, "config", "core.bare", "true")
	require.Equal(t, "true", runGitInteg(t, fixture, "config", "--local", "--get", "core.bare"))

	repair := gitrepohealth.RepairKnownConfigCorruption(context.Background(), fixture)
	require.True(t, repair.StatusSucceeded, "fixture corruption repair must leave fixture usable: %+v", repair)
	_, err := runGitIntegOutput(fixture, "status", "--short")
	require.NoError(t, err, "fixture must observe its own repaired worktree")
	_, err = runGitIntegOutput(fixture, "config", "--local", "--get", "core.bare")
	require.Error(t, err, "fixture core.bare must be removed by repair")
}
