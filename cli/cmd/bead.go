package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/activework"
	agentpkg "github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/serverreg"
	"github.com/spf13/cobra"
)

// BeadStatusReport is the JSON payload emitted by `ddx bead status --json`.
// It preserves the lifecycle counts while exposing the reconciled active-work
// snapshot separately.
type BeadStatusReport struct {
	Total             int                 `json:"total"`
	Open              int                 `json:"open"`
	InProgress        int                 `json:"in_progress"`
	Closed            int                 `json:"closed"`
	Blocked           int                 `json:"blocked"`
	Proposed          int                 `json:"proposed"`
	Cancelled         int                 `json:"cancelled"`
	NeedsHuman        int                 `json:"needs_human"`
	Ready             int                 `json:"ready"`
	WorkerReady       int                 `json:"worker_ready"`
	DependencyWaiting int                 `json:"dependency_waiting"`
	ExternalBlocked   int                 `json:"external_blocked"`
	OperatorAttention int                 `json:"operator_attention"`
	ActiveWork        activework.Snapshot `json:"active_work"`
}

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
	cmd.AddCommand(f.newBeadNeedsHumanCommand())
	cmd.AddCommand(f.newBeadOperatorAttentionCommand())
	cmd.AddCommand(f.newBeadHumanCommand())
	cmd.AddCommand(f.newBeadBlockedCommand())
	cmd.AddCommand(f.newBeadStatusCommand())
	cmd.AddCommand(f.newBeadDepCommand())
	cmd.AddCommand(f.newBeadQueueCommand())
	cmd.AddCommand(f.newBeadEvidenceCommand())
	cmd.AddCommand(f.newBeadRoutingCommand())
	cmd.AddCommand(f.newBeadImportCommand())
	cmd.AddCommand(f.newBeadExportCommand())
	cmd.AddCommand(f.newBeadMergeCommand())
	cmd.AddCommand(f.newBeadReviewCommand())
	cmd.AddCommand(f.newBeadMetricsCommand())
	cmd.AddCommand(f.newBeadDoctorCommand())
	cmd.AddCommand(f.newBeadCooldownCommand())
	cmd.AddCommand(f.newBeadClearCooldownCommand())
	cmd.AddCommand(f.newBeadReconcileCommand())
	cmd.AddCommand(f.newBeadReconcileAttachmentsCommand())
	cmd.AddCommand(f.newBeadMigrateCommand())
	cmd.AddCommand(f.newBeadArchiveCommand())
	cmd.AddCommand(f.newBeadAcCheckCommand())
	cmd.AddCommand(f.newBeadLintCommand())
	cmd.AddCommand(f.newBeadValidateReadyCommand())
	cmd.AddCommand(f.newBeadReplayCommand())
	cmd.AddCommand(f.newBeadReplayBenchCommand())
	cmd.AddCommand(f.newBeadReapCommand())

	return cmd
}

// beadAutoCommit commits .ddx/beads.jsonl when configured by git.auto_commit.
// The operation string describes what happened (e.g. "create ddx-abc123").
// When a commit lands, the resulting SHA is returned.
func (f *CommandFactory) beadAutoCommit(operation string) (string, error) {
	return f.beadAutoCommitWithMode(operation, false)
}

func (f *CommandFactory) beadAutoCommitPaths(operation string, paths []string) (string, error) {
	return f.beadAutoCommitPathsWithMode(operation, paths, false)
}

func (f *CommandFactory) beadAutoCommitIncludingStaged(operation string) (string, error) {
	return f.beadAutoCommitWithMode(operation, true)
}

func (f *CommandFactory) beadExternalizeArchiveAutoCommit(stats bead.MigrateStats) (string, error) {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}
	stateRoot := ddxroot.JoinProject(workspaceRoot)
	activeFile := filepath.Join(stateRoot, "beads.jsonl")
	archiveFile, _ := bead.DefaultRegistry().Resolve(bead.BeadsArchiveCollection).PathsUnder(stateRoot)

	paths := []string{activeFile}
	if stats.Archived > 0 {
		paths = append(paths, archiveFile)
	}
	if stats.AttachmentsTouched > 0 {
		paths = append(paths, filepath.Join(stateRoot, "attachments"))
	}

	operation := fmt.Sprintf(
		"externalize and archive (%d events to %d attachments, %d beads archived)",
		stats.EventRecordsExternalized,
		stats.AttachmentsTouched,
		stats.Archived,
	)
	return f.beadAutoCommitPaths(operation, paths)
}

func (f *CommandFactory) beadAutoCommitWithMode(operation string, includeStaged bool) (string, error) {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}
	beadsFile := ddxroot.JoinProject(workspaceRoot, "beads.jsonl")
	return f.beadAutoCommitPathsWithMode(operation, []string{beadsFile}, includeStaged)
}

func (f *CommandFactory) beadAutoCommitPathsWithMode(operation string, paths []string, includeStaged bool) (string, error) {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}

	cfg, err := config.LoadWithWorkingDir(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("load config for bead auto-commit: %w", err)
	}
	if cfg.Git == nil {
		return "", nil
	}
	acCfg := gitpkg.AutoCommitConfig{
		AutoCommit:    cfg.Git.AutoCommit,
		CommitPrefix:  cfg.Git.CommitPrefix,
		IncludeStaged: includeStaged,
	}
	sha, err := bead.AutoCommitFilesWithRecovery(paths, "beads", operation, acCfg)
	if err != nil {
		return "", fmt.Errorf("auto-commit beads tracker after %s: %w", operation, err)
	}
	return sha, nil
}

