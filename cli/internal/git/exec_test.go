package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommand_StripsLocalEnvVarsHonorsDir verifies that Command()
// produces a *exec.Cmd which discovers the repo from cmd.Dir even when
// the parent process has GIT_DIR set to point somewhere else. This is
// the regression gate for ddx-aa8a5fb3 (Bug A: hook-inherited GIT_DIR
// causing git to mutate the wrong repo).
func TestCommand_StripsLocalEnvVarsHonorsDir(t *testing.T) {
	// Create a real repo at workDir; the test will assert that git ops
	// inside Command() resolve to workDir/.git, not the polluting
	// GIT_DIR set in the env.
	workDir := t.TempDir()
	if out, err := exec.Command("git", "-C", workDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Pollute parent env with a bogus GIT_DIR pointing at a different
	// (non-existent) repo. If the wrapper does its job, git ignores it.
	pollute := t.TempDir()
	bogusGitDir := filepath.Join(pollute, ".git", "bogus")
	t.Setenv("GIT_DIR", bogusGitDir)
	t.Setenv("GIT_WORK_TREE", pollute)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(pollute, "INDEX"))

	// And set a NON-local var the wrapper must preserve.
	t.Setenv("GIT_AUTHOR_NAME", "DDx Test")
	t.Setenv("GIT_SSH_COMMAND", "ssh -i /tmp/fake_key")

	ctx := context.Background()
	cmd := Command(ctx, workDir, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}

	gitDir := strings.TrimSpace(string(out))
	// Resolve workDir for symlinks (macOS /tmp → /private/tmp etc.)
	wantDir := filepath.Join(workDir, ".git")
	if absWant, err := filepath.EvalSymlinks(wantDir); err == nil {
		wantDir = absWant
	}
	if absGit, err := filepath.EvalSymlinks(gitDir); err == nil {
		gitDir = absGit
	}

	if gitDir != wantDir && gitDir != ".git" {
		t.Fatalf("git --git-dir = %q, want %q (or %q); inherited GIT_DIR was %q — wrapper failed to strip",
			gitDir, wantDir, ".git", bogusGitDir)
	}

	// Confirm preserved env: GIT_AUTHOR_NAME and GIT_SSH_COMMAND must
	// still be in cmd.Env (they're not local-env-vars).
	gotAuthor := false
	gotSSH := false
	for _, kv := range cmd.Env {
		if kv == "GIT_AUTHOR_NAME=DDx Test" {
			gotAuthor = true
		}
		if kv == "GIT_SSH_COMMAND=ssh -i /tmp/fake_key" {
			gotSSH = true
		}
	}
	if !gotAuthor {
		t.Error("GIT_AUTHOR_NAME was stripped; wrapper should preserve identity vars")
	}
	if !gotSSH {
		t.Error("GIT_SSH_COMMAND was stripped; wrapper should preserve transport vars")
	}

	// Confirm GIT_DIR / GIT_WORK_TREE / GIT_INDEX_FILE were stripped.
	for _, kv := range cmd.Env {
		for _, banned := range []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE="} {
			if strings.HasPrefix(kv, banned) {
				t.Errorf("local-env-var %q leaked into cmd.Env", kv)
			}
		}
	}
}

// TestCleanEnv_StripsLocalEnvVars exercises the helper directly.
func TestCleanEnv_StripsLocalEnvVars(t *testing.T) {
	t.Setenv("GIT_DIR", "/tmp/should-be-stripped")
	t.Setenv("GIT_OBJECT_DIRECTORY", "/tmp/objs")
	t.Setenv("GIT_AUTHOR_EMAIL", "preserve@me")
	t.Setenv("DDX_TEST_NON_GIT_VAR", "preserve@me")

	env := CleanEnv()
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_DIR=") {
			t.Errorf("GIT_DIR not stripped: %q", kv)
		}
		if strings.HasPrefix(kv, "GIT_OBJECT_DIRECTORY=") {
			t.Errorf("GIT_OBJECT_DIRECTORY not stripped: %q", kv)
		}
	}
	got := map[string]bool{}
	for _, kv := range env {
		got[kv] = true
	}
	if !got["GIT_AUTHOR_EMAIL=preserve@me"] {
		t.Error("GIT_AUTHOR_EMAIL incorrectly stripped — should be preserved (not in local-env-vars)")
	}
	if !got["DDX_TEST_NON_GIT_VAR=preserve@me"] {
		t.Error("non-git var incorrectly stripped")
	}
}

