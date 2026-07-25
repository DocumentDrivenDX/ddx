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
	// Fail closed during setup if the suite cannot prove a private config scope
	// before any intentional mutation. Then install an invoking-repo guard that
	// receives hostile GIT_* selection so leaks cannot rewrite a shared checkout.
	invoking := newInvokingRepoConfigGuard(t)

	// Dedicated temporary fixture repository — never the process cwd / host repo.
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"
	runGitInteg(t, projectRoot, "config", "extensions.worktreeConfig", "true")

	fixtureConfigPath := filepath.Join(projectRoot, ".git", "config")

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

	// Fixture local config must not retain the harness contamination keys either.
	afterFixtureConfig, err := os.ReadFile(fixtureConfigPath)
	require.NoError(t, err)
	require.NotContains(t, string(afterFixtureConfig), "fixture@ddx.test")
	require.NotContains(t, string(afterFixtureConfig), "nested@ddx.test")
	require.NotContains(t, string(afterFixtureConfig), "DDxFixture")

	// Invoking stand-in common config must be byte-identical (no core.bare /
	// core.worktree / user.name / user.email leak from the fixture path).
	invoking.assertUnchanged(t)
}

// TestAgentGitConfigEnvSanitizesRepositorySelection proves the shared fixture
// git helper (fixtureGitEnvInteg / fixtureGitCommand) clears inherited
// GIT_DIR, GIT_WORK_TREE, and GIT_INDEX_FILE before any fixture git config
// write, while still binding commands to the intended temporary repository
// via cmd.Dir.
func TestAgentGitConfigEnvSanitizesRepositorySelection(t *testing.T) {
	// Outer checkout: stands in for a hook-contaminated primary repository.
	outer := filepath.Join(t.TempDir(), "outer")
	runGitInteg(t, filepath.Dir(outer), "init", "-b", "main", outer)
	runGitInteg(t, outer, "config", "user.name", "Outer Checkout")
	runGitInteg(t, outer, "config", "user.email", "outer@ddx.test")
	require.NoError(t, os.WriteFile(filepath.Join(outer, "seed.txt"), []byte("outer\n"), 0o644))
	runGitInteg(t, outer, "add", "seed.txt")
	runGitInteg(t, outer, "commit", "-m", "chore: seed outer")

	// Fixture repository that must receive config writes.
	fixture := filepath.Join(t.TempDir(), "fixture")
	runGitInteg(t, filepath.Dir(fixture), "init", "-b", "main", fixture)

	// Contaminate the process environment the way lefthook does.
	t.Setenv("GIT_DIR", filepath.Join(outer, ".git"))
	t.Setenv("GIT_WORK_TREE", outer)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(outer, ".git", "index"))

	// The constructed command must not carry repository-selection vars, and
	// must still resolve to the fixture (cmd.Dir binding after the scrub).
	cmd := fixtureGitCommand(t, fixture, "rev-parse", "--show-toplevel")
	for _, kv := range cmd.Env {
		for _, banned := range []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE="} {
			if strings.HasPrefix(kv, banned) {
				t.Fatalf("fixture git env leaked repository-selection var %q", kv)
			}
		}
	}
	// Private config scope must be re-bound after CleanEnv strips GIT_CONFIG_*.
	gotPrivate := false
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "GIT_CONFIG_GLOBAL=") {
			gotPrivate = true
			require.Equal(t, "GIT_CONFIG_GLOBAL="+testFixtureGitConfigPath, kv)
		}
	}
	require.True(t, gotPrivate, "fixture helper must bind GIT_CONFIG_GLOBAL to the TestMain private config")

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "fixture rev-parse under contaminated env: %s", out)
	gotTop, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	require.NoError(t, err)
	wantTop, err := filepath.EvalSymlinks(fixture)
	require.NoError(t, err)
	require.Equal(t, wantTop, gotTop, "helper must bind git to fixture dir, not inherited GIT_DIR")

	// A real config write must land in the fixture, not the outer checkout.
	runGitInteg(t, fixture, "config", "user.email", "sanitized-fixture@ddx.test")
	email := runGitInteg(t, fixture, "config", "--local", "--get", "user.email")
	require.Equal(t, "sanitized-fixture@ddx.test", email)

	outerConfig, err := os.ReadFile(filepath.Join(outer, ".git", "config"))
	require.NoError(t, err)
	require.NotContains(t, string(outerConfig), "sanitized-fixture@ddx.test",
		"config write under contaminated env must not touch outer checkout")
}

