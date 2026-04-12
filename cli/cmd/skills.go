package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/skills"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newSkillsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Validate bundled skill packages",
		Long: `Validate SKILL.md metadata for DDx or plugin skill packages.

The validator enforces the DDx skill contract:
• YAML frontmatter must exist
• top-level "name" is required
• top-level "description" is required
• nested "skill:" metadata is rejected
• markdown body must not be empty

Examples:
  ddx skills check
  ddx skills check skills
  ddx skills check .agents/skills
  ddx skills check ~/.ddx/plugins/helix/skills`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newSkillsCheckCommand())
	return cmd
}

func (f *CommandFactory) newSkillsCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check [path...]",
		Short: "Validate SKILL.md metadata",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.runSkillsCheck(cmd, args)
		},
	}
}

func (f *CommandFactory) runSkillsCheck(cmd *cobra.Command, args []string) error {
	paths := args
	if len(paths) == 0 {
		paths = f.defaultSkillCheckPaths()
	}

	files, issues := skills.ValidatePaths(paths)
	for _, issue := range issues {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skill issue: %s\n", issue.Error())
	}
	if len(issues) > 0 {
		return NewExitError(ExitCodeGeneralError, fmt.Sprintf("skill validation failed with %d issue(s)", len(issues)))
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "validated %d skill files\n", len(files))
	return nil
}

func (f *CommandFactory) defaultSkillCheckPaths() []string {
	candidates := []string{
		filepath.Join(f.WorkingDir, "skills"),
		filepath.Join(f.WorkingDir, ".agents", "skills"),
		filepath.Join(f.WorkingDir, ".claude", "skills"),
		filepath.Join(f.WorkingDir, "cli", "internal", "skills"),
	}

	var paths []string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			paths = append(paths, candidate)
		}
	}
	return paths
}
