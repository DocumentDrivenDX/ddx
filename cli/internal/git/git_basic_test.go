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

// TestGetCurrentBranch tests getting the current branch name
func TestGetCurrentBranch_Basic(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Git operations require working in the repository directory
	require.NoError(t, os.Chdir(repoDir))

	// Check default branch (master or main depending on git version)
	branch, err := GetCurrentBranch()
	assert.NoError(t, err)
	assert.Contains(t, []string{"master", "main"}, branch)

	// Create and switch to a new branch
	cmd := exec.Command("git", "checkout", "-b", "feature-test")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to create branch: %s", string(output))

	branch, err = GetCurrentBranch()
	assert.NoError(t, err)
	assert.Equal(t, "feature-test", branch)
}

// TestHasUncommittedChanges tests checking for uncommitted changes
func TestHasUncommittedChanges_Basic(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Git operations require working in the repository directory
	require.NoError(t, os.Chdir(repoDir))

	// Clean repository
	hasChanges, err := HasUncommittedChanges(".")
	assert.NoError(t, err)
	assert.False(t, hasChanges)

	// Add a new file
	require.NoError(t, os.WriteFile("new.txt", []byte("new content"), 0644))

	hasChanges, err = HasUncommittedChanges(".")
	assert.NoError(t, err)
	assert.True(t, hasChanges)

	// Stage the file
	cmd := exec.Command("git", "add", "new.txt")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	// Should still have uncommitted changes (staged but not committed)
	hasChanges, err = HasUncommittedChanges(".")
	assert.NoError(t, err)
	assert.True(t, hasChanges)

	// Commit the changes
	cmd = exec.Command("git", "commit", "-m", "Add new file")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	// Now should be clean
	hasChanges, err = HasUncommittedChanges(".")
	assert.NoError(t, err)
	assert.False(t, hasChanges)
}

// TestCommitChanges tests committing changes
func TestCommitChanges_Basic(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Git operations require working in the repository directory
	require.NoError(t, os.Chdir(repoDir))

	// Create a new file
	require.NoError(t, os.WriteFile("test.txt", []byte("test content"), 0644))

	// Commit the changes
	err := CommitChanges("Test commit")
	assert.NoError(t, err)

	// Verify the file was committed
	hasChanges, err := HasUncommittedChanges(".")
	assert.NoError(t, err)
	assert.False(t, hasChanges)

	// Verify commit message
	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "Test commit")
}

// TestCommitChanges_EdgeCases tests edge cases for CommitChanges
func TestCommitChanges_EdgeCases(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Git operations require working in the repository directory
	require.NoError(t, os.Chdir(repoDir))

	// Test empty commit message
	err := CommitChanges("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "commit message cannot be empty")

	// Test no changes to commit
	err = CommitChanges("Should fail")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no changes to commit")

	// Test from non-git directory
	nonGitDir := t.TempDir()
	require.NoError(t, os.Chdir(nonGitDir))
	err = CommitChanges("Should fail")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

// TestHasUncommittedChanges_EdgeCases tests edge cases for HasUncommittedChanges
func TestHasUncommittedChanges_EdgeCases(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Git operations require working in the repository directory
	require.NoError(t, os.Chdir(repoDir))

	// Test empty path (should default to current directory)
	hasChanges, err := HasUncommittedChanges("")
	assert.NoError(t, err)
	assert.False(t, hasChanges)

	// Test from non-git directory
	nonGitDir := t.TempDir()
	hasChanges, err = HasUncommittedChanges(nonGitDir)
	assert.Error(t, err)
	// The error message could be "invalid path" or "not a git repository"
	assert.True(t, strings.Contains(err.Error(), "not a git repository") || strings.Contains(err.Error(), "invalid path"))
	assert.False(t, hasChanges)
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
}

// TestGetCurrentBranch_EdgeCases tests edge cases for GetCurrentBranch
func TestGetCurrentBranch_EdgeCases(t *testing.T) {
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()

	// Test from non-git directory
	nonGitDir := t.TempDir()
	require.NoError(t, os.Chdir(nonGitDir))

	branch, err := GetCurrentBranch()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
	assert.Equal(t, "", branch)
}
