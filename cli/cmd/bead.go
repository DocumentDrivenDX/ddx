package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/easel/ddx/internal/bead"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bead",
		Short: "Manage beads (portable work items)",
		Long: `Manage beads — portable, ephemeral work items with metadata.

Beads provide a lightweight work queue for AI agents and developers.
They track tasks, dependencies, and status without coupling to any
specific workflow methodology.

Examples:
  ddx bead init                                    # Initialize bead storage
  ddx bead create "Fix auth bug" --type bug        # Create a bead
  ddx bead list --status open                      # List open beads
  ddx bead ready                                   # Show beads ready for work
  ddx bead dep add <id> <dep-id>                   # Add a dependency
  ddx bead import --from jsonl beads.jsonl          # Import from JSONL`,
		Aliases: []string{"beads"},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newBeadInitCommand())
	cmd.AddCommand(f.newBeadCreateCommand())
	cmd.AddCommand(f.newBeadShowCommand())
	cmd.AddCommand(f.newBeadUpdateCommand())
	cmd.AddCommand(f.newBeadCloseCommand())
	cmd.AddCommand(f.newBeadListCommand())
	cmd.AddCommand(f.newBeadReadyCommand())
	cmd.AddCommand(f.newBeadBlockedCommand())
	cmd.AddCommand(f.newBeadStatusCommand())
	cmd.AddCommand(f.newBeadDepCommand())
	cmd.AddCommand(f.newBeadImportCommand())
	cmd.AddCommand(f.newBeadExportCommand())

	return cmd
}

func (f *CommandFactory) beadStore() *bead.Store {
	return bead.NewStore("")
}

func (f *CommandFactory) newBeadInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize bead storage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			if err := s.Init(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized bead storage at %s\n", s.File)
			return nil
		},
	}
}

func (f *CommandFactory) newBeadCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			b := &bead.Bead{Title: args[0]}

			if v, _ := cmd.Flags().GetString("type"); v != "" {
				b.Type = v
			}
			if v, _ := cmd.Flags().GetInt("priority"); cmd.Flags().Changed("priority") {
				b.Priority = v
			}
			if v, _ := cmd.Flags().GetString("labels"); v != "" {
				b.Labels = strings.Split(v, ",")
			}
			if v, _ := cmd.Flags().GetString("acceptance"); v != "" {
				b.Acceptance = v
			}
			if v, _ := cmd.Flags().GetString("description"); v != "" {
				b.Description = v
			}
			if v, _ := cmd.Flags().GetString("parent"); v != "" {
				b.Parent = v
			}

			if err := s.Create(b); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b.ID)
			return nil
		},
	}

	cmd.Flags().String("type", "", "Bead type (task, bug, epic, chore)")
	cmd.Flags().Int("priority", 2, "Priority (0=highest, 4=lowest)")
	cmd.Flags().String("labels", "", "Comma-separated labels")
	cmd.Flags().String("acceptance", "", "Acceptance criteria")
	cmd.Flags().String("description", "", "Description")
	cmd.Flags().String("parent", "", "Parent bead ID")

	return cmd
}

func (f *CommandFactory) newBeadShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			b, err := s.Get(args[0])
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(b)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "ID:       %s\n", b.ID)
			fmt.Fprintf(out, "Title:    %s\n", b.Title)
			fmt.Fprintf(out, "Type:     %s\n", b.Type)
			fmt.Fprintf(out, "Status:   %s\n", b.Status)
			fmt.Fprintf(out, "Priority: %d\n", b.Priority)
			if len(b.Labels) > 0 {
				fmt.Fprintf(out, "Labels:   %s\n", strings.Join(b.Labels, ", "))
			}
			if b.Assignee != "" {
				fmt.Fprintf(out, "Assignee: %s\n", b.Assignee)
			}
			if b.Parent != "" {
				fmt.Fprintf(out, "Parent:   %s\n", b.Parent)
			}
			if len(b.Deps) > 0 {
				fmt.Fprintf(out, "Deps:     %s\n", strings.Join(b.Deps, ", "))
			}
			if b.Description != "" {
				fmt.Fprintf(out, "Desc:     %s\n", b.Description)
			}
			if b.Acceptance != "" {
				fmt.Fprintf(out, "Accept:   %s\n", b.Acceptance)
			}
			fmt.Fprintf(out, "Created:  %s\n", b.Created.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(out, "Updated:  %s\n", b.Updated.Format("2006-01-02 15:04:05"))
			for k, v := range b.Extra {
				fmt.Fprintf(out, "%s: %v\n", k, v)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newBeadUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			return s.Update(args[0], func(b *bead.Bead) {
				if v, _ := cmd.Flags().GetString("title"); cmd.Flags().Changed("title") {
					b.Title = v
				}
				if v, _ := cmd.Flags().GetString("status"); cmd.Flags().Changed("status") {
					b.Status = v
				}
				if v, _ := cmd.Flags().GetInt("priority"); cmd.Flags().Changed("priority") {
					b.Priority = v
				}
				if v, _ := cmd.Flags().GetString("labels"); cmd.Flags().Changed("labels") {
					if v == "" {
						b.Labels = []string{}
					} else {
						b.Labels = strings.Split(v, ",")
					}
				}
				if v, _ := cmd.Flags().GetString("acceptance"); cmd.Flags().Changed("acceptance") {
					b.Acceptance = v
				}
				if v, _ := cmd.Flags().GetString("assignee"); cmd.Flags().Changed("assignee") {
					b.Assignee = v
				}
				if claim, _ := cmd.Flags().GetBool("claim"); claim {
					b.Status = bead.StatusInProgress
					b.Assignee = "ddx"
					if b.Extra == nil {
						b.Extra = make(map[string]any)
					}
					b.Extra["claimed-at"] = time.Now().UTC().Format(time.RFC3339)
					b.Extra["claimed-pid"] = fmt.Sprintf("%d", os.Getpid())
				}
			})
		},
	}

	cmd.Flags().String("title", "", "New title")
	cmd.Flags().String("status", "", "New status (open, in_progress, closed)")
	cmd.Flags().Int("priority", 0, "New priority")
	cmd.Flags().String("labels", "", "New labels (comma-separated)")
	cmd.Flags().String("acceptance", "", "New acceptance criteria")
	cmd.Flags().String("assignee", "", "New assignee")
	cmd.Flags().Bool("claim", false, "Claim: set status=in_progress, assignee=ddx")

	return cmd
}

