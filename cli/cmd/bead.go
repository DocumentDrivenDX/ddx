package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
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
	cmd.AddCommand(f.newBeadEvidenceCommand())
	cmd.AddCommand(f.newBeadImportCommand())
	cmd.AddCommand(f.newBeadExportCommand())

	return cmd
}

// beadAutoCommit commits .ddx/beads.jsonl if git.auto_commit is "always".
// The operation string describes what happened (e.g. "create ddx-abc123").
// Errors are silently ignored — auto-commit is best-effort.
func (f *CommandFactory) beadAutoCommit(operation string) {
	cfg, err := config.LoadWithWorkingDir(f.WorkingDir)
	if err != nil {
		return
	}
	if cfg.Git == nil {
		return
	}
	acCfg := gitpkg.AutoCommitConfig{
		AutoCommit:   cfg.Git.AutoCommit,
		CommitPrefix: cfg.Git.CommitPrefix,
	}
	beadsFile := filepath.Join(f.WorkingDir, ".ddx", "beads.jsonl")
	_ = gitpkg.AutoCommit(beadsFile, "beads", operation, acCfg)
}

func (f *CommandFactory) beadStore() *bead.Store {
	dir := os.Getenv("DDX_BEAD_DIR")
	if dir == "" && f.WorkingDir != "" {
		dir = filepath.Join(f.WorkingDir, ".ddx")
	}
	return bead.NewStore(dir)
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

			// Auto-migrate from .helix/issues.jsonl if present
			n, migrated, err := s.MigrateFromHelix()
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: migration from .helix/issues.jsonl failed: %v\n", err)
			} else if migrated {
				fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d beads from .helix/issues.jsonl\n", n)
			}
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
				b.IssueType = v
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
			if setFlags, _ := cmd.Flags().GetStringArray("set"); len(setFlags) > 0 {
				if b.Extra == nil {
					b.Extra = make(map[string]any)
				}
				for _, kv := range setFlags {
					k, v, ok := strings.Cut(kv, "=")
					if !ok {
						return fmt.Errorf("--set requires key=value format, got: %s", kv)
					}
					switch v {
					case "true":
						b.Extra[k] = true
					case "false":
						b.Extra[k] = false
					default:
						b.Extra[k] = v
					}
				}
			}

			if err := s.Create(b); err != nil {
				return err
			}
			f.beadAutoCommit("create " + b.ID)
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
	cmd.Flags().StringArray("set", nil, "Set custom field (key=value, repeatable)")

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
				data, err := bead.MarshalBead(*b)
				if err != nil {
					return err
				}
				var pretty json.RawMessage = data
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(pretty)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "ID:       %s\n", b.ID)
			fmt.Fprintf(out, "Title:    %s\n", b.Title)
			fmt.Fprintf(out, "Type:     %s\n", b.IssueType)
			fmt.Fprintf(out, "Status:   %s\n", b.Status)
			fmt.Fprintf(out, "Priority: %d\n", b.Priority)
			if len(b.Labels) > 0 {
				fmt.Fprintf(out, "Labels:   %s\n", strings.Join(b.Labels, ", "))
			}
			if b.Owner != "" {
				fmt.Fprintf(out, "Owner:    %s\n", b.Owner)
			}
			if b.Parent != "" {
				fmt.Fprintf(out, "Parent:   %s\n", b.Parent)
			}
			if len(b.Dependencies) > 0 {
				fmt.Fprintf(out, "Deps:     %s\n", strings.Join(b.DepIDs(), ", "))
			}
			if b.Description != "" {
				fmt.Fprintf(out, "Desc:     %s\n", b.Description)
			}
			if b.Acceptance != "" {
				fmt.Fprintf(out, "Accept:   %s\n", b.Acceptance)
			}
			fmt.Fprintf(out, "Created:  %s\n", b.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(out, "Updated:  %s\n", b.UpdatedAt.Format("2006-01-02 15:04:05"))
			// Show agent session evidence if present
			if b.Extra != nil {
				if sessionID, ok := b.Extra["session_id"]; ok && sessionID != "" {
					fmt.Fprintf(out, "Session:  %v\n", sessionID)
					// Try to resolve session details
					if sess := f.resolveAgentSession(fmt.Sprint(sessionID)); sess != nil {
						fmt.Fprintf(out, "Harness:  %s\n", sess.Harness)
						if sess.Model != "" {
							fmt.Fprintf(out, "Model:    %s\n", sess.Model)
						}
						if sess.Tokens > 0 {
							fmt.Fprintf(out, "Tokens:   %d (in: %d, out: %d)\n", sess.Tokens, sess.InputTokens, sess.OutputTokens)
						}
						if sess.CostUSD > 0 {
							fmt.Fprintf(out, "Cost:     $%.4f\n", sess.CostUSD)
						}
						if sess.Duration > 0 {
							fmt.Fprintf(out, "Duration: %dms\n", sess.Duration)
						}
					}
				}
				if commitSHA, ok := b.Extra["closing_commit_sha"]; ok && commitSHA != "" {
					fmt.Fprintf(out, "Commit:   %v\n", commitSHA)
				}
				if v, ok := b.Extra["claimed-at"]; ok {
					fmt.Fprintf(out, "Claimed:  %v\n", v)
				}
				if v, ok := b.Extra["claimed-machine"]; ok {
					fmt.Fprintf(out, "Machine:  %v\n", v)
				}
				if v, ok := b.Extra["claimed-session"]; ok {
					fmt.Fprintf(out, "Session:  %v\n", v)
				}
				if v, ok := b.Extra["claimed-worktree"]; ok {
					fmt.Fprintf(out, "Worktree: %v\n", v)
				}
			}
			claimKeys := map[string]bool{
				"claimed-at": true, "claimed-pid": true,
				"claimed-machine": true, "claimed-session": true, "claimed-worktree": true,
				"session_id": true, "closing_commit_sha": true,
			}
			for k, v := range b.Extra {
				if !claimKeys[k] {
					fmt.Fprintf(out, "%s: %v\n", k, v)
				}
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

			// --claim and --unclaim use dedicated store methods
			if claim, _ := cmd.Flags().GetBool("claim"); claim {
				assignee, _ := cmd.Flags().GetString("assignee")
				if assignee == "" {
					assignee = resolveClaimAssignee()
				}
				if err := s.Claim(args[0], assignee); err != nil {
					return err
				}
				f.beadAutoCommit("claim " + args[0])
				return nil
			}
			if unclaim, _ := cmd.Flags().GetBool("unclaim"); unclaim {
				if err := s.Unclaim(args[0]); err != nil {
					return err
				}
				f.beadAutoCommit("unclaim " + args[0])
				return nil
			}

			if err := s.Update(args[0], func(b *bead.Bead) {
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
					b.Owner = v
				}
				if v, _ := cmd.Flags().GetString("parent"); cmd.Flags().Changed("parent") {
					b.Parent = v
				}
				if v, _ := cmd.Flags().GetString("description"); cmd.Flags().Changed("description") {
					b.Description = v
				}
				if v, _ := cmd.Flags().GetString("notes"); cmd.Flags().Changed("notes") {
					b.Notes = v
				}
				if setFlags, _ := cmd.Flags().GetStringArray("set"); len(setFlags) > 0 {
					if b.Extra == nil {
						b.Extra = make(map[string]any)
					}
					for _, kv := range setFlags {
						k, v, ok := strings.Cut(kv, "=")
						if !ok {
							continue
						}
						// Route known field names to struct fields
						switch k {
						case "parent":
							b.Parent = v
						case "description":
							b.Description = v
						case "notes":
							b.Notes = v
						case "acceptance":
							b.Acceptance = v
						case "issue_type":
							b.IssueType = v
						default:
							// Parse booleans and numbers for proper typing
							switch v {
							case "true":
								b.Extra[k] = true
							case "false":
								b.Extra[k] = false
							default:
								b.Extra[k] = v
							}
						}
					}
				}
			}); err != nil {
				return err
			}
			f.beadAutoCommit("update " + args[0])
			return nil
		},
	}

	cmd.Flags().String("title", "", "New title")
	cmd.Flags().String("status", "", "New status (open, in_progress, closed)")
	cmd.Flags().Int("priority", 0, "New priority")
	cmd.Flags().String("labels", "", "New labels (comma-separated)")
	cmd.Flags().String("acceptance", "", "New acceptance criteria")
	cmd.Flags().String("assignee", "", "New assignee or claim assignee fallback")
	cmd.Flags().String("parent", "", "New parent bead ID")
	cmd.Flags().String("description", "", "New description")
	cmd.Flags().String("notes", "", "New notes")
	cmd.Flags().Bool("claim", false, "Claim: set status=in_progress, assignee=ddx")
	cmd.Flags().Bool("unclaim", false, "Unclaim: set status=open, clear assignee and claim fields")
	cmd.Flags().StringArray("set", nil, "Set custom field (key=value, repeatable)")

	return cmd
}

