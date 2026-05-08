package metaprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInjectMetaPrompt tests meta-prompt injection
func TestInjectMetaPrompt(t *testing.T) {
	tests := []struct {
		name          string
		existingFile  string
		promptPath    string
		promptContent string
		expectError   bool
		expectContent string
	}{
		{
			name:          "inject into new file",
			existingFile:  "",
			promptPath:    "claude/system-prompts/test.md",
			promptContent: "# Test Prompt\nContent here",
			expectError:   false,
			expectContent: "<!-- DDX-META-PROMPT:START -->",
		},
		{
			name:          "inject into existing file",
			existingFile:  "# CLAUDE.md\n\nExisting content",
			promptPath:    "claude/system-prompts/test.md",
			promptContent: "# Test Prompt",
			expectError:   false,
			expectContent: "Existing content",
		},
		{
			name:          "replace existing meta-prompt",
			existingFile:  "Content\n<!-- DDX-META-PROMPT:START -->\nOld\n<!-- DDX-META-PROMPT:END -->",
			promptPath:    "claude/system-prompts/new.md",
			promptContent: "New prompt",
			expectError:   false,
			expectContent: "New prompt",
		},
		{
			name:         "prompt file not found",
			existingFile: "",
			promptPath:   "nonexistent/prompt.md",
			expectError:  true,
		},
		{
			name:          "prompt too large",
			existingFile:  "",
			promptPath:    "claude/system-prompts/huge.md",
			promptContent: strings.Repeat("x", MaxMetaPromptSize+1),
			expectError:   true,
		},
		{
			name:         "empty prompt path",
			existingFile: "",
			promptPath:   "",
			expectError:  true,
		},
		{
			name:          "preserves content outside markers",
			existingFile:  "Before\n<!-- DDX-META-PROMPT:START -->\nOld\n<!-- DDX-META-PROMPT:END -->\nAfter",
			promptPath:    "claude/system-prompts/test.md",
			promptContent: "New",
			expectError:   false,
			expectContent: "Before",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory
			testDir := t.TempDir()
			claudePath := filepath.Join(testDir, "CLAUDE.md")
			libraryPath := filepath.Join(testDir, ".ddx", "plugins", "ddx")

			// Create library directory structure
			promptDir := filepath.Join(libraryPath, "prompts", filepath.Dir(tt.promptPath))
			if err := os.MkdirAll(promptDir, 0755); err != nil {
				t.Fatalf("Failed to create prompt dir: %v", err)
			}

			// Create prompt file if content provided
			if tt.promptContent != "" {
				promptFile := filepath.Join(libraryPath, "prompts", tt.promptPath)
				if err := os.WriteFile(promptFile, []byte(tt.promptContent), 0644); err != nil {
					t.Fatalf("Failed to create prompt file: %v", err)
				}
			}

			// Create existing CLAUDE.md if provided
			if tt.existingFile != "" {
				if err := os.WriteFile(claudePath, []byte(tt.existingFile), 0644); err != nil {
					t.Fatalf("Failed to create CLAUDE.md: %v", err)
				}
			}

			// Create injector
			injector := NewMetaPromptInjectorWithPaths(
				"CLAUDE.md",
				filepath.Join(".ddx", "plugins", "ddx"),
				testDir,
			)

			// Test injection
			err := injector.InjectMetaPrompt(tt.promptPath)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify content if no error expected
			if !tt.expectError && tt.expectContent != "" {
				content, err := os.ReadFile(claudePath)
				if err != nil {
					t.Fatalf("Failed to read CLAUDE.md: %v", err)
				}
				if !strings.Contains(string(content), tt.expectContent) {
					t.Errorf("Expected content to contain %q, got:\n%s", tt.expectContent, string(content))
				}
			}

			// Verify markers are present when successful
			if !tt.expectError {
				content, _ := os.ReadFile(claudePath)
				if !strings.Contains(string(content), MetaPromptStartMarker) {
					t.Errorf("Missing start marker in CLAUDE.md")
				}
				if !strings.Contains(string(content), MetaPromptEndMarker) {
					t.Errorf("Missing end marker in CLAUDE.md")
				}
			}
		})
	}
}

