package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/activework"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

// WorkFocusActiveWorker is one entry in the active_workers section.
type WorkFocusActiveWorker struct {
	WorkerID       string `json:"worker_id"`
	CurrentBead    string `json:"current_bead,omitempty"`
	AttemptID      string `json:"attempt_id,omitempty"`
	Phase          string `json:"phase,omitempty"`
	LastActivityAt string `json:"last_activity_at,omitempty"`
}

// WorkFocusBead is one item in the operator-attention section.
type WorkFocusBead struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Reason          string `json:"reason,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

// WorkFocusBlockedBead is one item in the blocked_or_planning section.
type WorkFocusBlockedBead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	BlockerKind string `json:"blocker_kind"`
	Detail      string `json:"detail,omitempty"`
}

// WorkFocusReadySummary is the ready-queue depth summary.
type WorkFocusReadySummary struct {
	Count int    `json:"count"`
	Depth string `json:"depth"` // "empty", "shallow", "moderate", "deep"
}

// WorkFocusReport is the structured result of "ddx work focus".
type WorkFocusReport struct {
	HumanRequired         []WorkFocusBead         `json:"human_required"`
	BlockedOrPlanning     []WorkFocusBlockedBead  `json:"blocked_or_planning"`
	ReadySummary          WorkFocusReadySummary   `json:"ready_summary"`
	ActiveWorkers         []WorkFocusActiveWorker `json:"active_workers,omitempty"`
	ActiveWork            activework.Snapshot     `json:"active_work"`
	ProjectRootDirtyPaths []string                `json:"project_root_dirty_paths"`
	WorkerRecommendation  string                  `json:"worker_recommendation"`
	Unknowns              []string                `json:"unknowns"`
}

// workerReadyDepthLabel returns a human-readable depth label for the ready count.
func workerReadyDepthLabel(count int) string {
	switch {
	case count == 0:
		return "empty"
	case count <= 1:
		return "shallow"
	case count <= 3:
		return "moderate"
	default:
		return "deep"
	}
}

// buildWorkFocusReport queries the store and returns a WorkFocusReport.
//
// projectRoot, when non-empty, enables the active_workers section and active
// work summary: fresh claim heartbeats, liveness sidecars, and run-state files
// are reconciled into one project-scoped view so the operator-facing answer
// stays aligned across bead, work, and GraphQL status surfaces.
func buildWorkFocusReport(store *bead.Store, projectRoot string) (WorkFocusReport, error) {
	// Collect operator-attention beads (status=proposed, any dep state).
	operatorAttentionBeads, err := store.ProposedOperatorAttention()
	if err != nil {
		return WorkFocusReport{}, fmt.Errorf("work focus: operator attention query: %w", err)
	}

	// Build a set of operator-attention IDs so we can exclude them from blocked section.
	operatorAttentionSet := make(map[string]bool, len(operatorAttentionBeads))
	humanRequired := make([]WorkFocusBead, 0, len(operatorAttentionBeads))
	for _, b := range operatorAttentionBeads {
		operatorAttentionSet[b.ID] = true
		meta := bead.GetNeedsHumanMeta(b)
		item := WorkFocusBead{
			ID:    b.ID,
			Title: b.Title,
		}
		if meta.Reason != "" {
			item.Reason = meta.Reason
		} else if meta.Summary != "" {
			item.Reason = meta.Summary
		}
		if meta.SuggestedAction != "" {
			item.SuggestedAction = meta.SuggestedAction
		}
		humanRequired = append(humanRequired, item)
	}

	// Collect all blocked beads; exclude operator-attention beads already captured above.
	allBlocked, err := store.BlockedAll()
	if err != nil {
		return WorkFocusReport{}, fmt.Errorf("work focus: blocked query: %w", err)
	}
	var blockedOrPlanning []WorkFocusBlockedBead
	for _, bb := range allBlocked {
		if operatorAttentionSet[bb.ID] {
			continue
		}
		item := WorkFocusBlockedBead{
			ID:          bb.ID,
			Title:       bb.Title,
			BlockerKind: bb.Blocker.Kind,
		}
		switch bb.Blocker.Kind {
		case bead.BlockerKindDependency:
			if len(bb.Blocker.UnclosedDepIDs) > 0 {
				item.Detail = "unclosed deps: " + strings.Join(bb.Blocker.UnclosedDepIDs, ", ")
			}
		case bead.BlockerKindRetryCooldown:
			if bb.Blocker.NextEligibleAt != "" {
				item.Detail = "retry after: " + bb.Blocker.NextEligibleAt
			}
		default:
			if bb.Blocker.Reason != "" {
				item.Detail = bb.Blocker.Reason
			}
		}
		blockedOrPlanning = append(blockedOrPlanning, item)
	}

	// Collect worker-ready depth.
	readyBeads, err := store.ReadyExecution()
	if err != nil {
		return WorkFocusReport{}, fmt.Errorf("work focus: ready query: %w", err)
	}
	readyCount := len(readyBeads)
	readySummary := WorkFocusReadySummary{
		Count: readyCount,
		Depth: workerReadyDepthLabel(readyCount),
	}

	// Observe in_progress beads as a capacity signal.
	inProgressBeads, err := store.List(bead.StatusInProgress, "", nil)
	if err != nil {
		return WorkFocusReport{}, fmt.Errorf("work focus: in_progress query: %w", err)
	}
	inProgressCount := len(inProgressBeads)

	// Shared active-work snapshot: claim heartbeats, worker liveness
	// sidecars, and run-state records are reconciled into one project-scoped
	// view so operator surfaces can agree on what is actually active.
	activeWork, err := collectActiveWorkSnapshot(projectRoot, store, time.Now())
	if err != nil {
		return WorkFocusReport{}, fmt.Errorf("work focus: active work query: %w", err)
	}
	activeWorkers := activeWorkRecordsForFocus(activeWork)
	dirtyPaths := agent.CanonicalRootDirtyPaths(projectRoot)

	// Conservative worker recommendation. Treat fresh active work as the
	// capacity signal rather than the raw lifecycle count.
	workerRec := buildWorkerRecommendation(readyCount, activeWork.Count, dirtyPaths)

	// Unknowns: worker process liveness is not verifiable from the bead store
	// alone; only add the hazard when we have in-progress beads but the only
	// active evidence is a claim heartbeat.
	var unknowns []string
	hasNonClaimActive := false
	for _, rec := range activeWork.Records {
		if rec.Source != "claim" {
			hasNonClaimActive = true
			break
		}
	}
	if inProgressCount > 0 && !hasNonClaimActive {
		unknowns = append(unknowns, fmt.Sprintf(
			"worker process liveness: %d bead(s) are in_progress but no active worker snapshot is fresh; check `ddx work status` for live processes",
			inProgressCount,
		))
	}

	return WorkFocusReport{
		HumanRequired:         humanRequired,
		BlockedOrPlanning:     blockedOrPlanning,
		ReadySummary:          readySummary,
		ActiveWorkers:         activeWorkers,
		ActiveWork:            activeWork,
		ProjectRootDirtyPaths: dirtyPaths,
		WorkerRecommendation:  workerRec,
		Unknowns:              unknowns,
	}, nil
}

// buildWorkerRecommendation returns a conservative recommendation string.
// It suggests another worker only when ready depth is high and capacity is observable.
func buildWorkerRecommendation(readyCount, activeCount int, dirtyPaths []string) string {
	if len(dirtyPaths) > 0 {
		return fmt.Sprintf(
			"%d bead(s) ready, but the project root has uncommitted tracked changes (%s); commit or clean those paths before starting ddx work.",
			readyCount,
			strings.Join(dirtyPaths, ", "),
		)
	}
	switch {
	case readyCount == 0:
		return "Queue is empty; no worker action needed."
	case activeCount > 0 && readyCount > 0:
		return fmt.Sprintf(
			"%d bead(s) ready; %d active worker(s) observed. Monitor progress or run 'ddx work' to add capacity.",
			readyCount, activeCount,
		)
	case readyCount >= 3:
		return fmt.Sprintf(
			"%d bead(s) ready and no active workers detected; consider running: ddx work",
			readyCount,
		)
	default:
		return fmt.Sprintf(
			"%d bead(s) ready; run 'ddx work' to process.",
			readyCount,
		)
	}
}

func (f *CommandFactory) newWorkFocusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "focus",
		Short: "Show work requiring operator attention",
		Long: `focus shows a read-only summary of queue items that require operator
