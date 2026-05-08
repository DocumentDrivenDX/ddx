package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			serverreg.TryRegisterAsync(f.WorkingDir)
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newBeadInitCommand())
	cmd.AddCommand(f.newBeadCreateCommand())
	cmd.AddCommand(f.newBeadShowCommand())
	cmd.AddCommand(f.newBeadUpdateCommand())
	cmd.AddCommand(f.newBeadCloseCommand())
	cmd.AddCommand(f.newBeadReopenCommand())
	cmd.AddCommand(f.newBeadListCommand())
	cmd.AddCommand(f.newBeadReadyCommand())
	cmd.AddCommand(f.newBeadBlockedCommand())
	cmd.AddCommand(f.newBeadStatusCommand())
	cmd.AddCommand(f.newBeadDepCommand())
	cmd.AddCommand(f.newBeadEvidenceCommand())
	cmd.AddCommand(f.newBeadRoutingCommand())
	cmd.AddCommand(f.newBeadImportCommand())
	cmd.AddCommand(f.newBeadExportCommand())
	cmd.AddCommand(f.newBeadReviewCommand())
	cmd.AddCommand(f.newBeadMetricsCommand())
	cmd.AddCommand(f.newBeadDoctorCommand())

	return cmd
}

// beadAutoCommit commits .ddx/beads.jsonl if git.auto_commit is "always".
// The operation string describes what happened (e.g. "create ddx-abc123").
// Errors are silently ignored — auto-commit is best-effort.
// When a commit lands, the resulting SHA is returned.
func (f *CommandFactory) beadAutoCommit(operation string) string {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}

	cfg, err := config.LoadWithWorkingDir(workspaceRoot)
	if err != nil {
		return ""
	}
	if cfg.Git == nil {
		return ""
	}
	acCfg := gitpkg.AutoCommitConfig{
		AutoCommit:   cfg.Git.AutoCommit,
		CommitPrefix: cfg.Git.CommitPrefix,
	}
	beadsFile := filepath.Join(workspaceRoot, ".ddx", "beads.jsonl")
	sha, _ := gitpkg.AutoCommit(beadsFile, "beads", operation, acCfg)
	return sha
}