// TestAgentGitConfigHelperUsesPrivateConfig proves core.bare, core.worktree,
// user.name, and user.email writes through the fixture helper land only in the
// temporary fixture repository/config scope and not in the invoking checkout's
// common config.
func TestAgentGitConfigHelperUsesPrivateConfig(t *testing.T) {
	// Invoking checkout with a linked worktree (common-dir selection surface).
	primary := filepath.Join(t.TempDir(), "primary")
	runGitInteg(t, filepath.Dir(primary), "init", "-b", "main", primary)
	runGitInteg(t, primary, "config", "user.name", "Primary Guard")
	runGitInteg(t, primary, "config", "user.email", "primary-guard@ddx.test")
	require.NoError(t, os.WriteFile(filepath.Join(primary, "seed.txt"), []byte("seed\n"), 0o644))
	runGitInteg(t, primary, "add", "seed.txt")
	runGitInteg(t, primary, "commit", "-m", "chore: seed primary")
	runGitInteg(t, primary, "config", "extensions.worktreeConfig", "true")

	linked := filepath.Join(t.TempDir(), "linked")
	runGitInteg(t, primary, "worktree", "add", "-b", "private-config-linked", linked, "HEAD")
	t.Cleanup(func() { _ = runGitInteg(t, primary, "worktree", "remove", "--force", linked) })

	primaryConfigPath := filepath.Join(primary, ".git", "config")
	beforePrimary, err := os.ReadFile(primaryConfigPath)
	require.NoError(t, err)
	beforeValues := fixtureConfigValues(t, primary)

	// Contaminate selection to the shared common gitdir of the primary checkout.
	t.Setenv("GIT_DIR", filepath.Join(primary, ".git"))
	t.Setenv("GIT_WORK_TREE", primary)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(primary, ".git", "index"))

	fixture, _ := newScriptHarnessRepo(t, 0)
	redirectedWorktree := filepath.Join(t.TempDir(), "redirected-worktree")
	// Dangerous keys that previously leaked into the shared checkout config.
	// Write identity first (no bare/worktree interaction), then the core keys.
	runGitInteg(t, fixture, "config", "user.name", "Fixture Private Name")
	runGitInteg(t, fixture, "config", "user.email", "fixture-private@ddx.test")
	runGitInteg(t, fixture, "config", "core.bare", "true")
	runGitInteg(t, fixture, "config", "core.worktree", redirectedWorktree)

	// Fixture local scope received the writes. Read the config file directly:
	// any further `git config --get` under CombinedOutput mixes a stderr
	// warning when both core.bare and core.worktree are set ("do not make sense").
	fixtureCfgPath := filepath.Join(fixture, ".git", "config")
	fixtureCfg, err := os.ReadFile(fixtureCfgPath)
	require.NoError(t, err)
	fixtureText := string(fixtureCfg)
	require.Contains(t, fixtureText, "bare = true")
	require.Contains(t, fixtureText, "worktree = "+redirectedWorktree)
	require.Contains(t, fixtureText, "name = Fixture Private Name")
	require.Contains(t, fixtureText, "email = fixture-private@ddx.test")

	// Invoking checkout common config must be byte-identical and retain values.
	afterPrimary, err := os.ReadFile(primaryConfigPath)
	require.NoError(t, err)
	require.Equal(t, beforePrimary, afterPrimary, "fixture helper must not rewrite primary common config")
	require.Equal(t, beforeValues, fixtureConfigValues(t, primary), "primary core/user config must be unchanged")

	// Linked worktree must remain usable (shared common dir not corrupted).
	_, statusErr := runGitIntegOutput(linked, "status", "--short")
	require.NoError(t, statusErr, "linked worktree must remain usable after fixture config writes")

	// Explicit content guard: private identity strings never appear in primary.
	for _, banned := range []string{
		"Fixture Private Name",
		"fixture-private@ddx.test",
		"redirected-worktree",
	} {
		require.NotContains(t, string(afterPrimary), banned,
			"primary common config must not contain fixture-private value %q", banned)
	}
}

