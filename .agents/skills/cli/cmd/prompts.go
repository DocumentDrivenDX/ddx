package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

// Command registration is now handled by command_factory.go
// This file only contains the prompts subcommand implementations

// runPromptsList implements the prompts list command
func (f *CommandFactory) runPromptsList(cmd *cobra.Command, args []string) error {
	workingDir := f.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}

	// Get library path using working directory
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var libPath string
	if cfg.Library != nil {
		libPath = cfg.Library.Path
	}

	promptsDir := filepath.Join(workingDir, libPath, "prompts")

	// Check if prompts directory exists
	if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No prompts directory found")
		return nil
	}

	// Get search filter
	searchFilter, _ := cmd.Flags().GetString("search")
	verbose, _ := cmd.Flags().GetBool("verbose")

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Available prompts:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Walk through prompts directory
	err = filepath.Walk(promptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == promptsDir {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(promptsDir, path)
		if err != nil {
			return err
		}

		// Skip hidden files
		if strings.HasPrefix(filepath.Base(path), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply search filter if provided
		if searchFilter != "" && !strings.Contains(strings.ToLower(path), strings.ToLower(searchFilter)) {
			return nil
		}

		// Print directories and markdown files
		if info.IsDir() {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📁 %s/\n", relPath)
		} else if strings.HasSuffix(path, ".md") {
			// Show full filename with extension in verbose mode
			if verbose {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  📝 %s\n", relPath)
			} else {
				// Remove .md extension for display
				name := strings.TrimSuffix(relPath, ".md")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  📝 %s\n", name)
			}
		}

		return nil
	})

	return err
}

// runPromptsShow implements the prompts show command
func runPromptsShow(cmd *cobra.Command, args []string) error {
	promptName := args[0]

	// Get working directory from command factory context
	workingDir := "."
	if factory, ok := cmd.Context().Value("factory").(*CommandFactory); ok {
		workingDir = factory.WorkingDir
	}

	// Get library path using working directory
	cfg, err := config.LoadWithWorkingDir(workingDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var libPath string
	if cfg.Library != nil {
		libPath = cfg.Library.Path
	}

	// Try different paths for the prompt
	possiblePaths := []string{
		filepath.Join(libPath, "prompts", promptName+".md"),
		filepath.Join(libPath, "prompts", promptName),
		filepath.Join(libPath, "prompts", promptName, "README.md"),
	}

	var promptPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			promptPath = path
			break
		}
	}

	if promptPath == "" {
		return fmt.Errorf("prompt not found: %s", promptName)
	}

	// Read and display the prompt
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return fmt.Errorf("failed to read prompt: %w", err)
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), string(content))
	return nil
}
