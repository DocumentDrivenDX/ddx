package cmd

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

var validCheckpointName = regexp.MustCompile(`^[a-zA-Z0-9.\-]+$`)

func (f *CommandFactory) newCheckpointCommand() *cobra.Command {
	var list bool
	var restore string

	cmd := &cobra.Command{
		Use:   "checkpoint [name]",
		Short: "Create, list, or restore named git checkpoints",
		Long: `Manage named checkpoints as lightweight git tags with a DDx prefix.

Examples:
  ddx checkpoint pre-build          # Create checkpoint tagged ddx/pre-build
  ddx checkpoint --list              # List all DDx checkpoints
  ddx checkpoint --restore pre-build # Restore working tree to checkpoint`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix := "ddx/"
			if cfg, err := config.LoadWithWorkingDir(f.docRoot()); err == nil && cfg.Git != nil && cfg.Git.CheckpointPrefix != "" {
				prefix = cfg.Git.CheckpointPrefix
			}

			if list {
				return runCheckpointList(cmd, prefix)
			}

			if restore != "" {
				return runCheckpointRestore(cmd, prefix, restore)
			}

			if len(args) == 0 {
				return fmt.Errorf("provide a checkpoint name, --list, or --restore <name>")
			}

			return runCheckpointCreate(cmd, prefix, args[0])
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "List all checkpoints")
	cmd.Flags().StringVar(&restore, "restore", "", "Restore to this checkpoint")
	return cmd
}

func checkpointNameValid(name string) error {
	if name == "" {
		return fmt.Errorf("checkpoint name cannot be empty")
	}
	if !validCheckpointName.MatchString(name) {
		return fmt.Errorf("checkpoint name %q contains invalid characters (only alphanumeric, hyphens, and dots allowed)", name)
	}
	return nil
}

func runCheckpointCreate(cmd *cobra.Command, prefix, name string) error {
	if err := checkpointNameValid(name); err != nil {
		return err
	}
	tag := prefix + name
	out, err := exec.Command("git", "tag", tag).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "not a git repository") {
			return fmt.Errorf("not in a git repository; git features are unavailable")
		}
		if strings.Contains(msg, "already exists") {
			return fmt.Errorf("checkpoint %q already exists", name)
		}
		return fmt.Errorf("git tag failed: %s", msg)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "checkpoint created: %s\n", tag)
	return nil
}

func runCheckpointList(cmd *cobra.Command, prefix string) error {
	pattern := prefix + "*"
	out, err := exec.Command("git", "tag", "-l", pattern).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "not a git repository") {
			return fmt.Errorf("not in a git repository; git features are unavailable")
		}
		return fmt.Errorf("git tag failed: %s", msg)
	}
	output := strings.TrimRight(string(out), "\n")
	if output == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "No checkpoints found.")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), output)
	return nil
}

func runCheckpointRestore(cmd *cobra.Command, prefix, name string) error {
	if err := checkpointNameValid(name); err != nil {
		return err
	}
	tag := prefix + name
	out, err := exec.Command("git", "checkout", tag).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "not a git repository") {
			return fmt.Errorf("not in a git repository; git features are unavailable")
		}
		if strings.Contains(msg, "did not match") || strings.Contains(msg, "pathspec") {
			return fmt.Errorf("checkpoint %q not found", name)
		}
		return fmt.Errorf("git checkout failed: %s", msg)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "restored to checkpoint: %s\n", tag)
	return nil
}