// TestAgentGitConfigHelperFailsClosedWhenPrivateScopeUnavailable proves setup
// returns a deterministic error before mutation when the helper cannot establish
// a fixture-private config scope.
func TestAgentGitConfigHelperFailsClosedWhenPrivateScopeUnavailable(t *testing.T) {
	// Build an outer checkout that a non-fail-closed helper would mutate when
	// GIT_DIR is inherited and a config write is attempted.
	outer := filepath.Join(t.TempDir(), "outer")
	runGitInteg(t, filepath.Dir(outer), "init", "-b", "main", outer)
	runGitInteg(t, outer, "config", "user.name", "Outer FailClosed")
	runGitInteg(t, outer, "config", "user.email", "outer-failclosed@ddx.test")
	outerConfigPath := filepath.Join(outer, ".git", "config")
	beforeOuter, err := os.ReadFile(outerConfigPath)
	require.NoError(t, err)

	fixture := t.TempDir()
	// Contaminate selection toward outer so any accidental git spawn would hit it.
	t.Setenv("GIT_DIR", filepath.Join(outer, ".git"))
	t.Setenv("GIT_WORK_TREE", outer)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(outer, ".git", "index"))

	// Case 1: private config path unset — helper must refuse before spawning git.
	saved := testFixtureGitConfigPath
	t.Cleanup(func() { testFixtureGitConfigPath = saved })
	testFixtureGitConfigPath = ""

	env, envErr := fixtureGitEnvInteg()
	require.Error(t, envErr, "fixtureGitEnvInteg must fail closed when private path is unset")
	require.Nil(t, env)
	require.Contains(t, envErr.Error(), "test fixture private git config is not initialized")

	out, runErr := runGitIntegOutput(fixture, "config", "core.bare", "true")
	require.Error(t, runErr, "runGitIntegOutput must refuse without private config scope")
	require.Contains(t, runErr.Error(), "without private config")
	require.Empty(t, out)

	afterUnset, err := os.ReadFile(outerConfigPath)
	require.NoError(t, err)
	require.Equal(t, beforeOuter, afterUnset, "fail-closed path must not mutate outer config when path is unset")

	// Case 2: private config path points at a non-regular path (directory).
	badDir := t.TempDir()
	testFixtureGitConfigPath = badDir
	env, envErr = fixtureGitEnvInteg()
	require.Error(t, envErr, "fixtureGitEnvInteg must fail closed when private path is not a regular file")
	require.Nil(t, env)
	require.Contains(t, envErr.Error(), "not a regular file")

	out, runErr = runGitIntegOutput(fixture, "config", "user.email", "should-not-write@ddx.test")
	require.Error(t, runErr)
	require.Contains(t, runErr.Error(), "without private config")
	require.Empty(t, out)

	afterBad, err := os.ReadFile(outerConfigPath)
	require.NoError(t, err)
	require.Equal(t, beforeOuter, afterBad, "fail-closed path must not mutate outer config when private scope is unusable")
	require.NotContains(t, string(afterBad), "should-not-write@ddx.test")
	require.NotContains(t, string(afterBad), "bare = true")
}

