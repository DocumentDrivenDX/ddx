package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// Test that demonstrates the functional design principles
func TestInitProject_FunctionalDesign(t *testing.T) {
	// Test 1: Test with git validation (should fail in non-git directory)
	te := NewTestEnvironment(t, WithGitInit(false))

	opts := InitOptions{
		Force: false,
		NoGit: false, // Enable git validation for test
	}

	result, err := initProject(te.Dir, opts)
	// Should fail because tempDir is not a git repository
	if err == nil {
		t.Error("Expected error for non-git directory, but got none")
	}
	if result != nil {
		t.Error("Expected nil result on error")
	}

	// Test 2: Test with NoGit option (should succeed)
	opts.NoGit = true
	result, err = initProject(te.Dir, opts)

	// Should work now that we skip git validation
	if err != nil {
		t.Errorf("Expected success with NoGit=true, but got error: %v", err)
	}
	if result == nil {
		t.Error("Expected result struct, got nil")
	}
}

func TestInitProject_ManagesScratchGitignoreRules(t *testing.T) {
	te := NewTestEnvironment(t, WithGitInit(false))

	result, err := initProject(te.Dir, InitOptions{NoGit: true})
	if err != nil {
		t.Fatalf("expected initProject success, got %v", err)
	}
	if result == nil || !result.ConfigCreated {
		t.Fatalf("expected config to be created")
	}

	raw, err := os.ReadFile(filepath.Join(te.Dir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to be written, got %v", err)
	}
	content := string(raw)
	if !containsExactLine(content, ".ddx/.execute-bead-wt-*/") {
		t.Fatalf("expected execute-bead scratch worktree ignore rule in .gitignore, got:\n%s", content)
	}
	if containsExactLine(content, ".ddx/executions/") {
		t.Fatalf("did not expect tracked execution evidence to be ignored, got:\n%s", content)
	}
}

// Test the pure git validation function
func TestValidateGitRepo_Pure(t *testing.T) {
	// Test with non-git directory
	te := NewTestEnvironment(t, WithGitInit(false))

	// Should return error for non-git directory
	err := validateGitRepo(te.Dir)
	if err == nil {
		t.Error("Expected error for non-git directory, but got none")
	}
}

// Test InitOptions struct
func TestInitOptions_Structure(t *testing.T) {
	opts := InitOptions{
		Force: true,
		NoGit: false,
	}

	// Verify all fields are accessible
	if !opts.Force {
		t.Error("Force field not set correctly")
	}
	if opts.NoGit {
		t.Error("NoGit field not set correctly")
	}
}

// Test InitResult struct
func TestInitResult_Structure(t *testing.T) {
	result := &InitResult{
		ConfigCreated: true,
		LibraryExists: false,
	}

	// Verify all fields are accessible
	if !result.ConfigCreated {
		t.Error("ConfigCreated field not set correctly")
	}
	if result.LibraryExists {
		t.Error("LibraryExists field not set correctly")
	}
}

// Test that business logic functions don't depend on cobra.Command
func TestBusinessLogicIndependence(t *testing.T) {
	// These functions should work without cobra.Command

	// Test validateGitRepo - pure function
	te := NewTestEnvironment(t, WithGitInit(false))

	err := validateGitRepo(te.Dir)
	if err == nil {
		t.Error("validateGitRepo should work without cobra.Command")
	}

	// Test initializeSynchronizationPure - pure function
	cfg := &config.Config{
		Library: &config.LibraryConfig{
			Repository: &config.RepositoryConfig{
				URL:    "",
				Branch: "",
			},
		},
	}
	err = initializeSynchronizationPure(cfg)
	if err == nil {
		t.Error("initializeSynchronizationPure should return error for empty URL")
	}
}
