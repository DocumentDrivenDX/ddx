package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initScriptTestRepo creates a temp dir, initialises a git repo with a
// scrubbed environment, and returns the path. The repo has an initial
// commit so that subsequent `git commit` calls succeed without errors.
func initScriptTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = scrubbedGitEnvScript()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Seed an initial commit so HEAD exists.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = scrubbedGitEnvScript()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// gitLogCount returns the number of commits reachable from HEAD.
func gitLogCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnvScript()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	lines := 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(l) != "" {
			lines++
		}
	}
	return lines
}

// gitLastCommitMsg returns the message of the most recent commit.
func gitLastCommitMsg(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnvScript()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log -1: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitIsClean returns true if the working tree has no uncommitted changes.
func gitIsClean(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnvScript()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out)) == ""
}

// writeDirectives writes a directive string to a temp file and returns its path.
func writeDirectives(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "directives-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// runScript is a convenience wrapper that calls RunViaService with the script harness.
func runScript(t *testing.T, workDir, directivePath string, corr map[string]string) (*Result, error) {
	t.Helper()
	return RunViaService(context.Background(), workDir, RunOptions{
		Harness:     "script",
		WorkDir:     workDir,
		Model:       directivePath,
		Correlation: corr,
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestScriptHarness_AppendLineAndCommit(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
append-line tests/smoke.txt hello
commit test
`)
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	content, err := os.ReadFile(filepath.Join(repo, "tests/smoke.txt"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", string(content))
	}
	if gitLogCount(t, repo) != 2 { // init + test
		t.Fatalf("expected 2 commits, got %d", gitLogCount(t, repo))
	}
	if msg := gitLastCommitMsg(t, repo); msg != "test" {
		t.Fatalf("expected commit message 'test', got %q", msg)
	}
	if !gitIsClean(t, repo) {
		t.Fatal("expected clean working tree")
	}
}

func TestScriptHarness_NoOp(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, "no-op\n")
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if gitLogCount(t, repo) != 1 {
		t.Fatal("expected no new commits")
	}
	if !gitIsClean(t, repo) {
		t.Fatal("expected clean working tree")
	}
}

func TestScriptHarness_CreateFileAndCommit(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
create-file foo.txt bar
commit create
`)
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	content, err := os.ReadFile(filepath.Join(repo, "foo.txt"))
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(content) != "bar" {
		t.Fatalf("expected 'bar', got %q", string(content))
	}
	if gitLogCount(t, repo) != 2 {
		t.Fatalf("expected 2 commits, got %d", gitLogCount(t, repo))
	}
}

func TestScriptHarness_SetExit1(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
set-exit 1
no-op
`)
	// set-exit does NOT stop execution and does NOT return an error — it just
	// sets the final exit code. RunScript returns nil error, ExitCode == 1.
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
}

func TestScriptHarness_FailDuring(t *testing.T) {
	repo := initScriptTestRepo(t)
	// Directive index 0: append-line a.txt x
	// Directive index 1: fail-during 1  (fails at index 1 — i.e. itself)
	// Directive index 2: commit skip     (never reached)
	df := writeDirectives(t, `
append-line a.txt x
fail-during 1
commit skip
`)
	_, err := runScript(t, repo, df, nil)
	if err == nil {
		t.Fatal("expected error from fail-during")
	}
	// a.txt should exist (directive 0 ran)
	content, ferr := os.ReadFile(filepath.Join(repo, "a.txt"))
	if ferr != nil {
		t.Fatalf("a.txt not found: %v", ferr)
	}
	if string(content) != "x\n" {
		t.Fatalf("expected 'x\\n', got %q", string(content))
	}
	// No commit should have been made (the commit directive was skipped)
	if gitLogCount(t, repo) != 1 {
		t.Fatal("expected no new commit")
	}
}

func TestScriptHarness_EnvVarInterpolation(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
append-line out.txt hello ${DDX_BEAD_ID}
commit test
`)
	corr := map[string]string{"bead_id": "ddx-abc12345"}
	result, err := runScript(t, repo, df, corr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	content, ferr := os.ReadFile(filepath.Join(repo, "out.txt"))
	if ferr != nil {
		t.Fatalf("out.txt not found: %v", ferr)
	}
	if string(content) != "hello ddx-abc12345\n" {
		t.Fatalf("expected 'hello ddx-abc12345\\n', got %q", string(content))
	}
}

func TestScriptHarness_AbsolutePathRejected(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `append-line /etc/evil x`)
	_, err := runScript(t, repo, df, nil)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute path rejected") {
		t.Fatalf("expected 'absolute path rejected' in error, got: %v", err)
	}
}

func TestScriptHarness_ModifyLine(t *testing.T) {
	repo := initScriptTestRepo(t)
	// Seed file before running directives
	if err := os.WriteFile(filepath.Join(repo, "config.txt"), []byte("foo=old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	df := writeDirectives(t, `
modify-line config.txt foo=.* foo=new
commit bump
`)
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	content, ferr := os.ReadFile(filepath.Join(repo, "config.txt"))
	if ferr != nil {
		t.Fatalf("config.txt not found: %v", ferr)
	}
	if !strings.Contains(string(content), "foo=new") {
		t.Fatalf("expected 'foo=new' in config.txt, got %q", string(content))
	}
}

func TestScriptHarness_RunShellCommand(t *testing.T) {
	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
run echo hello > x.txt
commit run
`)
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	content, ferr := os.ReadFile(filepath.Join(repo, "x.txt"))
	if ferr != nil {
		t.Fatalf("x.txt not found: %v", ferr)
	}
	if strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("expected 'hello' in x.txt, got %q", string(content))
	}
	if gitLogCount(t, repo) != 2 {
		t.Fatalf("expected 2 commits, got %d", gitLogCount(t, repo))
	}
}

func TestScriptHarness_GitEnvScrubbed(t *testing.T) {
	// Inject a bogus GIT_DIR to verify the harness scrubs it before git ops.
	t.Setenv("GIT_DIR", "/bogus/path/that/does/not/exist")

	repo := initScriptTestRepo(t)
	df := writeDirectives(t, `
create-file hello.txt world
commit scrubbed
`)
	result, err := runScript(t, repo, df, nil)
	if err != nil {
		t.Fatalf("unexpected error (GIT_DIR not scrubbed?): %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if gitLogCount(t, repo) != 2 {
		t.Fatalf("expected 2 commits, got %d", gitLogCount(t, repo))
	}
}