func (f *CommandFactory) withBeadTrackerWriteLock(fn func() error) error {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}
	return agentpkg.WithMainGitLock(workspaceRoot, "cmd_bead_write", fn)
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

	cmd := gitpkg.Command(ctx, repoDir, "rev-parse", "--verify", commitSHA+"^{commit}")
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

	cmd := gitpkg.Command(ctx, f.WorkingDir, "show", "--pretty=format:", "--name-only", commitSHA)
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
			if !filepath.IsAbs(dir) && f.WorkingDir != "" {
				if workspaceRoot := gitpkg.FindNearestDDxWorkspace(f.WorkingDir); workspaceRoot != "" {
					return workspaceRoot
				}
				dir = filepath.Join(f.WorkingDir, dir)
			}
			return filepath.Dir(dir)
		}
		if !filepath.IsAbs(dir) && f.WorkingDir != "" {
			return filepath.Join(f.WorkingDir, dir)
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
	return bead.NewStore(resolveBeadStoreRoot(workspaceRoot))
}

func (f *CommandFactory) beadStatusStore(projectRoot string) *bead.Store {
	inTree := filepath.Join(projectRoot, ddxroot.DirName)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return bead.NewStore(inTree)
	}
	return f.beadStore()
}

func (f *CommandFactory) newBeadInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize bead storage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			if err := s.Init(context.Background()); err != nil {
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
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
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
			if dependsOn, _ := cmd.Flags().GetStringSlice("depends-on"); len(dependsOn) > 0 {
				normalized := make([]string, 0, len(dependsOn))
				for _, depID := range dependsOn {
					if depID = strings.TrimSpace(depID); depID != "" {
						normalized = append(normalized, depID)
					}
				}
				if len(normalized) > 0 {
					if b.ID == "" {
						id, err := s.GenID(context.Background())
						if err != nil {
							return err
						}
						b.ID = id
					}
					for _, depID := range normalized {
						b.AddDep(depID, "blocks")
					}
				}
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

			if err := f.withBeadTrackerWriteLock(func() error {
				if err := s.Create(context.Background(), b); err != nil {
					return err
				}
				if markerPresent, _ := s.HasLifecycleSchemaMarker(); !markerPresent && !beadHasLegacyLifecycleInputs(*b) {
					if err := s.WriteLifecycleSchemaMarker(time.Now().UTC()); err != nil {
						return err
					}
				}
				if _, err := f.beadAutoCommitPaths("create "+b.ID, []string{s.File, s.LifecycleSchemaMarkerPath()}); err != nil {
					return err
				}
				return nil
			}); err != nil {
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
	cmd.Flags().StringSlice("depends-on", nil, "Dependency bead ID (comma-separated or repeated)")
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
			b, err := s.GetWithArchive(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				// Externalized events live in a sidecar; inline them for the
				// JSON view so consumers (and the `events` UI projection)
				// see a single uniform shape.
				if err := s.LoadEventsInline(b); err != nil {
					return err
				}
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
				if lease, found, err := s.ClaimLease(b.ID); err != nil {
					return err
				} else if found {
					mergeClaimLeaseIntoJSON(obj, b, lease)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(obj)
			}

			out := cmd.OutOrStdout()
			lease, leaseFound, err := s.ClaimLease(b.ID)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "ID:       %s\n", b.ID)
			fmt.Fprintf(out, "Title:    %s\n", b.Title)
			fmt.Fprintf(out, "Type:     %s\n", b.IssueType)
			fmt.Fprintf(out, "Status:   %s\n", b.Status)
			fmt.Fprintf(out, "Priority: %d\n", b.Priority)
			if len(b.Labels) > 0 {
				fmt.Fprintf(out, "Labels:   %s\n", strings.Join(b.Labels, ", "))
			}
			if owner := strings.TrimSpace(b.Owner); owner != "" {
				fmt.Fprintf(out, "Owner:    %s\n", owner)
			} else if leaseFound && strings.TrimSpace(lease.Owner) != "" {
				fmt.Fprintf(out, "Owner:    %s\n", lease.Owner)
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
				} else if leaseFound && !lease.StartedAt.IsZero() {
					fmt.Fprintf(out, "Claimed:  %s\n", lease.StartedAt.Format(time.RFC3339))
				}
				if v, ok := b.Extra["claimed-machine"]; ok {
					fmt.Fprintf(out, "Machine:  %v\n", v)
				} else if leaseFound && lease.Machine != "" {
					fmt.Fprintf(out, "Machine:  %s\n", lease.Machine)
				}
				if v, ok := b.Extra["claimed-session"]; ok {
					fmt.Fprintf(out, "Session:  %v\n", v)
				} else if leaseFound && lease.Session != "" {
					fmt.Fprintf(out, "Session:  %s\n", lease.Session)
				}
				if v, ok := b.Extra["claimed-worktree"]; ok {
					fmt.Fprintf(out, "Worktree: %v\n", v)
				} else if leaseFound && lease.Worktree != "" {
					fmt.Fprintf(out, "Worktree: %s\n", lease.Worktree)
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

			// Show intake.blocked events with structured fields
			events, err := s.Events(args[0])
			if err == nil && len(events) > 0 {
				var intakeBlockedEvents []bead.BeadEvent
				for _, ev := range events {
					if ev.Kind == "intake.blocked" || ev.Kind == "pre_claim_intake.blocked" {
						intakeBlockedEvents = append(intakeBlockedEvents, ev)
					}
				}
				if len(intakeBlockedEvents) > 0 {
					fmt.Fprintf(out, "IntakeBlocked:\n")
					for _, ev := range intakeBlockedEvents {
						var body map[string]any
						if err := json.Unmarshal([]byte(ev.Body), &body); err != nil {
							fmt.Fprintf(out, "  - (parse error: %v)\n", err)
							continue
						}
						ruleFp := ""
						if fp, ok := body["rule_fingerprint"].(string); ok {
							ruleFp = fp
						}
						operatorOverride := false
						if override, ok := body["operator_override"].(bool); ok {
							operatorOverride = override
						}
						fmt.Fprintf(out, "  - %s | outcome: %s | fingerprint: %s | override: %v\n",
							ev.CreatedAt.Format("2006-01-02 15:04:05"), ev.Summary, ruleFp, operatorOverride)
					}
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
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
			s := f.beadStore()

			// --claim and --unclaim use dedicated store methods
			if claim, _ := cmd.Flags().GetBool("claim"); claim {
				assignee, _ := cmd.Flags().GetString("assignee")
				if assignee == "" {
					assignee = resolveClaimAssignee()
				}
				if err := f.withBeadTrackerWriteLock(func() error {
					if err := s.Claim(args[0], assignee); err != nil {
						return err
					}
					if _, err := f.beadAutoCommit("claim " + args[0]); err != nil {
						return err
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			}
			if unclaim, _ := cmd.Flags().GetBool("unclaim"); unclaim {
				if err := f.withBeadTrackerWriteLock(func() error {
					if err := s.Unclaim(args[0]); err != nil {
						return err
					}
					if _, err := f.beadAutoCommit("unclaim " + args[0]); err != nil {
						return err
					}
					return nil
				}); err != nil {
					return err
				}
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

			statusValue, _ := cmd.Flags().GetString("status")
			statusChanged := cmd.Flags().Changed("status")
			statusOpts := beadTransitionOptionsFromSetFlags(statusValue, setFlags, "ddx bead update")

			applyUpdateFields := func(b *bead.Bead) error {
				if v, _ := cmd.Flags().GetString("title"); cmd.Flags().Changed("title") {
					b.Title = v
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
				return nil
			}

			if err := f.withBeadTrackerWriteLock(func() error {
				var err error
				if statusChanged {
					err = s.UpdateWithLifecycleStatus(args[0], statusValue, statusOpts, applyUpdateFields)
				} else {
					err = s.Update(context.Background(), args[0], func(b *bead.Bead) {
						_ = applyUpdateFields(b)
					})
				}
				if err != nil {
					return err
				}
				if _, err := f.beadAutoCommit("update " + args[0]); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().String("title", "", "New title")
	cmd.Flags().String("status", "", "New lifecycle status (validated transition)")
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

func mergeClaimLeaseIntoJSON(obj map[string]any, b *bead.Bead, lease bead.ClaimLeaseRecord) {
	if obj == nil || b == nil {
		return
	}
	if strings.TrimSpace(b.Owner) == "" && lease.Owner != "" {
		obj["owner"] = lease.Owner
	}
	if _, ok := obj["claimed-at"]; !ok && !lease.StartedAt.IsZero() {
		obj["claimed-at"] = lease.StartedAt.Format(time.RFC3339)
	}
	if _, ok := obj["claimed-pid"]; !ok && lease.PID != 0 {
		obj["claimed-pid"] = lease.PID
	}
	if _, ok := obj["claimed-machine"]; !ok && lease.Machine != "" {
		obj["claimed-machine"] = lease.Machine
	}
	if _, ok := obj["claimed-session"]; !ok && lease.Session != "" {
		obj["claimed-session"] = lease.Session
	}
	if _, ok := obj["claimed-worktree"]; !ok && lease.Worktree != "" {
		obj["claimed-worktree"] = lease.Worktree
	}
}

func isProtectedBeadExtraKey(key string) bool {
	switch key {
	case "events", "session_id", "claimed-at", "claimed-pid", "claimed-machine", "claimed-session", "claimed-worktree":
		return true
	default:
		return false
	}
}

// beadHasLegacyLifecycleInputs detects beads that require lifecycle migration
// before they can be executed. Migration-only: these status values and labels
// are legacy; new rows use canonical statuses and the lifecycle state machine.
func beadHasLegacyLifecycleInputs(b bead.Bead) bool {
	if b.Status == "needs_investigation" || b.Status == "needs_human" {
		return true
	}
	for _, label := range b.Labels {
		if label == bead.LabelNeedsHuman || label == bead.LabelNeedsInvestigation {
			return true
		}
	}
	return false
}

func beadTransitionOptionsFromSetFlags(status string, setFlags []string, source string) bead.LifecycleTransitionOptions {
	opts := bead.LifecycleTransitionOptions{
		Reason: "set lifecycle status",
		Source: source,
	}
	if status == bead.StatusBlocked {
		for _, kv := range setFlags {
			k, v, ok := strings.Cut(kv, "=")
			if ok && k == bead.ExtraLifecycleExternalBlockerReason {
				opts.ExternalBlockerReason = v
				break
			}
		}
	}
	return opts
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
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
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

			return f.withBeadTrackerWriteLock(func() error {
				target, err := s.Get(context.Background(), args[0])
				if err != nil {
					return err
				}
				// CloseWithEvidence runs the closure gate (ddx-e30e60a9); manual
				// operator closes without evidence are intentionally a separate
				// path. When --session and --commit are both unset, we are in
				// manual-administration territory — use the ungated Store.Close
				// so the gate doesn't reject legitimate tracker admin.
				if sessionID == "" && commitSHA == "" {
					if err := s.Close(context.Background(), args[0]); err != nil {
						return err
					}
				} else if err := s.CloseWithEvidence(args[0], sessionID, commitSHA); err != nil {
					return err
				}

				landedSHA, err := f.beadAutoCommitIncludingStaged("close " + args[0])
				if err != nil {
					return err
				}
				if commitSHA == "" && landedSHA != "" {
					if f.commitIsMetadataOnlyTrackerBackfill(landedSHA) {
						if isReviewCloseBead(target) {
							// Tracker-only review-finding closes must not retain any prior
							// replay boundary. The close commit itself is the metadata-only
							// backfill, so clear stale closing provenance instead of
							// preserving an unrelated implementation SHA.
							if err := s.Update(context.Background(), args[0], func(b *bead.Bead) {
								if b.Extra == nil {
									return
								}
								delete(b.Extra, "closing_commit_sha")
							}); err != nil {
								return err
							}
							followupSHA, err := f.beadAutoCommit("close " + args[0])
							if err != nil {
								return err
							}
							if followupSHA == "" {
								return fmt.Errorf("close %s: failed to auto-commit closing provenance", args[0])
							}
						}
					} else {
						// Only stamp closing provenance when the close commit includes
						// real implementation work. Pure tracker backfills should not
						// advertise a replay boundary that points at metadata-only
						// provenance.
						if err := s.Update(context.Background(), args[0], func(b *bead.Bead) {
							if b.Extra == nil {
								b.Extra = make(map[string]any)
							}
							b.Extra["closing_commit_sha"] = landedSHA
						}); err != nil {
							return err
						}
						followupSHA, err := f.beadAutoCommit("close " + args[0])
						if err != nil {
							return err
						}
						if followupSHA == "" {
							return fmt.Errorf("close %s: failed to auto-commit closing provenance", args[0])
						}
					}
				}
				return nil
			})
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
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
			s := f.beadStore()
			reason, _ := cmd.Flags().GetString("reason")
			appendNotes, _ := cmd.Flags().GetString("append-notes")
			return f.withBeadTrackerWriteLock(func() error {
				if err := s.Reopen(args[0], reason, appendNotes); err != nil {
					return err
				}
				if _, err := f.beadAutoCommit("reopen " + args[0]); err != nil {
					return err
				}
				return nil
			})
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

			// Always consult the archive partner so `bead list` survives a
			// `bead migrate` — the --all flag is now a no-op kept for
			// compatibility.
			_, _ = cmd.Flags().GetBool("all")
			beads, err := s.ListWithArchive(status, label, where)
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
	cmd.Flags().Bool("all", false, "Include archived beads (.ddx/beads-archive.jsonl)")
	cmd.Flags().StringArray("where", nil, "Filter by field value (key=value); may be repeated")

	return cmd
}

func (f *CommandFactory) newBeadReadyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "List beads ready for work",
		Long: `List beads in the lifecycle-derived ready bucket, sorted by priority.

Ready work has status=open, closed dependencies, and no active cooldown,
external blocker, supersession, ineligible marker, or epic-container exclusion.
Use --execution to include stale in_progress claims that ddx work can reclaim.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
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
	cmd.Flags().Bool("include-human", false, "Deprecated no-op; operator-attention work is status=proposed and excluded from ready output")
	return cmd
}

type beadNeedsHumanRow struct {
	ID              string   `json:"id"`
	Priority        int      `json:"priority"`
	Title           string   `json:"title"`
	Reason          string   `json:"reason,omitempty"`
	Since           string   `json:"since,omitempty"`
	Source          string   `json:"source,omitempty"`
	SuggestedAction string   `json:"suggested_action,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Labels          []string `json:"labels,omitempty"`
}

func (f *CommandFactory) newBeadNeedsHumanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "needs-human",
		Short: "Deprecated alias: list proposed operator-attention beads",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			beads, err := f.beadStore().ProposedOperatorAttention()
			if err != nil {
				return err
			}
			rows := make([]beadNeedsHumanRow, 0, len(beads))
			for _, b := range beads {
				rows = append(rows, needsHumanRow(b))
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No operator-attention beads.")
				return nil
			}
			for _, row := range rows {
				detail := row.Summary
				if detail == "" {
					detail = row.Reason
				}
				if detail == "" {
					detail = row.SuggestedAction
				}
				if detail != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  %s\n", row.ID, row.Priority, row.Title, detail)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s\n", row.ID, row.Priority, row.Title)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// beadOperatorAttentionRow is one wedge/timeout lease release surfaced by
// `ddx bead operator-attention`. When a ddx work worker gives up a held lease
// on a wedge or timeout it emits a durable operator_attention event; this row
// carries the four fields an operator needs to triage it (bead-id, attempt-id,
// last_activity_at, diagnosis) without tailing JSONL agent-logs and noticing
// empty harness/route fields (ddx-58a69bb0, parent ddx-8f2e0ebf criterion D).
type beadOperatorAttentionRow struct {
	BeadID         string `json:"bead_id"`
	AttemptID      string `json:"attempt_id,omitempty"`
	LastActivityAt string `json:"last_activity_at,omitempty"`
	Reason         string `json:"reason"`
	Diagnosis      string `json:"diagnosis,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

// wedgeReleaseReasons is the set of operator_attention event summaries that
// represent a worker releasing a held lease on a wedge or timeout: the
// route-resolution timeout (ddx-d8970a7b), the progress-watchdog fire
// (ddx-dc23f001), and the consecutive-wedge guard (ddx-9714eaac).
var wedgeReleaseReasons = map[string]bool{
	agentpkg.FailureModeRouteResolutionTimeout: true,
	agentpkg.FailureModeProgressWatchdog:       true,
	agentpkg.FailureModeConsecutiveWedge:       true,
}

func (f *CommandFactory) newBeadOperatorAttentionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator-attention",
		Short: "List wedge/timeout lease releases needing operator attention",
		Long: `List every lease release a "ddx work" worker emitted because it gave
up a held lease on a wedge or timeout: a route-resolution timeout, a
progress-watchdog fire, or the consecutive-wedge guard. Each release shows the
bead-id, attempt-id, last_activity_at, and a diagnosis so an operator can
triage it without tailing JSONL agent-logs for empty harness/route fields.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := f.collectOperatorAttentionReleases()
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No wedge/timeout lease releases.")
				return nil
			}
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  attempt=%s  last_activity_at=%s  %s  %s\n",
					row.BeadID, dashIfEmpty(row.AttemptID), dashIfEmpty(row.LastActivityAt), row.Reason, row.Diagnosis)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

// collectOperatorAttentionReleases scans every bead's event stream for the
// operator_attention events emitted by the wedge-release primitives and returns
// them oldest-first.
func (f *CommandFactory) collectOperatorAttentionReleases() ([]beadOperatorAttentionRow, error) {
	s := f.beadStore()
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}
	rows := make([]beadOperatorAttentionRow, 0)
	for _, b := range beads {
		events, err := s.Events(b.ID)
		if err != nil {
			continue
		}
		for _, ev := range events {
			if ev.Kind != "operator_attention" || !wedgeReleaseReasons[ev.Summary] {
				continue
			}
			rows = append(rows, operatorAttentionRowFromEvent(b.ID, ev))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].CreatedAt < rows[j].CreatedAt
	})
	return rows, nil
}

// operatorAttentionRowFromEvent extracts the four triage fields from an
// operator_attention event. The event Summary is the failure mode (reason); the
// JSON body carries bead_id, attempt_id, last_activity_at, and diagnosis when
// the emitting primitive recorded them.
func operatorAttentionRowFromEvent(beadID string, ev bead.BeadEvent) beadOperatorAttentionRow {
	row := beadOperatorAttentionRow{BeadID: beadID, Reason: ev.Summary}
	if !ev.CreatedAt.IsZero() {
		row.CreatedAt = ev.CreatedAt.UTC().Format(time.RFC3339)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(ev.Body), &body); err == nil {
		if v, ok := body["bead_id"].(string); ok && v != "" {
			row.BeadID = v
		}
		if v, ok := body["attempt_id"].(string); ok {
			row.AttemptID = v
		}
		if v, ok := body["last_activity_at"].(string); ok {
			row.LastActivityAt = v
		}
		if v, ok := body["diagnosis"].(string); ok {
			row.Diagnosis = v
		}
	}
	return row
}

func (f *CommandFactory) newBeadHumanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "human",
		Short: "Deprecated alias: resolve proposed operator-attention beads",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	resolveCmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve a proposed operator-attention bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action, _ := cmd.Flags().GetString("action")
			note, _ := cmd.Flags().GetString("note")
			children, _ := cmd.Flags().GetStringSlice("children")
			return f.resolveNeedsHumanBead(args[0], action, note, children)
		},
	}
	resolveCmd.Flags().String("action", "", "Resolution action: retry, split, obsolete, defer")
	resolveCmd.Flags().String("note", "", "Required operator note")
	resolveCmd.Flags().StringSlice("children", nil, "Existing child bead IDs for split resolution (comma-separated or repeated)")
	_ = resolveCmd.MarkFlagRequired("action")
	cmd.AddCommand(resolveCmd)
	return cmd
}

func (f *CommandFactory) resolveNeedsHumanBead(id, action, note string, children []string) error {
	action = strings.TrimSpace(action)
	note = strings.TrimSpace(note)
	if note == "" {
		return fmt.Errorf("--note is required")
	}
	s := f.beadStore()
	return f.withBeadTrackerWriteLock(func() error {
		switch action {
		case "retry":
			if err := s.UpdateWithLifecycleStatus(id, bead.StatusOpen, bead.LifecycleTransitionOptions{
				Reason: "human resolve retry",
				Actor:  resolveClaimAssignee(),
				Source: "ddx bead human resolve",
			}, func(b *bead.Bead) error {
				// Migration-only cleanup: removes LabelNeedsHuman from legacy rows;
				// new rows carry status=proposed rather than this label.
				removeBeadLabel(b, bead.LabelNeedsHuman)
				bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
				return nil
			}); err != nil {
				return err
			}
			if err := appendNeedsHumanResolutionEvent(s, id, action, note, nil); err != nil {
				return err
			}
		case "split":
			children = normalizeChildIDs(children)
			if len(children) == 0 {
				return fmt.Errorf("--children is required when --action split")
			}
			if err := validateSplitChildren(s, id, children); err != nil {
				return err
			}
			for _, childID := range children {
				if err := s.DepAdd(id, childID); err != nil {
					return err
				}
			}
			if err := s.UpdateWithLifecycleStatus(id, bead.StatusOpen, bead.LifecycleTransitionOptions{
				Reason: "human resolve split",
				Actor:  resolveClaimAssignee(),
				Source: "ddx bead human resolve",
			}, func(b *bead.Bead) error {
				// Migration-only cleanup: removes LabelNeedsHuman from legacy rows;
				// new rows carry status=proposed rather than this label.
				removeBeadLabel(b, bead.LabelNeedsHuman)
				bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{})
				return nil
			}); err != nil {
				return err
			}
			if err := appendNeedsHumanResolutionEvent(s, id, action, note, children); err != nil {
				return err
			}
		case "obsolete":
			if err := appendNeedsHumanResolutionEvent(s, id, action, note, nil); err != nil {
				return err
			}
			if err := s.Close(context.Background(), id); err != nil {
				return err
			}
		case "defer":
			if err := appendNeedsHumanResolutionEvent(s, id, action, note, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown --action %q (valid: retry, split, obsolete, defer)", action)
		}
		if _, err := f.beadAutoCommit("human resolve " + id); err != nil {
			return err
		}
		return nil
	})
}

func needsHumanRow(b bead.Bead) beadNeedsHumanRow {
	meta := bead.GetNeedsHumanMeta(b)
	return beadNeedsHumanRow{
		ID:              b.ID,
		Priority:        b.Priority,
		Title:           b.Title,
		Reason:          meta.Reason,
		Since:           meta.Since,
		Source:          meta.Source,
		SuggestedAction: meta.SuggestedAction,
		Summary:         meta.Summary,
		Labels:          append([]string(nil), b.Labels...),
	}
}

func removeBeadLabel(b *bead.Bead, label string) {
	if b == nil {
		return
	}
	labels := b.Labels[:0]
	for _, existing := range b.Labels {
		if existing != label {
			labels = append(labels, existing)
		}
	}
	b.Labels = labels
}

func appendNeedsHumanResolutionEvent(s *bead.Store, id, action, note string, children []string) error {
	body := note
	if len(children) > 0 {
		body = fmt.Sprintf("%s\nchildren: %s", note, strings.Join(children, ", "))
	}
	return s.AppendEvent(id, bead.BeadEvent{
		Kind:    "needs_human_resolved",
		Summary: "action=" + action,
		Body:    body,
		Actor:   resolveClaimAssignee(),
		Source:  "ddx bead human resolve",
	})
}

func normalizeChildIDs(children []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range children {
		for _, part := range strings.Split(raw, ",") {
			child := strings.TrimSpace(part)
			if child == "" || seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
		}
	}
	return out
}

func validateSplitChildren(s *bead.Store, parentID string, children []string) error {
	if _, err := s.Get(context.Background(), parentID); err != nil {
		return err
	}
	for _, childID := range children {
		if childID == parentID {
			return fmt.Errorf("split child cannot be the parent bead: %s", childID)
		}
		if _, err := s.Get(context.Background(), childID); err != nil {
			return fmt.Errorf("split child %s: %w", childID, err)
		}
	}
	return nil
}

func (f *CommandFactory) newBeadBlockedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocked",
		Short: "List externally blocked beads",
		Long: `List beads with status=blocked because of a hard external,
recheckable blocker. Ordinary dependency waits, retry cooldowns, proposed
operator-attention work, and epic/planning buckets are lifecycle-derived queue
state surfaced by "ddx bead status" and "ddx work focus", not by this command.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.checkLifecycleMigrationGate(cmd); err != nil {
				return err
			}
			s := f.beadStore()
			entries, err := s.BlockedAll()
			if err != nil {
				return err
			}
			entries = externalBlockedEntries(entries)
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
				fmt.Fprintln(cmd.OutOrStdout(), "No externally blocked beads.")
				return nil
			}

			for _, e := range entries {
				switch e.Blocker.Kind {
				case bead.BlockerKindRetryCooldown:
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  retry-after: %s\n",
						e.ID, e.Priority, e.Title, e.Blocker.NextEligibleAt)
				case bead.BlockerKindNeedsInvestigation, bead.BlockerKindOperatorAttention, bead.BlockerKindNotEligible, bead.BlockerKindBlockedStatus, bead.BlockerKindSuperseded, bead.BlockerKindEpicOnly:
					fmt.Fprintf(cmd.OutOrStdout(), "%s  P%d  %s  %s: %s\n",
						e.ID, e.Priority, e.Title, e.Blocker.Kind, e.Blocker.Reason)
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

func externalBlockedEntries(entries []bead.BlockedBead) []bead.BlockedBead {
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.Blocker.Kind == bead.BlockerKindBlockedStatus {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (f *CommandFactory) newBeadReconcileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [id ...]",
		Short: "Reconcile stale bead lifecycle metadata",
		Long: `Reconcile stale no_changes lifecycle metadata using supported bead
store mutations. The command is dry-run by default; pass --apply to mutate the
tracker. It never edits .ddx/beads.jsonl directly.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apply, _ := cmd.Flags().GetBool("apply")
			_, _ = cmd.Flags().GetBool("dry-run")
			asJSON, _ := cmd.Flags().GetBool("json")
			s := f.beadStore()
			var (
				plans []bead.ReconcilePlan
				err   error
			)
			runReconcile := func() error {
				plans, err = s.ReconcileLifecycleMetadata(bead.ReconcileOptions{Apply: apply, IDs: args})
				if err != nil {
					return err
				}
				if apply {
					if _, err := f.beadAutoCommit("reconcile lifecycle metadata"); err != nil {
						return err
					}
				}
				return nil
			}
			if apply {
				err = f.withBeadTrackerWriteLock(runReconcile)
			} else {
				err = runReconcile()
			}
			if err != nil {
				return err
			}
			if plans == nil {
				plans = []bead.ReconcilePlan{}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(plans)
			}
			if len(plans) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No lifecycle metadata repairs.")
				return nil
			}
			mode := "would repair"
			if apply {
				mode = "repaired"
			}
			for _, p := range plans {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", p.BeadID, mode, p.Reason)
				if len(p.ClearFields) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  clear: %s\n", strings.Join(p.ClearFields, ", "))
				}
				if len(p.RemoveLabels) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  remove-labels: %s\n", strings.Join(p.RemoveLabels, ", "))
				}
				if len(p.AddLabels) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  add-labels: %s\n", strings.Join(p.AddLabels, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("apply", false, "Apply proposed repairs")
	cmd.Flags().Bool("dry-run", false, "Preview proposed repairs without mutating (default)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newBeadStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show bead counts and active work",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot := f.beadWorkspaceRoot()
			s := f.beadStatusStore(projectRoot)
			counts, err := s.Status()
			if err != nil {
				return err
			}
			activeWork, err := collectActiveWorkSnapshot(projectRoot, s, time.Now().UTC())
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				report := BeadStatusReport{
					Total:             counts.Total,
					Open:              counts.Open,
					InProgress:        counts.InProgress,
					Closed:            counts.Closed,
					Blocked:           counts.Blocked,
					Proposed:          counts.Proposed,
					Cancelled:         counts.Cancelled,
					NeedsHuman:        counts.NeedsHuman,
					Ready:             counts.Ready,
					WorkerReady:       counts.WorkerReady,
					DependencyWaiting: counts.DependencyWaiting,
					ExternalBlocked:   counts.ExternalBlocked,
					OperatorAttention: counts.OperatorAttention,
					ActiveWork:        activeWork,
				}
				return enc.Encode(report)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Total:              %d\n", counts.Total)
			fmt.Fprintf(out, "Open:               %d\n", counts.Open)
			fmt.Fprintf(out, "Lifecycle in progress: %d\n", counts.InProgress)
			fmt.Fprintf(out, "Closed:             %d\n", counts.Closed)
			fmt.Fprintf(out, "Blocked:            %d\n", counts.Blocked)
			fmt.Fprintf(out, "Proposed:           %d\n", counts.Proposed)
			fmt.Fprintf(out, "Cancelled:          %d\n", counts.Cancelled)
			fmt.Fprintf(out, "Active workers:     %d\n", activeWork.Count)
			if len(activeWork.BeadIDs) > 0 {
				fmt.Fprintf(out, "Active bead IDs:    %s\n", strings.Join(activeWork.BeadIDs, ", "))
			}
			fmt.Fprintf(out, "Ready:              %d\n", counts.Ready)
			fmt.Fprintf(out, "Worker ready:       %d\n", counts.WorkerReady)
			fmt.Fprintf(out, "Dependency waiting: %d\n", counts.DependencyWaiting)
			fmt.Fprintf(out, "External blocked:   %d\n", counts.ExternalBlocked)
			fmt.Fprintf(out, "Operator attention: %d\n", counts.OperatorAttention)
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
			return f.withBeadTrackerWriteLock(func() error {
				if err := f.beadStore().DepAdd(args[0], args[1]); err != nil {
					return err
				}
				if _, err := f.beadAutoCommit("dep-add " + args[0]); err != nil {
					return err
				}
				return nil
			})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <id> <dep-id>",
		Short: "Remove a dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return f.withBeadTrackerWriteLock(func() error {
				if err := f.beadStore().DepRemove(args[0], args[1]); err != nil {
					return err
				}
				if _, err := f.beadAutoCommit("dep-remove " + args[0]); err != nil {
					return err
				}
				return nil
			})
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

			n := 0
			if err := f.withBeadTrackerWriteLock(func() error {
				var err error
				n, err = s.Import(from, file)
				if err != nil {
					return err
				}
				if n > 0 {
					if _, err := f.beadAutoCommit(fmt.Sprintf("import %d beads", n)); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
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

func (f *CommandFactory) newBeadMergeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge [path]",
		Short: "Resolve a Git conflict in .ddx/beads.jsonl",
		Long: `Resolve a Git conflict in .ddx/beads.jsonl using DDx tracker rules.

This is the supported escape hatch when Git leaves the bead tracker in an
unmerged state. It reads the base, ours, and theirs versions from the Git
index stages (:1:, :2:, :3:), merges records by bead id, preserves append-only
events and dependency edges, writes the resolved JSONL file, and reports any
scalar fields that required deterministic conflict resolution.

This command is not a general hand-edit workflow for bead tracker data.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ddxroot.JoinRelative("beads.jsonl")
			if len(args) > 0 {
				path = args[0]
			}
			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}
			repoRoot := gitpkg.FindProjectRoot(workspaceRoot)
			relPath, err := filepath.Rel(repoRoot, filepath.Join(workspaceRoot, path))
			if err != nil || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
				relPath = filepath.ToSlash(path)
			}
			relPath = filepath.ToSlash(filepath.Clean(relPath))

			base, err := gitStageBlob(cmd.Context(), repoRoot, 1, relPath)
			if err != nil {
				return err
			}
			ours, err := gitStageBlob(cmd.Context(), repoRoot, 2, relPath)
			if err != nil {
				return err
			}
			theirs, err := gitStageBlob(cmd.Context(), repoRoot, 3, relPath)
			if err != nil {
				return err
			}

			merged, report, err := bead.MergeTrackerConflictJSONL(base, ours, theirs)
			if err != nil {
				return err
			}
			outPath := filepath.Join(repoRoot, relPath)
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return fmt.Errorf("bead merge: mkdir: %w", err)
			}
			if err := os.WriteFile(outPath, merged, 0o644); err != nil {
				return fmt.Errorf("bead merge: write %s: %w", relPath, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Merged %s: %d records (%d ours-only, %d theirs-only, %d same-id merged)\n",
				relPath, report.TotalRecords, report.PreservedOurs, report.PreservedTheirs, report.MergedRecords)
			for _, conflict := range report.ScalarConflicts {
				fmt.Fprintf(cmd.OutOrStdout(), "Resolved scalar conflict: %s.%s chose %s (%s)\n",
					conflict.ID, conflict.Field, conflict.Choice, conflict.Reason)
			}
			return nil
		},
	}
	return cmd
}

func gitStageBlob(ctx context.Context, repoRoot string, stage int, path string) ([]byte, error) {
	spec := fmt.Sprintf(":%d:%s", stage, path)
	out, err := gitpkg.Command(ctx, repoRoot, "show", spec).Output()
	if err != nil {
		return nil, fmt.Errorf("bead merge: read Git stage %d for %s: %w", stage, path, err)
	}
	return out, nil
}

// newBeadCooldownCommand wires `ddx bead cooldown show|clear <id>`.
//
// `cooldown show` prints the bead's current work cooldown fields
// (retry-after, last-status, last-detail) in human or JSON form. `cooldown
// clear` removes those three fields so the bead becomes execution-eligible
// again at the next loop pass. This is the first-class operator-facing
// surface for the underlying `work-retry-after` Extra key — the
// `ddx bead update --set/--unset work-retry-after=...` workflow
// continues to work as a power-user fallback, but operators should reach
// for `cooldown clear` for the common case.
func (f *CommandFactory) newBeadCooldownCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cooldown",
		Short: "Inspect and clear work cooldowns",
		Long: `Inspect and clear the work cooldown that parks a bead from
re-execution. Cooldowns are set automatically by the loop in three cases:

  * no_changes with a vague rationale (short retry, default 6h)
  * push_failed (long park, 365d, requires operator action)
  * declined_needs_decomposition (long park, 365d, requires decomposition)

Use this command instead of editing the magic Extra key directly:

  ddx bead cooldown show <bead-id>     # show retry-after, last-status, last-detail
  ddx bead cooldown clear <bead-id>    # remove cooldown so the bead re-enters the queue
`,
		Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
	}

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show the work cooldown fields for a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			b, err := s.Get(context.Background(), args[0])
			if err != nil {
				return err
			}
			retry, _ := b.Extra["work-retry-after"].(string)
			lastStatus, _ := b.Extra["work-last-status"].(string)
			lastDetail, _ := b.Extra["work-last-detail"].(string)

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					BeadID      string `json:"bead_id"`
					RetryAfter  string `json:"retry_after,omitempty"`
					LastStatus  string `json:"last_status,omitempty"`
					LastDetail  string `json:"last_detail,omitempty"`
					ParkedUntil string `json:"parked_until,omitempty"`
				}{BeadID: b.ID, RetryAfter: retry, LastStatus: lastStatus, LastDetail: lastDetail, ParkedUntil: retry})
			}

			if retry == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: no cooldown set\n", b.ID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bead:        %s\n", b.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "retry_after: %s\n", retry)
			if lastStatus != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "last_status: %s\n", lastStatus)
			}
			if lastDetail != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "last_detail: %s\n", lastDetail)
			}
			return nil
		},
	}
	showCmd.Flags().Bool("json", false, "Output as JSON")
	cmd.AddCommand(showCmd)

	clearCmd := &cobra.Command{
		Use:   "clear <id>",
		Short: "Clear the work cooldown for a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			cleared := false
			if err := f.withBeadTrackerWriteLock(func() error {
				if err := s.Update(context.Background(), args[0], func(b *bead.Bead) {
					if b.Extra == nil {
						return
					}
					for _, key := range []string{
						"work-retry-after",
						"work-last-status",
						"work-last-detail",
					} {
						if _, ok := b.Extra[key]; ok {
							delete(b.Extra, key)
							cleared = true
						}
					}
				}); err != nil {
					return err
				}
				if _, err := f.beadAutoCommit("cooldown clear " + args[0]); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
			if cleared {
				fmt.Fprintf(cmd.OutOrStdout(), "Cleared cooldown on %s\n", args[0])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: no cooldown set\n", args[0])
			}
			return nil
		},
	}
	cmd.AddCommand(clearCmd)

	return cmd
}