// TestIsInSync tests sync detection
func TestIsInSync(t *testing.T) {
	t.Run("in sync after injection", func(t *testing.T) {
		// Setup test directory
		testDir := t.TempDir()
		libraryPath := filepath.Join(testDir, ".ddx", "plugins", "ddx")
		promptPath := "claude/system-prompts/test.md"
		promptContent := "Test prompt content"

		// Create library prompt
		promptDir := filepath.Join(libraryPath, "prompts", "claude", "system-prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			t.Fatalf("Failed to create prompt dir: %v", err)
		}
		promptFile := filepath.Join(libraryPath, "prompts", promptPath)
		if err := os.WriteFile(promptFile, []byte(promptContent), 0644); err != nil {
			t.Fatalf("Failed to create prompt file: %v", err)
		}

		// Create injector and inject
		injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", filepath.Join(".ddx", "plugins", "ddx"), testDir)
		if err := injector.InjectMetaPrompt(promptPath); err != nil {
			t.Fatalf("Failed to inject: %v", err)
		}

		// Check sync - should be in sync
		inSync, err := injector.IsInSync()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !inSync {
			t.Errorf("Expected in sync after injection")
		}
	})

	t.Run("out of sync after library change", func(t *testing.T) {
		// Setup test directory
		testDir := t.TempDir()
		libraryPath := filepath.Join(testDir, ".ddx", "plugins", "ddx")
		promptPath := "claude/system-prompts/test.md"

		// Create library prompt
		promptDir := filepath.Join(libraryPath, "prompts", "claude", "system-prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			t.Fatalf("Failed to create prompt dir: %v", err)
		}
		promptFile := filepath.Join(libraryPath, "prompts", promptPath)
		if err := os.WriteFile(promptFile, []byte("Old content"), 0644); err != nil {
			t.Fatalf("Failed to create prompt file: %v", err)
		}

		// Inject old content
		injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", filepath.Join(".ddx", "plugins", "ddx"), testDir)
		if err := injector.InjectMetaPrompt(promptPath); err != nil {
			t.Fatalf("Failed to inject: %v", err)
		}

		// Update library prompt
		if err := os.WriteFile(promptFile, []byte("New content"), 0644); err != nil {
			t.Fatalf("Failed to update prompt file: %v", err)
		}

		// Check sync - should be out of sync
		inSync, err := injector.IsInSync()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if inSync {
			t.Errorf("Expected out of sync after library change")
		}
	})

	t.Run("no meta-prompt section", func(t *testing.T) {
		testDir := t.TempDir()
		claudePath := filepath.Join(testDir, "CLAUDE.md")

		// Create CLAUDE.md without meta-prompt
		if err := os.WriteFile(claudePath, []byte("# CLAUDE.md\n\nNo meta-prompt"), 0644); err != nil {
			t.Fatalf("Failed to create CLAUDE.md: %v", err)
		}

		injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", filepath.Join(".ddx", "plugins", "ddx"), testDir)
		_, err := injector.IsInSync()

		if err == nil {
			t.Errorf("Expected error for missing meta-prompt section")
		}
	})

	t.Run("whitespace differences ignored", func(t *testing.T) {
		// Setup test directory
		testDir := t.TempDir()
		libraryPath := filepath.Join(testDir, ".ddx", "plugins", "ddx")
		promptPath := "claude/system-prompts/test.md"

		// Create library prompt with specific whitespace
		promptDir := filepath.Join(libraryPath, "prompts", "claude", "system-prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			t.Fatalf("Failed to create prompt dir: %v", err)
		}
		promptFile := filepath.Join(libraryPath, "prompts", promptPath)
		if err := os.WriteFile(promptFile, []byte("Test   prompt"), 0644); err != nil {
			t.Fatalf("Failed to create prompt file: %v", err)
		}

		// Inject
		injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", filepath.Join(".ddx", "plugins", "ddx"), testDir)
		if err := injector.InjectMetaPrompt(promptPath); err != nil {
			t.Fatalf("Failed to inject: %v", err)
		}

		// Update library with different whitespace
		if err := os.WriteFile(promptFile, []byte("Test prompt"), 0644); err != nil {
			t.Fatalf("Failed to update prompt file: %v", err)
		}

		// Should still be in sync due to normalization
		inSync, err := injector.IsInSync()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !inSync {
			t.Errorf("Expected in sync (whitespace should be ignored)")
		}
	})

	t.Run("library file missing", func(t *testing.T) {
		testDir := t.TempDir()
		claudePath := filepath.Join(testDir, "CLAUDE.md")

		// Create CLAUDE.md with meta-prompt referencing missing file
		claudeContent := buildCLAUDEWithPrompt("Test", "claude/system-prompts/missing.md")
		if err := os.WriteFile(claudePath, []byte(claudeContent), 0644); err != nil {
			t.Fatalf("Failed to create CLAUDE.md: %v", err)
		}

		injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", filepath.Join(".ddx", "plugins", "ddx"), testDir)
		inSync, err := injector.IsInSync()

		if err != nil {
			t.Errorf("Unexpected error (should return false, not error): %v", err)
		}
		if inSync {
			t.Errorf("Expected out of sync when library file missing")
		}
	})
}