func resolveClaimAssignee() string {
	for _, key := range []string{"DDX_AGENT_NAME", "USER", "LOGNAME", "USERNAME", "SUDO_USER"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "ddx"
}

func (f *CommandFactory) newBeadEvidenceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Manage append-only execution evidence",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add <id>",
		Short: "Append execution evidence to a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, _ := cmd.Flags().GetString("kind")
			summary, _ := cmd.Flags().GetString("summary")
			body, _ := cmd.Flags().GetString("body")
			source, _ := cmd.Flags().GetString("source")
			actor, _ := cmd.Flags().GetString("actor")
			if actor == "" {
				actor = resolveClaimAssignee()
			}
			if kind == "" {
				kind = "summary"
			}
			if source == "" {
				source = "ddx bead evidence add"
			}
			return f.beadStore().AppendEvent(args[0], bead.BeadEvent{
				Kind:    kind,
				Summary: summary,
				Body:    body,
				Actor:   actor,
				Source:  source,
			})
		},
	})
	addCmd := cmd.Commands()[0]
	addCmd.Flags().String("kind", "summary", "Evidence kind")
	addCmd.Flags().String("summary", "", "Short summary")
	addCmd.Flags().String("body", "", "Detailed body")
	addCmd.Flags().String("source", "", "Evidence source")
	addCmd.Flags().String("actor", "", "Actor identity")

	cmd.AddCommand(&cobra.Command{
		Use:   "list <id>",
		Short: "List execution evidence for a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := f.beadStore().Events(args[0])
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(events)
			}

			for _, e := range events {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", e.CreatedAt.Format(time.RFC3339), e.Kind, e.Summary)
			}
			return nil
		},
	})
	listCmd := cmd.Commands()[1]
	listCmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func (f *CommandFactory) newBeadCloseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, _ := cmd.Flags().GetString("session")
			commitSHA, _ := cmd.Flags().GetString("commit")

			if err := f.beadStore().CloseWithEvidence(args[0], sessionID, commitSHA); err != nil {
				return err
			}
			f.beadAutoCommit("close " + args[0])
			return nil
		},
	}
	cmd.Flags().String("session", "", "Agent session ID that completed this bead")
	cmd.Flags().String("commit", "", "Closing commit SHA (auto-detected if not provided)")
	return cmd
}

