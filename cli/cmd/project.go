package cmd

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Inspect project-scoped DDx state",
	}

	cmd.AddCommand(f.newProjectWorktreeCommand())
	return cmd
}

func (f *CommandFactory) newProjectWorktreeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Inspect and manage the DDx project worktree registry",
	}

	cmd.AddCommand(f.newProjectWorktreeListCommand())
	cmd.AddCommand(f.newProjectWorktreeMasterCommand())
	return cmd
}

func (f *CommandFactory) newProjectWorktreeListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered worktrees for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := resolveProjectRoot("", f.WorkingDir)
			registry, err := ddxroot.LoadWorktreeRegistry(cmd.Context(), projectRoot)
			if err != nil {
				return err
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "MASTER\tPATH\tHOSTNAME\tFIRST_SEEN_AT\tLAST_SEEN_AT")
			for _, entry := range registry.Paths {
				marker := ""
				if entry.Path == registry.Master {
					marker = "*"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", marker, entry.Path, entry.Hostname, entry.FirstSeenAt, entry.LastSeenAt)
			}
			return tw.Flush()
		},
	}
}

func (f *CommandFactory) newProjectWorktreeMasterCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "master <path>",
		Short: "Set the master worktree path for the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := resolveProjectRoot("", f.WorkingDir)
			if err := ddxroot.SetMasterWorktree(cmd.Context(), projectRoot, args[0]); err != nil {
				return err
			}

			registry, err := ddxroot.LoadWorktreeRegistry(context.Background(), projectRoot)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "master worktree: %s\n", registry.Master)
			return err
		},
	}
}