// TestRemoveMetaPrompt tests meta-prompt removal
func TestRemoveMetaPrompt(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectContent string
		expectError   bool
	}{
		{
			name:          "remove existing section",
			input:         "Before\n<!-- DDX-META-PROMPT:START -->\nPrompt\n<!-- DDX-META-PROMPT:END -->\nAfter",
			expectContent: "Before\n\nAfter",
			expectError:   false,
		},
		{
			name:          "no section to remove",
			input:         "# CLAUDE.md\n\nNo meta-prompt here",
			expectContent: "# CLAUDE.md\n\nNo meta-prompt here",
			expectError:   false,
		},
		{
			name:          "malformed section (no end marker)",
			input:         "Content\n<!-- DDX-META-PROMPT:START -->\nNo end marker",
			expectContent: "Content",
			expectError:   false,
		},
		{
			name:          "section at start",
			input:         "<!-- DDX-META-PROMPT:START -->\nPrompt\n<!-- DDX-META-PROMPT:END -->\nAfter",
			expectContent: "After",
			expectError:   false,
		},
		{
			name:          "section at end",
			input:         "Before\n<!-- DDX-META-PROMPT:START -->\nPrompt\n<!-- DDX-META-PROMPT:END -->",
			expectContent: "Before",
			expectError:   false,
		},
		{
			name:          "file doesn't exist",
			input:         "",
			expectContent: "",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory
			testDir := t.TempDir()
			claudePath := filepath.Join(testDir, "CLAUDE.md")

			// Create CLAUDE.md if input provided
			if tt.input != "" {
				if err := os.WriteFile(claudePath, []byte(tt.input), 0644); err != nil {
					t.Fatalf("Failed to create CLAUDE.md: %v", err)
				}
			}

			// Create injector
			injector := NewMetaPromptInjectorWithPaths(
				"CLAUDE.md",
				filepath.Join(".ddx", "plugins", "ddx"),
				testDir,
			)

			// Test removal
			err := injector.RemoveMetaPrompt()

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify content
			if tt.expectContent != "" || tt.input != "" {
				if fileExists(claudePath) {
					content, err := os.ReadFile(claudePath)
					if err != nil {
						t.Fatalf("Failed to read CLAUDE.md: %v", err)
					}
					if strings.TrimSpace(string(content)) != strings.TrimSpace(tt.expectContent) {
						t.Errorf("Expected content:\n%s\n\nGot:\n%s", tt.expectContent, string(content))
					}
				} else if tt.expectContent != "" {
					t.Errorf("Expected file to exist with content, but it doesn't")
				}
			}
		})
	}
}

// TestGetCurrentMetaPrompt tests getting current prompt source
func TestGetCurrentMetaPrompt(t *testing.T) {
	tests := []struct {
		name          string
		claudeContent string
		expectPath    string
		expectError   bool
	}{
		{
			name:          "valid meta-prompt",
			claudeContent: buildCLAUDEWithPrompt("Content", "claude/system-prompts/focused.md"),
			expectPath:    "claude/system-prompts/focused.md",
			expectError:   false,
		},
		{
			name:          "no meta-prompt section",
			claudeContent: "# CLAUDE.md\n\nNo prompt",
			expectPath:    "",
			expectError:   true,
		},
		{
			name:          "file doesn't exist",
			claudeContent: "",
			expectPath:    "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory
			testDir := t.TempDir()
			claudePath := filepath.Join(testDir, "CLAUDE.md")

			// Create CLAUDE.md if content provided
			if tt.claudeContent != "" {
				if err := os.WriteFile(claudePath, []byte(tt.claudeContent), 0644); err != nil {
					t.Fatalf("Failed to create CLAUDE.md: %v", err)
				}
			}

			// Create injector
			injector := NewMetaPromptInjectorWithPaths(
				"CLAUDE.md",
				filepath.Join(".ddx", "plugins", "ddx"),
				testDir,
			)

			// Test getting current prompt
			path, err := injector.GetCurrentMetaPrompt()

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check path
			if !tt.expectError && path != tt.expectPath {
				t.Errorf("Expected path %q, got %q", tt.expectPath, path)
			}
		})
	}
}

// Helper function to build CLAUDE.md with meta-prompt section
func buildCLAUDEWithPrompt(promptContent, sourcePath string) string {
	return strings.Join([]string{
		"# CLAUDE.md",
		"",
		"Project content here",
		"",
		MetaPromptStartMarker,
		"<!-- Source: " + sourcePath + " -->",
		promptContent,
		MetaPromptEndMarker,
		"",
		"More content",
	}, "\n")
}
