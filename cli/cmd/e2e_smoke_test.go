package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getSmokeTestBinaryPath builds the CLI binary once and returns the path.
func getSmokeTestBinaryPath(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "could not get caller info")

	// e2e_smoke_test.go is in cli/cmd/, so go up one level to get cli/
	cliRoot := filepath.Join(filepath.Dir(filename), "..")
	cliRoot, err := filepath.Abs(cliRoot)
	require.NoError(t, err)

	binaryPath := filepath.Join(t.TempDir(), "ddx")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = cliRoot
	out, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(out))
	require.NoError(t, os.Chmod(binaryPath, 0755))

	return binaryPath
}

// runInDir executes the binary with args in the given directory and returns combined output.
// It strips DDx-specific env vars so the subprocess uses only the isolated workDir state.
func runInDir(t *testing.T, binary, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	// Build a clean environment: keep PATH/HOME/USER/TMPDIR but strip DDx overrides
	// so the binary resolves config and storage from dir, not from the parent shell.
	ddxOverrides := map[string]bool{
		"DDX_BEAD_DIR":          true,
		"DDX_BEAD_PREFIX":       true,
		"DDX_BEAD_BACKEND":      true,
		"DDX_LIBRARY_BASE_PATH": true,
	}
	for _, kv := range os.Environ() {
		key := kv
		if idx := strings.Index(kv, "="); idx >= 0 {
			key = kv[:idx]
		}
		if !ddxOverrides[key] {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.CombinedOutput()
	output := string(out)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return output, exitCode
}

// TestE2ESmokeJourney validates the core onboarding journey end-to-end using
// the built binary in a fresh temp git directory.
func TestE2ESmokeJourney(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E smoke test in short mode")
	}

	binary := getSmokeTestBinaryPath(t)

	// Create isolated temp directory with a git repo
	workDir := t.TempDir()

	gitCmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "smoke@example.com"},
		{"git", "config", "user.name", "Smoke Test"},
	}
	for _, args := range gitCmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = workDir
		require.NoError(t, c.Run(), "git setup: %v", args)
	}

	// Create an initial commit so git has something to work with
	readmePath := filepath.Join(workDir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Smoke Test\n"), 0644))
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "Initial commit"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = workDir
		require.NoError(t, c.Run(), "git commit: %v", args)
	}

	// TC-001: Init
	t.Run("TC-001-Init", func(t *testing.T) {
		testLibraryURL := "file://" + GetTestLibraryPath()
		out, code := runInDir(t, binary, workDir,
			"init",
			"--repository", testLibraryURL,
			"--branch", "master",
			"--silent",
			"--skip-claude-injection",
		)
		t.Logf("init output: %s", out)
		assert.Equal(t, 0, code, "ddx init should exit 0")
		assert.FileExists(t, filepath.Join(workDir, ".ddx", "config.yaml"))
	})

	// Seed a .md persona so persona commands can find it
	personasDir := filepath.Join(workDir, ".ddx", "library", "personas")
	require.NoError(t, os.MkdirAll(personasDir, 0755))
	personaContent := `---
name: strict-code-reviewer
roles: [code-reviewer]
description: Strict code reviewer
tags: [strict]
---

# Strict Code Reviewer

You are a strict code reviewer.`
	require.NoError(t, os.WriteFile(
		filepath.Join(personasDir, "strict-code-reviewer.md"),
		[]byte(personaContent),
		0644,
	))

	// TC-002: List
	t.Run("TC-002-List", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "list")
		t.Logf("list output: %s", out)
		assert.Equal(t, 0, code, "ddx list should exit 0")
		// Output should contain at least one resource category
		hasCategory := strings.Contains(out, "Templates") ||
			strings.Contains(out, "Prompts") ||
			strings.Contains(out, "Personas") ||
			strings.Contains(out, "Mcp-Servers")
		assert.True(t, hasCategory, "list output should contain document categories; got: %s", out)
	})

	// TC-003: Doctor
	t.Run("TC-003-Doctor", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "doctor")
		t.Logf("doctor output: %s", out)
		assert.Equal(t, 0, code, "ddx doctor should exit 0")
		assert.NotContains(t, strings.ToUpper(out), "ERROR", "doctor output should not contain ERROR")
	})

	// TC-004: Persona list
	t.Run("TC-004-PersonaList", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "persona", "list")
		t.Logf("persona list output: %s", out)
		assert.Equal(t, 0, code, "ddx persona list should exit 0")
		assert.NotContains(t, out, "No personas found", "should list at least one persona")
		assert.Contains(t, out, "strict-code-reviewer")
	})

	// TC-005: Persona bind
	t.Run("TC-005-PersonaBind", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "persona", "bind", "code-reviewer", "strict-code-reviewer")
		t.Logf("persona bind output: %s", out)
		assert.Equal(t, 0, code, "ddx persona bind should exit 0; output: %s", out)
	})

	// TC-006: Bead create
	t.Run("TC-006-BeadCreate", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "bead", "create", "Smoke test", "--type", "task")
		t.Logf("bead create output: %s", out)
		assert.Equal(t, 0, code, "ddx bead create should exit 0")
		// Output should be a bead ID (non-empty trimmed string)
		assert.NotEmpty(t, strings.TrimSpace(out), "bead create should output a bead ID")
	})

	// TC-007: Bead list
	t.Run("TC-007-BeadList", func(t *testing.T) {
		out, code := runInDir(t, binary, workDir, "bead", "list")
		t.Logf("bead list output: %s", out)
		assert.Equal(t, 0, code, "ddx bead list should exit 0")
		assert.Contains(t, out, "Smoke test", "bead list should contain the created bead title")
	})
}