// TestGitFixtureHelpersStripHookEnv proves that fixture git config writes
// (user.email, user.name) land in the fixture repo even when GIT_DIR,
// GIT_WORK_TREE, and GIT_INDEX_FILE are contaminated to point at an outer
// repo — the scenario that caused core.bare=true and leaked fixture@ddx.test
// into the primary checkout during the ddx-b8bbd0a6 incident.
func TestGitFixtureHelpersStripHookEnv(t *testing.T) {
	ctx := context.Background()

	// outer repo: simulates the hook-context repo whose GIT_DIR is inherited.
	outerDir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", outerDir).CombinedOutput(); err != nil {
		t.Fatalf("git init outer: %v\n%s", err, out)
	}
	outerGitDir := filepath.Join(outerDir, ".git")

	// Contaminate the process env (t.Setenv restores after the test).
	t.Setenv("GIT_DIR", outerGitDir)
	t.Setenv("GIT_WORK_TREE", outerDir)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(outerGitDir, "index"))

	// fixture repo: should receive config writes, must not touch outer repo.
	fixtureDir := t.TempDir()

	// Use Command() — the wrapper that strips local env vars before spawning git.
	if out, err := Command(ctx, fixtureDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init fixture: %v\n%s", err, out)
	}
	if out, err := Command(ctx, fixtureDir, "config", "user.email", "fixture@ddx.test").CombinedOutput(); err != nil {
		t.Fatalf("git config email: %v\n%s", err, out)
	}
	if out, err := Command(ctx, fixtureDir, "config", "user.name", "DDx Fixture").CombinedOutput(); err != nil {
		t.Fatalf("git config name: %v\n%s", err, out)
	}

	// Assert fixture received the writes.
	emailOut, err := Command(ctx, fixtureDir, "config", "--get", "user.email").Output()
	if err != nil {
		t.Fatalf("read fixture user.email: %v", err)
	}
	if got := strings.TrimSpace(string(emailOut)); got != "fixture@ddx.test" {
		t.Errorf("fixture user.email = %q, want fixture@ddx.test", got)
	}

	// Assert outer repo config is clean — no fixture identity leaked.
	outerConfig, err := os.ReadFile(filepath.Join(outerGitDir, "config"))
	if err != nil {
		t.Fatalf("read outer .git/config: %v", err)
	}
	if strings.Contains(string(outerConfig), "fixture@ddx.test") {
		t.Errorf("fixture email leaked into outer repo config:\n%s", outerConfig)
	}
	if strings.Contains(string(outerConfig), "DDx Fixture") {
		t.Errorf("fixture name leaked into outer repo config:\n%s", outerConfig)
	}
}

// TestLocalEnvVars_IncludesEssentials guards the hardcoded fallback set
// against accidental edits.
func TestLocalEnvVars_IncludesEssentials(t *testing.T) {
	vars := localEnvVars()
	for _, must := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"} {
		if !vars[must] {
			t.Errorf("local-env-vars set missing essential %q", must)
		}
	}
	// Must NOT include vars git considers global / preserve-on-spawn.
	for _, mustNot := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_SSH_COMMAND", "GIT_TRACE"} {
		if vars[mustNot] {
			t.Errorf("local-env-vars set should NOT include %q (would break ssh/identity/tracing)", mustNot)
		}
	}

	// Sanity: the placeholder os.Environ() reference shows we're loading
	// vars at runtime (not a compile-time constant of one).
	_ = os.Environ()
}
