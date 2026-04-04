package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/easel/ddx/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

var (
	testLibraryPath string
	testLibraryOnce sync.Once
)

// GetTestLibraryPath returns the absolute path to a bare git repository
// that mimics a real upstream library (like GitHub). This is the ONLY way
// tests should get the test library path.
func GetTestLibraryPath() string {
	testLibraryOnce.Do(func() {
		// Find the test fixtures directory
		_, filename, _, _ := runtime.Caller(0)
		cmdDir := filepath.Dir(filename)
		fixtureDir := filepath.Join(cmdDir, "..", "test", "fixtures", "ddx-library")
		fixtureDir, _ = filepath.Abs(fixtureDir)

		// Determine base temp directory: TMP -> TMPDIR -> /tmp
		tempBase := "/tmp"
		if tmp := os.Getenv("TMP"); tmp != "" {
			tempBase = tmp
		} else if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
			tempBase = tmpdir
		}

		workingRepo := filepath.Join(tempBase, ".test-library")
		bareRepo := filepath.Join(tempBase, ".test-library.git")
		testLibraryPath = bareRepo

		// Check if we need to recreate (fixtures changed or doesn't exist)
		needsRecreate := false
		if stat, err := os.Stat(bareRepo); err != nil || !stat.IsDir() {
			needsRecreate = true
		} else if os.Getenv("CI") == "" {
			// In local dev, check if any fixture is newer than the repo
			repoModTime := stat.ModTime()
			filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() && info.ModTime().After(repoModTime) {
					needsRecreate = true
				}
				return nil
			})
		}

		if needsRecreate {
			// Clean up old repos
			os.RemoveAll(workingRepo)
			os.RemoveAll(bareRepo)

			// Create working repo and copy fixtures
			if err := os.MkdirAll(workingRepo, 0755); err != nil {
				panic(fmt.Sprintf("Failed to create working repo: %v", err))
			}

			if err := syncDirectory(fixtureDir, workingRepo); err != nil {
				panic(fmt.Sprintf("Failed to copy fixtures: %v", err))
			}

			// Initialize git repo
			cmds := []struct {
				args []string
				dir  string
			}{
				{[]string{"git", "init", "-b", "master"}, workingRepo},
				{[]string{"git", "config", "user.email", "test@example.com"}, workingRepo},
				{[]string{"git", "config", "user.name", "Test User"}, workingRepo},
				{[]string{"git", "add", "."}, workingRepo},
				{[]string{"git", "commit", "--allow-empty", "-m", "Test fixture"}, workingRepo},
				{[]string{"git", "init", "--bare", bareRepo}, ""},
				{[]string{"git", "remote", "add", "origin", bareRepo}, workingRepo},
				{[]string{"git", "push", "origin", "master"}, workingRepo},
			}

			for _, c := range cmds {
				cmd := exec.Command(c.args[0], c.args[1:]...)
				if c.dir != "" {
					cmd.Dir = c.dir
				}
				if output, err := cmd.CombinedOutput(); err != nil {
					panic(fmt.Sprintf("Failed to run %v: %v\nOutput: %s", c.args, err, output))
				}
			}
		}
	})
	return testLibraryPath
}

// syncDirectory copies files from src to dst, preserving directory structure
func syncDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directories from source
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = srcFile.Close() }()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer func() { _ = dstFile.Close() }()

		if _, err := dstFile.ReadFrom(srcFile); err != nil {
			return err
		}

		return os.Chmod(dstPath, info.Mode())
	})
}

// TestEnvironment provides isolated testing environment for .ddx/config.yaml
type TestEnvironment struct {
	Dir            string
	ConfigPath     string
	LibraryPath    string
	TestLibraryURL string
	GitInitialized bool

	// Optional customizations
	Platform     string
	Architecture string
	HomeDir      string

	t *testing.T
}

// TestEnvOption is a functional option for configuring TestEnvironment
type TestEnvOption func(*TestEnvironment)

// WithGitInit controls whether to initialize a git repository
func WithGitInit(init bool) TestEnvOption {
	return func(te *TestEnvironment) {
		te.GitInitialized = init
	}
}

// WithPlatform sets the platform and architecture for installation tests
func WithPlatform(platform, arch string) TestEnvOption {
	return func(te *TestEnvironment) {
		te.Platform = platform
		te.Architecture = arch
	}
}

// WithHomeDir sets a custom home directory
func WithHomeDir(homeDir string) TestEnvOption {
	return func(te *TestEnvironment) {
		te.HomeDir = homeDir
	}
}

// WithCustomLibraryURL sets a custom library repository URL
func WithCustomLibraryURL(url string) TestEnvOption {
	return func(te *TestEnvironment) {
		te.TestLibraryURL = url
	}
}

// NewTestEnvironment creates a clean test environment with temp directory
// By default: creates git repo, uses test-library fixture via file:// URL
func NewTestEnvironment(t *testing.T, opts ...TestEnvOption) *TestEnvironment {
	t.Helper()

	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	configPath := filepath.Join(ddxDir, "config.yaml")

	te := &TestEnvironment{
		Dir:            tempDir,
		ConfigPath:     configPath,
		LibraryPath:    filepath.Join(ddxDir, "library"),
		GitInitialized: true, // default: init git
		t:              t,
	}

	// Apply options
	for _, opt := range opts {
		opt(te)
	}

	// Set default test library URL if not customized
	if te.TestLibraryURL == "" {
		te.TestLibraryURL = "file://" + GetTestLibraryPath()
	}

	// Create .ddx directory
	require.NoError(t, os.MkdirAll(ddxDir, 0755))

	// Initialize git repository if requested
	if te.GitInitialized {
		te.initGit()
	}

	return te
}