// TestAgentGitConfigFixturesDoNotLeakToPrimaryLinkedWorktree proves the test
// fixture boundary itself, rather than only ExecuteBead's harness wrapper.
// The parent process deliberately points Git at a real primary repository with a
// linked worktree; runGitInteg/newScriptHarnessRepo must scrub that selection,
// keep primary common config and linked worktree config.worktree byte-identical,
// and make the fixture observe+repair only its own temporary repository.
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
	// Distinct worktree-scoped identity so linked config.worktree exists and is
	// distinguishable from the primary common config surface.
	runGitInteg(t, linked, "config", "--worktree", "user.name", "Linked Worktree Guard")
	runGitInteg(t, linked, "config", "--worktree", "user.email", "linked-fixture-guard@ddx.test")

	primaryConfig := filepath.Join(primary, ".git", "config")
	linkedGitDir := strings.TrimSpace(runGitInteg(t, linked, "rev-parse", "--absolute-git-dir"))
	linkedWorktreeConfig := filepath.Join(linkedGitDir, "config.worktree")
	beforePrimaryConfig, err := os.ReadFile(primaryConfig)
	require.NoError(t, err)
	beforeLinkedWorktreeConfig, err := os.ReadFile(linkedWorktreeConfig)
	require.NoError(t, err)
	beforePrimaryValues := fixtureConfigValues(t, primary)
	beforeLinkedValues := fixtureConfigValues(t, linked)

	// This is the failure mode observed on the shared checkout: a caller's Git
	// repository selection survives into an agent fixture. If a fixture helper
	// ever stops using fixtureGitEnvInteg, its `git config core.bare true` below
	// targets primary (and can corrupt shared common config / worktree selection)
	// and the byte-identity checks fail.
	t.Setenv("GIT_DIR", filepath.Join(primary, ".git"))
	t.Setenv("GIT_WORK_TREE", primary)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(primary, ".git", "index"))
	runCoreBareRepairFixture(t)
	runLandFixtureGitMutationPaths(t)

	afterPrimaryConfig, err := os.ReadFile(primaryConfig)
	require.NoError(t, err)
	afterLinkedWorktreeConfig, err := os.ReadFile(linkedWorktreeConfig)
	require.NoError(t, err)
	require.Equal(t, beforePrimaryConfig, afterPrimaryConfig, "fixture must not rewrite primary .git/config")
	require.Equal(t, beforeLinkedWorktreeConfig, afterLinkedWorktreeConfig,
		"fixture must not rewrite linked-worktree config.worktree")
	require.Equal(t, beforePrimaryValues, fixtureConfigValues(t, primary),
		"fixture must preserve primary core.bare/core.worktree/user.name/user.email")
	require.Equal(t, beforeLinkedValues, fixtureConfigValues(t, linked),
		"fixture must preserve linked-worktree core.bare/core.worktree/user.name/user.email")

	_, primaryStatusErr := runGitIntegOutput(primary, "status", "--short")
	require.NoError(t, primaryStatusErr, "primary checkout must remain usable")
	_, linkedStatusErr := runGitIntegOutput(linked, "status", "--short")
	require.NoError(t, linkedStatusErr, "linked worktree must remain usable")
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

// runLandFixtureGitMutationPaths exercises every landTestRepo helper that
// creates commits in a linked fixture worktree. The hostile parent Git
// selection is deliberately still installed by the caller.
func runLandFixtureGitMutationPaths(t *testing.T) {
	t.Helper()
	r := newLandTestRepo(t)
	created := r.commitOnFiles(r.baseSHA, "feat: fixture files", map[string]string{
		"delete-me.txt": "temporary\n",
		"keep-me.txt":   "retained\n",
	})
	r.commitDeleteOn(created, "delete-me.txt", "test: delete fixture file")
	r.commitExecuteBeadEvidence(r.baseSHA, &ExecuteBeadResult{
		AttemptID: "20260717T000000Z",
		// Use an unignored legacy path: this proof exercises the fixture Git
		// subprocesses, not the separate evidence-ignore policy.
		ExecutionDir: "fixture-evidence",
	}, nil)
}
