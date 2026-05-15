package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain scrubs GIT_* environment variables for the whole test process.
// When the test suite is invoked from inside a lefthook pre-commit hook,
// lefthook sets GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_AUTHOR_*,
// GIT_COMMITTER_*, etc. to paths inside the *parent* repository. Any git
// subprocess these tests spawn — whether via raw exec.Command or via the
// production code under test — would otherwise inherit those variables
// and mutate the parent repo's config (e.g. leaking a stray
// `worktree = /tmp/TestXxx/001` line into the shared .git/config), which
// then corrupts every subsequent git operation in the parent repo.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				_ = os.Unsetenv(kv[:idx])
			}
		}
	}
	os.Exit(m.Run())
}

// scrubbedGitEnv returns the current environment with all GIT_* variables
// removed. When tests run inside a lefthook pre-commit hook, lefthook sets
// GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_AUTHOR_*, GIT_COMMITTER_*, etc.
// to the parent repo's paths. A child `git init` in a temp dir would inherit
// those, making the child write to the PARENT repo's config (leaking a stray
// `worktree = /tmp/TestXxx/001` line into the shared .git/config) and
// corrupting the parent. Always use this helper for test-local git
// subprocesses to keep them isolated.
func scrubbedGitEnv() []string {
	parent := os.Environ()
	env := make([]string, 0, len(parent))
	for _, kv := range parent {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runGitInDir runs a git command in dir with scrubbed GIT_* env. Fails the
// test if the command returns a non-zero exit status.
func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// Helper function to create a test git repository
func setupTestGitRepo(t *testing.T) string {
	tempDir := t.TempDir()

	runGitInDir(t, tempDir, "init")
	runGitInDir(t, tempDir, "config", "user.email", "test@example.com")
	runGitInDir(t, tempDir, "config", "user.name", "Test User")

	// Create initial commit
	testFile := filepath.Join(tempDir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test Repo"), 0644))

	runGitInDir(t, tempDir, "add", ".")
	runGitInDir(t, tempDir, "commit", "-m", "Initial commit")

	return tempDir
}

// TestIsRepository tests checking if a directory is a git repository
func TestIsRepository_Basic(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "valid git repository",
			setup: func() string {
				return setupTestGitRepo(t)
			},
			expected: true,
		},
		{
			name: "non-git directory",
			setup: func() string {
				return t.TempDir()
			},
			expected: false,
		},
		{
			name: "non-existent directory",
			setup: func() string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			result := IsRepository(path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindProjectRoot tests git root resolution
func TestFindProjectRoot(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	t.Run("returns repo root from root dir", func(t *testing.T) {
		root := FindProjectRoot(repoDir)
		assert.Equal(t, repoDir, root)
	})

	t.Run("returns repo root from subdirectory", func(t *testing.T) {
		subDir := filepath.Join(repoDir, "sub", "deep")
		require.NoError(t, os.MkdirAll(subDir, 0755))
		root := FindProjectRoot(subDir)
		assert.Equal(t, repoDir, root)
	})

	t.Run("returns input for non-git directory", func(t *testing.T) {
		nonGit := t.TempDir()
		root := FindProjectRoot(nonGit)
		assert.Equal(t, nonGit, root)
	})

	t.Run("prefers physical root over core.worktree redirect", func(t *testing.T) {
		linked := filepath.Join(filepath.Dir(repoDir), "redirected-wt")
		runGitInDir(t, repoDir, "worktree", "add", "--detach", linked, "HEAD")
		runGitInDir(t, repoDir, "config", "core.worktree", linked)

		raw := exec.Command("git", "rev-parse", "--show-toplevel")
		raw.Dir = repoDir
		raw.Env = scrubbedGitEnv()
		rawOut, err := raw.Output()
		require.NoError(t, err)
		assert.Equal(t, linked, strings.TrimSpace(string(rawOut)))

		root := FindProjectRoot(repoDir)
		assert.Equal(t, repoDir, root)

		linkedRoot := FindProjectRoot(linked)
		assert.Equal(t, linked, linkedRoot)
	})
}

// TestFindNearestDDxWorkspace_LinkedWorktreePrefersPrimary verifies that
// FindNearestDDxWorkspace resolves to the PRIMARY worktree's .ddx/ when
// called from inside a linked worktree, even if the linked worktree itself
// has its own .ddx/ directory. This protects bead store mutations from
// silently landing in an ephemeral execution worktree (ddx-381f4171).
func TestFindNearestDDxWorkspace_LinkedWorktreePrefersPrimary(t *testing.T) {
	primary := setupTestGitRepo(t)

	// Create the primary's .ddx/ with a marker file so we can tell them apart.
	primaryDdx := filepath.Join(primary, ddxDirSegment)
	require.NoError(t, os.MkdirAll(primaryDdx, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(primaryDdx, "marker.txt"), []byte("primary"), 0644))

	// Add a linked worktree as a sibling of the primary.
	linked := filepath.Join(filepath.Dir(primary), "linked-wt")
	runGitInDir(t, primary, "worktree", "add", "-b", "linked-branch", linked)

	// Create the linked worktree's own .ddx/ — this is the trap. Without the
	// fix, FindNearestDDxWorkspace would return the linked dir.
	linkedDdx := filepath.Join(linked, ddxDirSegment)
	require.NoError(t, os.MkdirAll(linkedDdx, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(linkedDdx, "marker.txt"), []byte("linked"), 0644))

	// From inside the linked worktree, we must resolve to the PRIMARY.
	got := FindNearestDDxWorkspace(linked)
	assert.Equal(t, primary, got, "expected primary worktree root, got linked worktree")

	// From a subdir of the linked worktree, same result.
	subdir := filepath.Join(linked, "some", "deep", "dir")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	got = FindNearestDDxWorkspace(subdir)
	assert.Equal(t, primary, got, "expected primary worktree root from subdir of linked worktree")

	// From inside the primary, resolve to the primary as before.
	got = FindNearestDDxWorkspace(primary)
	assert.Equal(t, primary, got)
}