func (f *CommandFactory) newBeadListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List beads",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			status, _ := cmd.Flags().GetString("status")
			label, _ := cmd.Flags().GetString("label")
			asJSON, _ := cmd.Flags().GetBool("json")
			whereSlice, _ := cmd.Flags().GetStringArray("where")

			where := map[string]string{}
			for _, kv := range whereSlice {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) == 2 {
					where[parts[0]] = parts[1]
				}
			}

			beads, err := s.List(status, label, where)
			if err != nil {
				return err
			}
			if beads == nil {
				beads = []bead.Bead{}
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
	cmd.Flags().StringArray("where", nil, "Filter by field value (key=value); may be repeated")

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
			if beads == nil {
				beads = []bead.Bead{}
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
			if beads == nil {
				beads = []bead.Bead{}
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
					b.ID, b.Priority, b.Title, strings.Join(b.DepIDs(), ", "))
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
			if err := f.beadStore().DepAdd(args[0], args[1]); err != nil {
				return err
			}
			f.beadAutoCommit("dep-add " + args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <id> <dep-id>",
		Short: "Remove a dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.beadStore().DepRemove(args[0], args[1]); err != nil {
				return err
			}
			f.beadAutoCommit("dep-remove " + args[0])
			return nil
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
			if n > 0 {
				f.beadAutoCommit(fmt.Sprintf("import %d beads", n))
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
