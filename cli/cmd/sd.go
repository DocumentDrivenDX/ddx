package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/easel/ddx/internal/artifact"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newSDCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sd",
		Short: "Manage Solution Designs",
		Long: `Manage Solution Designs (SDs).

Solution designs describe the chosen approach for a feature,
including scope, acceptance criteria, and component changes.

Examples:
  ddx sd create "User authentication flow" --depends-on FEAT-002
  ddx sd list
  ddx sd show SD-001
  ddx sd validate --all`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newSDCreateCommand())
	cmd.AddCommand(f.newSDListCommand())
	cmd.AddCommand(f.newSDShowCommand())
	cmd.AddCommand(f.newSDValidateCommand())

	return cmd
}

func (f *CommandFactory) newSDCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new Solution Design",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewSDManager(dir)

			var deps []string
			if v, _ := cmd.Flags().GetString("depends-on"); v != "" {
				deps = strings.Split(v, ",")
			}
			if v, _ := cmd.Flags().GetString("feature"); v != "" {
				deps = append(deps, v)
			}

			path, err := m.Create(artifact.TypeSD, args[0], deps)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("depends-on", "", "Comma-separated dependency IDs")
	cmd.Flags().String("feature", "", "Feature ID this design implements")
	cmd.Flags().String("dir", "", "Output directory (default: docs/designs)")

	return cmd
}

func (f *CommandFactory) newSDListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List Solution Designs",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewSDManager(dir)

			asJSON, _ := cmd.Flags().GetBool("json")

			infos, err := m.List(artifact.TypeSD)
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(infos)
			}

			if len(infos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Solution Designs found.")
				return nil
			}

			for _, info := range infos {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", info.ID, info.Title)
			}
			return nil
		},
	}

	cmd.Flags().String("dir", "", "Directory to scan (default: docs/designs)")
	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newSDShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a Solution Design",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewSDManager(dir)

			content, _, err := m.Show(artifact.TypeSD, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), content)
			return nil
		},
	}

	cmd.Flags().String("dir", "", "Directory to scan (default: docs/designs)")

	return cmd
}

func (f *CommandFactory) newSDValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate Solution Design structure",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			m := artifact.NewSDManager(dir)
			all, _ := cmd.Flags().GetBool("all")

			var errs []artifact.ValidationError
			if all || len(args) == 0 {
				errs = m.ValidateAll(artifact.TypeSD)
			} else {
				errs = m.Validate(artifact.TypeSD, args[0])
			}

			if len(errs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "All Solution Designs valid.")
				return nil
			}

			for _, e := range errs {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", e.Path, e.Message)
			}
			return fmt.Errorf("%d validation error(s)", len(errs))
		},
	}

	cmd.Flags().Bool("all", false, "Validate all SDs in directory")
	cmd.Flags().String("dir", "", "Directory to scan (default: docs/designs)")

	return cmd
}