attention — proposed beads, external blockers, dependency-waiting or
planning-required beads — along with a conservative worker-capacity
recommendation based on observable queue depth and in-progress signals.

Unlike 'ddx work plan', focus is designed for interactive queue stewardship:
it surfaces the items workers will NOT pick up and recommends operator actions.
Worker-ready beads are not listed as primary intervention items; they appear
only as a depth summary.

	Use --json for machine-readable output with stable keys:
	  human_required, blocked_or_planning, ready_summary,
	  active_work, worker_recommendation, unknowns
	`,
		Example: `  # Show intervention queue in human-readable format
  ddx work focus

  # Machine-readable output
  ddx work focus --json | jq '.human_required'`,
		Args: cobra.NoArgs,
		RunE: f.runWorkFocus,
	}
	cmd.Flags().Bool("json", false, "Output as JSON object with stable keys")
	return cmd
}

func (f *CommandFactory) runWorkFocus(cmd *cobra.Command, _ []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")

	ddxDir := resolveBeadStoreRoot(f.WorkingDir)
	store := bead.NewStore(ddxDir)

	report, err := buildWorkFocusReport(store, f.WorkingDir)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return printWorkFocusText(cmd, report)
}

func printWorkFocusText(cmd *cobra.Command, r WorkFocusReport) error {
	out := cmd.OutOrStdout()

	// Section: Operator attention
	fmt.Fprintf(out, "=== Operator attention (%d) ===\n", len(r.HumanRequired))
	if len(r.HumanRequired) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, item := range r.HumanRequired {
			fmt.Fprintf(out, "  [%s] %s\n", item.ID, item.Title)
			if item.Reason != "" {
				fmt.Fprintf(out, "       reason: %s\n", item.Reason)
			}
			if item.SuggestedAction != "" {
				fmt.Fprintf(out, "       suggested action: %s\n", item.SuggestedAction)
			}
		}
	}

	// Section: Blocked / planning
	fmt.Fprintf(out, "\n=== Blocked / planning (%d) ===\n", len(r.BlockedOrPlanning))
	if len(r.BlockedOrPlanning) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, item := range r.BlockedOrPlanning {
			fmt.Fprintf(out, "  [%s] %s\n", item.ID, item.Title)
			if item.Detail != "" {
				fmt.Fprintf(out, "       blocker: %s — %s\n", item.BlockerKind, item.Detail)
			} else {
				fmt.Fprintf(out, "       blocker: %s\n", item.BlockerKind)
			}
		}
	}

	// Section: Worker-ready summary
	fmt.Fprintf(out, "\n=== Worker-ready summary ===\n")
	fmt.Fprintf(out, "  %d bead(s) ready for worker execution (%s)\n", r.ReadySummary.Count, r.ReadySummary.Depth)
	if len(r.ProjectRootDirtyPaths) > 0 {
		fmt.Fprintf(out, "  project root dirty paths: %s\n", strings.Join(r.ProjectRootDirtyPaths, ", "))
	}

	// Section: Active workers (sidecar-derived). Surfaced so an operator
	// asking "is the worker alive?" gets a positive answer even when the
	// bead tracker's claim timestamp has not advanced.
	if len(r.ActiveWorkers) > 0 {
		fmt.Fprintf(out, "\n=== Active workers (%d) ===\n", len(r.ActiveWorkers))
		for _, w := range r.ActiveWorkers {
			fmt.Fprintf(out, "  [%s] bead=%s attempt=%s phase=%s last=%s\n",
				w.WorkerID, dashIfEmpty(w.CurrentBead), dashIfEmpty(w.AttemptID),
				dashIfEmpty(w.Phase), w.LastActivityAt)
		}
	}

	// Section: Worker recommendation
	fmt.Fprintf(out, "\n=== Worker recommendation ===\n")
	fmt.Fprintf(out, "  %s\n", r.WorkerRecommendation)

	// Section: Unknowns
	fmt.Fprintf(out, "\n=== Unknowns ===\n")
	if len(r.Unknowns) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, u := range r.Unknowns {
			fmt.Fprintf(out, "  - %s\n", u)
		}
	}

	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