func (f *CommandFactory) resolveCommitSHA(commitSHA string) (string, error) {
	if commitSHA == "" {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repoDir := f.WorkingDir
	if repoDir == "" {
		repoDir = "."
	}

	if isFullCommitSHA(commitSHA) && !gitpkg.IsRepository(repoDir) {
		return commitSHA, nil
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--verify", commitSHA+"^{commit}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", commitSHA, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func isFullCommitSHA(commitSHA string) bool {
	if len(commitSHA) != 40 {
		return false
	}
	for i := 0; i < len(commitSHA); i++ {
		c := commitSHA[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c >= 'a' && c <= 'f' {
			continue
		}
		if c >= 'A' && c <= 'F' {
			continue
		}
		return false
	}
	return true
}

func (f *CommandFactory) resolveClosingCommitSHA(commitSHA string) (string, error) {
	normalizedCommitSHA, err := f.resolveCommitSHA(commitSHA)
	if err != nil {
		return "", fmt.Errorf("invalid closing_commit_sha %q: %w", commitSHA, err)
	}
	if normalizedCommitSHA == "" {
		return "", fmt.Errorf("invalid closing_commit_sha %q: empty value", commitSHA)
	}
	return normalizedCommitSHA, nil
}

// commitIsMetadataOnlyTrackerBackfill reports whether the given commit changed
// only bead tracker state. Closing provenance is suppressed only for pure
// tracker backfills that touch .ddx/beads.jsonl and nothing else.
func (f *CommandFactory) commitIsMetadataOnlyTrackerBackfill(commitSHA string) bool {
	if commitSHA == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "show", "--pretty=format:", "--name-only", commitSHA)
	cmd.Dir = f.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(out), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if path != ".ddx/beads.jsonl" {
			return false
		}
	}

	return true
}

func isReviewCloseBead(b *bead.Bead) bool {
	if b == nil {
		return false
	}
	for _, label := range b.Labels {
		switch label {
		case "action:review", "kind:review", "phase:review", "review-finding":
			return true
		}
	}
	return false
}

func (f *CommandFactory) beadWorkspaceRoot() string {
	dir := os.Getenv("DDX_BEAD_DIR")
	if dir != "" {
		if filepath.Base(dir) == ".ddx" {
			return filepath.Dir(dir)
		}
		return dir
	}
	if f.WorkingDir == "" {
		return ""
	}
	if workspaceRoot := gitpkg.FindNearestDDxWorkspace(f.WorkingDir); workspaceRoot != "" {
		return workspaceRoot
	}
	return f.WorkingDir
}

func (f *CommandFactory) beadStore() *bead.Store {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		return bead.NewStore("")
	}
	return bead.NewStore(filepath.Join(workspaceRoot, ".ddx"))
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
				var obj map[string]any
				if err := json.Unmarshal(data, &obj); err != nil {
					return err
				}
				workspaceRoot := f.beadWorkspaceRoot()
				if workspaceRoot == "" {
					workspaceRoot = f.WorkingDir
				}
				metrics, err := beadMetricsFor(workspaceRoot, b.ID)
				if err != nil {
					return err
				}
				if metrics == nil {
					metrics = &beadMetricsSummary{}
				}
				obj["metrics"] = metrics
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(obj)
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

			if unsetFlags, _ := cmd.Flags().GetStringArray("unset"); len(unsetFlags) > 0 {
				for _, key := range unsetFlags {
					if isProtectedBeadExtraKey(key) {
						return fmt.Errorf("cannot unset protected bead field: %s", key)
					}
				}
			}

			var setFlags []string
			if rawSetFlags, _ := cmd.Flags().GetStringArray("set"); len(rawSetFlags) > 0 {
				setFlags = make([]string, 0, len(rawSetFlags))
				for _, kv := range rawSetFlags {
					k, v, ok := strings.Cut(kv, "=")
					if !ok {
						return fmt.Errorf("--set requires key=value format, got: %s", kv)
					}
					if k == "closing_commit_sha" {
						normalizedCommitSHA, err := f.resolveClosingCommitSHA(v)
						if err != nil {
							return err
						}
						v = normalizedCommitSHA
					}
					setFlags = append(setFlags, k+"="+v)
				}
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
				if len(setFlags) > 0 {
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
				if unsetFlags, _ := cmd.Flags().GetStringArray("unset"); len(unsetFlags) > 0 {
					for _, key := range unsetFlags {
						if b.Extra != nil {
							delete(b.Extra, key)
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
	cmd.Flags().StringArray("unset", nil, "Unset custom field (key repeatable)")

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

func isProtectedBeadExtraKey(key string) bool {
	switch key {
	case "events", "session_id", "claimed-at", "claimed-pid", "claimed-machine", "claimed-session", "claimed-worktree":
		return true
	default:
		return false
	}
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
			s := f.beadStore()
			sessionID, _ := cmd.Flags().GetString("session")
			commitSHA, _ := cmd.Flags().GetString("commit")

			if commitSHA != "" {
				normalizedCommitSHA, err := f.resolveCommitSHA(commitSHA)
				if err != nil {
					return err
				}
				commitSHA = normalizedCommitSHA
			}

			target, err := s.Get(args[0])
			if err != nil {
				return err
			}
			// CloseWithEvidence runs the closure gate (ddx-e30e60a9); manual
			// operator closes without evidence are intentionally a separate
			// path. When --session and --commit are both unset, we are in
			// manual-administration territory — use the ungated Store.Close
			// so the gate doesn't reject legitimate tracker admin.
			if sessionID == "" && commitSHA == "" {
				if err := s.Close(args[0]); err != nil {
					return err
				}
			} else if err := s.CloseWithEvidence(args[0], sessionID, commitSHA); err != nil {
				return err
			}

			landedSHA := f.beadAutoCommit("close " + args[0])
			if commitSHA == "" && landedSHA != "" {
				if f.commitIsMetadataOnlyTrackerBackfill(landedSHA) {
					if isReviewCloseBead(target) {
						// Tracker-only review-finding closes must not retain any prior
						// replay boundary. The close commit itself is the metadata-only
						// backfill, so clear stale closing provenance instead of
						// preserving an unrelated implementation SHA.
						if err := s.Update(args[0], func(b *bead.Bead) {
							if b.Extra == nil {
								return
							}
							delete(b.Extra, "closing_commit_sha")
						}); err != nil {
							return err
						}
						if followupSHA := f.beadAutoCommit("close " + args[0]); followupSHA == "" {
							return fmt.Errorf("close %s: failed to auto-commit closing provenance", args[0])
						}
					}
				} else {
					// Only stamp closing provenance when the close commit includes
					// real implementation work. Pure tracker backfills should not
					// advertise a replay boundary that points at metadata-only
					// provenance.
					if err := s.Update(args[0], func(b *bead.Bead) {
						if b.Extra == nil {
							b.Extra = make(map[string]any)
						}
						b.Extra["closing_commit_sha"] = landedSHA
					}); err != nil {
						return err
					}
					if followupSHA := f.beadAutoCommit("close " + args[0]); followupSHA == "" {
						return fmt.Errorf("close %s: failed to auto-commit closing provenance", args[0])
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().String("session", "", "Agent session ID that completed this bead")
	cmd.Flags().String("commit", "", "Closing commit SHA (auto-detected if not provided)")
	return cmd
}

func (f *CommandFactory) newBeadReopenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a closed bead",
		Long: `Reopen a closed or stalled bead.

Atomically sets status to open, clears claim fields, optionally appends
notes, and records a reopen event in the bead's event log.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			reason, _ := cmd.Flags().GetString("reason")
			appendNotes, _ := cmd.Flags().GetString("append-notes")
			if err := s.Reopen(args[0], reason, appendNotes); err != nil {
				return err
			}
			f.beadAutoCommit("reopen " + args[0])
			return nil
		},
	}
	cmd.Flags().String("reason", "", "Reason for reopening (recorded as event summary)")
	cmd.Flags().String("append-notes", "", "Text to append to the bead's notes field")
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
		Long: `List beads whose dependencies are all closed, sorted by priority.

By default this is the dependency-ready set: any open bead with no blocking
deps, including epics and beads on retry cooldown. 'ddx work' operates on a
narrower "execution-ready" subset — use --execution to see exactly what
'ddx work' would pick from, or the reverse: if 'ddx bead ready' shows work
but 'ddx work' reports none, the diff is epics, cooldown-waiting beads,
beads with execution-eligible=false, and superseded beads.`,
		Args: cobra.NoArgs,
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
	cmd.Flags().Bool("execution", false, "Filter to the execution-ready subset (what ddx work picks from): open, deps-closed, not an epic, execution-eligible, not superseded, not on retry cooldown")
	return cmd
}

func (f *CommandFactory) newBeadBlockedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocked",
		Short: "List beads blocked by deps or retry cooldowns",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			entries, err := s.BlockedAll()
			if err != nil {
				return err
			}
			if entries == nil {
				entries = []bead.BlockedBead{}
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No blocked beads.")
				return nil
			}

			for _, e := range entries {
				switch e.Blocker.Kind {
				case bead.BlockerKindRetryCooldown:
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  retry-after: %s\n",
						e.ID, e.Priority, e.Title, e.Blocker.NextEligibleAt)
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  deps: %s\n",
						e.ID, e.Priority, e.Title, strings.Join(e.DepIDs(), ", "))
				}
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
