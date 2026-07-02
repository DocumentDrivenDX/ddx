// Package testutils provides shared helpers for tests and acceptance demos
// that need a clean ddx-initialized git repo without polluting the main DDx
// project. The canonical entry point is NewFixtureRepo, which wraps
// scripts/build-fixture-repo.sh and auto-cleans via t.Cleanup.
package testutils

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// fixtureRepoBaseName is the leaf directory name given to every fixture
// project. It must satisfy the bead-identifier charset (^[a-zA-Z0-9-]+$,
// see internal/bead/id.go), because the bead prefix is derived from the git
// root's basename — a t.TempDir() leaf such as "001" would be numeric-only but
// the surrounding test-name path can contain characters (e.g. "_") that are
// rejected by `ddx bead create`. Nesting a fixed safe name under t.TempDir()
// guarantees a valid prefix regardless of the test name.
const fixtureRepoBaseName = "ddxfixture"

// NewFixtureRepo builds a clean ddx-initialized git repo for the given profile
// (minimal, standard, multi-project, federated) and returns its path. The repo
// is created under t.TempDir() so it is removed automatically when the test
// finishes (the auto-clean promised in scripts/build-fixture-repo.md).
//
// For minimal/standard the returned path is the project root; for
// multi-project/federated it is the parent dir whose sub-projects live
// underneath (proj-a, proj-b / hub, spoke).
//
// The ddx binary used for seeding is resolved via DDxBinary. Global and system
// git config are neutralized for the seeding subprocess so a developer box's
// commit.gpgsign (which points at a host-specific signer) does not break the
// fixture's own commits; repo-local commit.gpgsign is additionally disabled on
// each created project so subsequent ddx commits in the test never attempt to
// sign.
func NewFixtureRepo(t *testing.T, profile string) string {
	t.Helper()

	bin := DDxBinary(t)
	dest := filepath.Join(t.TempDir(), fixtureRepoBaseName)
	script := filepath.Join(repoRoot(t), "scripts", "build-fixture-repo.sh")

	emptyGlobalCfg := filepath.Join(t.TempDir(), "gitconfig-global")
	if err := os.WriteFile(emptyGlobalCfg, nil, 0o644); err != nil {
		t.Fatalf("write neutral global git config: %v", err)
	}

	cmd := exec.Command("bash", script, dest, "--profile", profile)
	cmd.Env = append(os.Environ(),
		"DDX_BIN="+bin,
		"GIT_CONFIG_GLOBAL="+emptyGlobalCfg,
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-fixture-repo.sh failed (profile=%s): %v\n%s", profile, err, out)
	}

	for _, dir := range fixtureProjectDirs(dest, profile) {
		gc := exec.Command("git", "-C", dir, "config", "commit.gpgsign", "false")
		gc.Env = append(os.Environ(), "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := gc.CombinedOutput(); err != nil {
			t.Fatalf("disable commit.gpgsign in %s: %v\n%s", dir, err, out)
		}
	}

	return dest
}

// fixtureProjectDirs returns the per-project git roots created for a profile,
// so repo-local config can be applied to each.
func fixtureProjectDirs(dest, profile string) []string {
	switch profile {
	case "multi-project":
		return []string{filepath.Join(dest, "proj-a"), filepath.Join(dest, "proj-b")}
	case "federated":
		return []string{filepath.Join(dest, "hub"), filepath.Join(dest, "spoke")}
	default:
		return []string{dest}
	}
}

var (
	builtBinaryOnce sync.Once
	builtBinaryPath string
	builtBinaryErr  error
)

// DDxBinary resolves a ddx binary for fixture seeding and subprocess tests. It
// prefers $DDX_BIN, then `ddx` on PATH, and otherwise lazily builds one from
// cli/ once per process. The built binary lives in an os.MkdirTemp dir that
// outlives any single test so concurrent fixtures can share it.
func DDxBinary(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("DDX_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	if p, err := exec.LookPath("ddx"); err == nil {
		return p
	}
	return BuildDDxBinary(t)
}

// BuildDDxBinary builds the ddx binary from the current cli/ source tree once
// per process and returns its path. Tests that must exercise the current source
// (rather than a possibly-stale `ddx` on PATH) should build with this and pin it
// via t.Setenv("DDX_BIN", ...) so NewFixtureRepo seeds with the same binary. The
// binary lives in an os.MkdirTemp dir that outlives any single test.
func BuildDDxBinary(t *testing.T) string {
	t.Helper()
	builtBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ddx-fixture-bin-*")
		if err != nil {
			builtBinaryErr = err
			return
		}
		out := filepath.Join(dir, "ddx")
		build := exec.Command("go", "build", "-buildvcs=false", "-o", out, ".")
		build.Dir = cliDir(t)
		if combined, err := build.CombinedOutput(); err != nil {
			builtBinaryErr = err
			t.Logf("go build ddx from source failed:\n%s", combined)
			return
		}
		builtBinaryPath = out
	})
	if builtBinaryErr != nil {
		t.Fatalf("build ddx binary from source: %v", builtBinaryErr)
	}
	return builtBinaryPath
}

// cliDir walks up from the test's working directory to the directory that
// holds go.mod (the cli/ module root).
func cliDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod (cli module root) from %s", wd)
		}
		dir = parent
	}
}

// repoRoot returns the repository root (parent of the cli/ module dir), where
// scripts/ lives.
func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(cliDir(t))
}
