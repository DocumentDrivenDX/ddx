package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/easel/ddx/internal/artifact"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newADRCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adr",
		Short: "Manage Architecture Decision Records",
		Long: `Manage Architecture Decision Records (ADRs).

ADRs capture binding architecture decisions with context, rationale,
alternatives considered, and consequences.

Examples:
  ddx adr create "Use PostgreSQL for persistence"
  ddx adr list
  ddx adr show ADR-001
  ddx adr validate --all`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newADRCreateCommand())
	cmd.AddCommand(f.newADRListCommand())
	cmd.AddCommand(f.newADRShowCommand())
	cmd.AddCommand(f.newADRValidateCommand())

	return cmd
}

func (f *CommandFactory) newADRCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new ADR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewADRManager(dir)

			var deps []string
			if v, _ := cmd.Flags().GetString("depends-on"); v != "" {
				deps = strings.Split(v, ",")
			}

			path, err := m.Create(artifact.TypeADR, args[0], deps)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("depends-on", "", "Comma-separated dependency IDs")
	cmd.Flags().String("dir", "", "Output directory (default: docs/adr)")

	return cmd
}

func (f *CommandFactory) newADRListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ADRs",
		Aliases: []string{"ls"},
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewADRManager(dir)

			asJSON, _ := cmd.Flags().GetBool("json")
			status, _ := cmd.Flags().GetString("status")

			infos, err := m.List(artifact.TypeADR)
			if err != nil {
				return err
			}

			if status != "" {
				var filtered []artifact.ArtifactInfo
				for _, info := range infos {
					if strings.EqualFold(info.Status, status) {
						filtered = append(filtered, info)
					}
				}
				infos = filtered
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(infos)
			}

			if len(infos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No ADRs found.")
				return nil
			}

			for _, info := range infos {
				status := ""
				if info.Status != "" {
					status = fmt.Sprintf("  [%s]", info.Status)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s%s\n", info.ID, info.Title, status)
			}
			return nil
		},
	}

	cmd.Flags().String("dir", "", "Directory to scan (default: docs/adr)")
	cmd.Flags().String("status", "", "Filter by status")
	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newADRShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an ADR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewADRManager(dir)

			content, _, err := m.Show(artifact.TypeADR, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), content)
			return nil
		},
	}

	cmd.Flags().String("dir", "", "Directory to scan (default: docs/adr)")

	return cmd
}

func (f *CommandFactory) newADRValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate ADR structure",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewADRManager(dir)
			all, _ := cmd.Flags().GetBool("all")

			var errs []artifact.ValidationError
			if all || len(args) == 0 {
				errs = m.ValidateAll(artifact.TypeADR)
			} else {
				errs = m.Validate(artifact.TypeADR, args[0])
			}

			if len(errs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "All ADRs valid.")
				return nil
			}

			for _, e := range errs {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", e.Path, e.Message)
			}
			return fmt.Errorf("%d validation error(s)", len(errs))
		},
	}

	cmd.Flags().Bool("all", false, "Validate all ADRs in directory")
	cmd.Flags().String("dir", "", "Directory to scan (default: docs/adr)")

	return cmd
}