// initGit initializes a git repository in the test directory
func (te *TestEnvironment) initGit() {
	te.t.Helper()

	// git init
	gitInit := exec.Command("git", "init")
	gitInit.Dir = te.Dir
	require.NoError(te.t, gitInit.Run(), "git init should succeed")

	// git config user.email
	gitEmail := exec.Command("git", "config", "user.email", "test@example.com")
	gitEmail.Dir = te.Dir
	require.NoError(te.t, gitEmail.Run(), "git config user.email should succeed")

	// git config user.name
	gitName := exec.Command("git", "config", "user.name", "Test User")
	gitName.Dir = te.Dir
	require.NoError(te.t, gitName.Run(), "git config user.name should succeed")
}

// CreateConfig creates a config file with the given content
func (te *TestEnvironment) CreateConfig(content string) {
	te.t.Helper()
	require.NoError(te.t, os.WriteFile(te.ConfigPath, []byte(content), 0644))
}

// CreateDefaultConfig creates a minimal valid config file using test library
func (te *TestEnvironment) CreateDefaultConfig() {
	te.t.Helper()
	content := fmt.Sprintf(`version: "1.0"
library:
  path: .ddx/library
  repository:
    url: %s
    branch: master
persona_bindings: {}
`, te.TestLibraryURL)
	te.CreateConfig(content)
}

// CreateConfigWithCustomURL creates a config with a custom repository URL
func (te *TestEnvironment) CreateConfigWithCustomURL(url string) {
	te.t.Helper()
	content := fmt.Sprintf(`version: "1.0"
library:
  path: .ddx/library
  repository:
    url: %s
    branch: master
persona_bindings: {}
`, url)
	te.CreateConfig(content)
}

// RunCommand runs a DDx command in the test environment and returns output
func (te *TestEnvironment) RunCommand(args ...string) (string, error) {
	te.t.Helper()
	factory := NewCommandFactory(te.Dir)
	cmd := factory.NewRootCommand()
	cmd.SetArgs(args)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	return buf.String(), err
}

// InitWithDDx properly initializes DDx in the test environment using ddx init
// If git is initialized, creates an initial commit before running ddx init.
// Otherwise uses --no-git flag. In CI environments, always uses --no-git.
func (te *TestEnvironment) InitWithDDx(flags ...string) {
	te.t.Helper()

	// Check if we should skip git operations
	hasNoGitFlag := false
	for _, flag := range flags {
		if flag == "--no-git" {
			hasNoGitFlag = true
			break
		}
	}

	// If git is initialized and we're not using --no-git, create initial commit
	if te.GitInitialized && !hasNoGitFlag {
		te.CreateFile("README.md", "# Test Project")
		gitAdd := exec.Command("git", "add", ".")
		gitAdd.Dir = te.Dir
		require.NoError(te.t, gitAdd.Run())
		gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
		gitCommit.Dir = te.Dir
		require.NoError(te.t, gitCommit.Run())
	}

	// Default flags if none provided
	if len(flags) == 0 {
		if te.GitInitialized {
			// Use --repository and --branch flags to specify test library
			flags = []string{"--repository", te.TestLibraryURL, "--branch", "master", "--silent", "--skip-claude-injection"}
		} else {
			// No git, use --no-git flag
			flags = []string{"--no-git", "--silent", "--skip-claude-injection"}
		}
	}

	args := append([]string{"init"}, flags...)
	output, err := te.RunCommand(args...)
	require.NoError(te.t, err, "init should succeed: %s", output)
}

// LoadConfig loads the config using ConfigLoader
func (te *TestEnvironment) LoadConfig() (*config.Config, error) {
	loader, err := config.NewConfigLoaderWithWorkingDir(te.Dir)
	if err != nil {
		return nil, err
	}
	return loader.LoadConfig()
}

// CreateFile creates any file in the test environment
func (te *TestEnvironment) CreateFile(relativePath, content string) {
	te.t.Helper()
	fullPath := filepath.Join(te.Dir, relativePath)
	dir := filepath.Dir(fullPath)
	require.NoError(te.t, os.MkdirAll(dir, 0755))
	require.NoError(te.t, os.WriteFile(fullPath, []byte(content), 0644))
}

// NewTestRootCommand creates a fresh root command for tests using isolated temp directory
// This is the preferred way to create test commands - it ensures test isolation.
func NewTestRootCommand(t *testing.T) *CommandFactory {
	t.Helper()
	tempDir := t.TempDir()
	return NewCommandFactory(tempDir)
}

// NewTestRootCommandWithDir creates a test command with a specific working directory
// Use this when your test needs to operate in a specific directory.
func NewTestRootCommandWithDir(dir string) *CommandFactory {
	return NewCommandFactory(dir)
}

// executeCommand is a helper to execute commands with captured output (legacy compatibility)
// New tests should use TestEnvironment.RunCommand() instead
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}