func (f *CommandFactory) newBeadCloseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "Close a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.beadStore().Close(args[0])
		},
	}
}

func (f *CommandFactory) newBeadListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List beads",
		Aliases: []string{"ls"},
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			status, _ := cmd.Flags().GetString("status")
			label, _ := cmd.Flags().GetString("label")
			asJSON, _ := cmd.Flags().GetBool("json")

			beads, err := s.List(status, label)
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(beads)
			}

			if len(beads) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No beads found.")
				return nil
			}

			for _, b := range beads {
				labels := ""
				if len(b.Labels) > 0 {
					labels = " [" + strings.Join(b.Labels, ",") + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-12s  P%d  %s%s\n",
					b.ID, b.Status, b.Priority, b.Title, labels)
			}
			return nil
		},
	}

	cmd.Flags().String("status", "", "Filter by status")
	cmd.Flags().String("label", "", "Filter by label")
	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newBeadReadyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "List beads ready for work (no unclosed deps)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			execution, _ := cmd.Flags().GetBool("execution")

			var beads []bead.Bead
			var err error
			if execution {
				beads, err = s.ReadyExecution()
			} else {
				beads, err = s.Ready()
			}
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(beads)
			}

			if len(beads) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No ready beads.")
				return nil
			}

			for _, b := range beads {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s\n", b.ID, b.Priority, b.Title)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	cmd.Flags().Bool("execution", false, "Filter by execution-eligible and not superseded")
	return cmd
}

func (f *CommandFactory) newBeadBlockedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocked",
		Short: "List beads blocked by unclosed deps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			beads, err := s.Blocked()
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(beads)
			}

			if len(beads) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No blocked beads.")
				return nil
			}

			for _, b := range beads {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  deps: %s\n",
					b.ID, b.Priority, b.Title, strings.Join(b.Deps, ", "))
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newBeadStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show bead counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			counts, err := s.Status()
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(counts)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Total:   %d\n", counts.Total)
			fmt.Fprintf(out, "Open:    %d\n", counts.Open)
			fmt.Fprintf(out, "Closed:  %d\n", counts.Closed)
			fmt.Fprintf(out, "Ready:   %d\n", counts.Ready)
			fmt.Fprintf(out, "Blocked: %d\n", counts.Blocked)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func (f *CommandFactory) newBeadDepCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dep",
		Short: "Manage bead dependencies",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add <id> <dep-id>",
		Short: "Add a dependency (id depends on dep-id)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.beadStore().DepAdd(args[0], args[1])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <id> <dep-id>",
		Short: "Remove a dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.beadStore().DepRemove(args[0], args[1])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "tree [id]",
		Short: "Show dependency tree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rootID := ""
			if len(args) > 0 {
				rootID = args[0]
			}
			tree, err := f.beadStore().DepTree(rootID)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), tree)
			return nil
		},
	})

	return cmd
}

func (f *CommandFactory) newBeadImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import beads from external source",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			from, _ := cmd.Flags().GetString("from")
			file := ""
			if len(args) > 0 {
				file = args[0]
			}

			n, err := s.Import(from, file)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d beads\n", n)
			return nil
		},
	}
	cmd.Flags().String("from", "auto", "Import source: auto, bd, br, jsonl")
	return cmd
}

func (f *CommandFactory) newBeadExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [file]",
		Short: "Export beads as JSONL",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			stdout, _ := cmd.Flags().GetBool("stdout")

			if stdout || len(args) == 0 {
				return s.ExportTo(cmd.OutOrStdout())
			}
			return s.ExportToFile(args[0])
		},
	}
	cmd.Flags().Bool("stdout", false, "Write to stdout")
	return cmd
}