// newBeadClearCooldownCommand wires `ddx bead clear-cooldown --all [--reason <status>]`.
// It is the bulk operator escape hatch: clears work-retry-after across the tracker
// in one pass and prints the count and IDs so the operator can verify which beads
// were released. Use `ddx bead cooldown clear <id>` for single-bead clearing.
func (f *CommandFactory) newBeadClearCooldownCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear-cooldown",
		Short: "Bulk-clear work cooldowns across the tracker",
		Long: `clear-cooldown scans all beads with an active work-retry-after field and
clears them in one pass. Use after a systemic infra issue (provider outage,
index lock storm) is resolved so affected beads re-enter the execution queue
without per-bead loops.

Requires --all or --reason to prevent accidental bulk clears.`,
		Example: `  # Clear all active cooldowns and print the affected IDs
  ddx bead clear-cooldown --all

  # Clear only provider_connectivity cooldowns
  ddx bead clear-cooldown --reason provider_connectivity`,
		Args: cobra.NoArgs,
		RunE: f.runBeadClearCooldown,
	}
	cmd.Flags().Bool("all", false, "Clear cooldowns on every bead with retry-after set")
	cmd.Flags().String("reason", "", "Clear only beads where last-status matches this value")
	return cmd
}

func (f *CommandFactory) runBeadClearCooldown(cmd *cobra.Command, _ []string) error {
	all, _ := cmd.Flags().GetBool("all")
	reason, _ := cmd.Flags().GetString("reason")

	if !all && reason == "" {
		return fmt.Errorf("requires --all or --reason <value>")
	}

	store := f.beadStore()

	// Collect IDs first so we can print them alongside the count.
	allBeads, err := store.ReadAll(context.Background())
	if err != nil {
		return err
	}

	var ids []string
	for _, b := range allBeads {
		if b.Extra == nil {
			continue
		}
		v, _ := b.Extra[bead.ExtraRetryAfter].(string)
		if v == "" {
			continue
		}
		if reason != "" {
			s, _ := b.Extra[bead.ExtraLastStatus].(string)
			if s != reason {
				continue
			}
		}
		ids = append(ids, b.ID)
	}

	var filter func(bead.Bead) bool
	if reason != "" {
		r := reason
		filter = func(b bead.Bead) bool {
			if b.Extra == nil {
				return false
			}
			v, _ := b.Extra[bead.ExtraLastStatus].(string)
			return v == r
		}
	}

	count, err := store.ClearCooldowns(filter)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%d cooldowns cleared\n", count)
	for _, id := range ids {
		fmt.Fprintf(w, "  %s\n", id)
	}
	return nil
}

// newBeadReapCommand wires `ddx bead reap [--apply]`.
// Lists epic beads whose children have all reached terminal state; with --apply
// closes them in a batch. Useful for backfill-closing epics that predate the
// auto-close feature (which fires during ddx bead close going forward).
func (f *CommandFactory) newBeadReapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reap",
		Short: "List (or close) epics whose children are all terminal",
		Long: `Reap lists epic beads whose children have all reached a terminal state
(closed or cancelled). With --apply, it closes those epics in a batch.

Going forward, auto-close fires during ddx bead close whenever the last
non-terminal child of an epic transitions to a terminal state. Reap is
provided as a backfill tool for epics that landed before auto-close.

Examples:
  ddx bead reap           # List candidates (dry run)
  ddx bead reap --apply   # Close all candidates`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apply, _ := cmd.Flags().GetBool("apply")
			s := f.beadStore()

			candidates, err := s.EpicClosureCandidates(context.Background())
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no epics with all-terminal children found")
				return nil
			}

			if !apply {
				fmt.Fprintf(cmd.OutOrStdout(), "%d epic(s) with all-terminal children:\n", len(candidates))
				for _, b := range candidates {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", b.ID, b.Title)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "\nRun with --apply to close them.")
				return nil
			}

			return f.withBeadTrackerWriteLock(func() error {
				closed := 0
				for _, b := range candidates {
					if err := s.Close(context.Background(), b.ID); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to close %s: %v\n", b.ID, err)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "closed %s  %s\n", b.ID, b.Title)
					closed++
				}
				if closed > 0 {
					if _, err := f.beadAutoCommit(fmt.Sprintf("reap %d epic(s)", closed)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().Bool("apply", false, "Close the candidate epics (default is dry run)")
	return cmd
}
