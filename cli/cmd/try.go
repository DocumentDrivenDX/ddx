package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/spf13/cobra"
)

// newTryCommand creates the top-level "ddx try" command: an operator-targeted
// one-shot dispatch of a single bead through the production execution
// machinery (claim → executor → land → review). It is the layer-2 primitive
// "ddx work" iterates over; reach for "try" when you need to force a specific
// bead through (deterministic ordering, debugging, comparison studies),
// reserve "work" for queue-priority drain.
func (f *CommandFactory) newTryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "try <bead-id>",
		Short: "Dispatch a specific bead through the execution machinery",
		Long: `try targets one specific bead by ID and dispatches it through the same
claim → executor → land → review path that "ddx work" iterates. It is the
operator-targeted layer-2 primitive: useful when you need deterministic
queue ordering (refactor execution, comparison studies) or when debugging
a single misbehaving bead under autonomous drain.

Pre-flight gates (fail-fast before any claim):
  - bead must exist
  - bead must be in a claimable status (open, or stale-in_progress)
  - bead must have all blocking dependencies closed

After the gates pass, try forces a one-shot iteration of the execute-bead
loop targeting only the specified bead. All routing flags (--harness,
--model, --profile, --provider, --effort) are forwarded as opaque
passthrough constraints — try does NOT validate them. The agent owns
routing within the requested bounds.

Exit code maps the bead's terminal status:
  0 — success or already_satisfied
  1 — preserved_for_review, no_changes, or any non-success non-failure
  2 — execution_failed (or pre-flight gate failed)
`,
		Example: `  # Dispatch one specific ready bead
  ddx try ddx-1a2b3c4d

  # Force a particular harness/model for debugging
  ddx try ddx-1a2b3c4d --harness codex --model gpt-5.4

  # Skip the post-merge reviewer
  ddx try ddx-1a2b3c4d --no-review`,
		Args:          cobra.ExactArgs(1),
		RunE:          f.runTry,
		SilenceErrors: true, // we manage stderr framing ourselves via cmd.PrintErrln
		SilenceUsage:  true, // pre-flight failures are not usage errors
	}

	// Routing knobs — passthrough only. Same surface as `ddx work`.
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("model", "", "Model constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("profile", agent.DefaultRoutingProfile, "Routing profile: default, cheap, fast, or smart")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("opaque-passthrough", true, "Treat harness/provider/model as opaque (skip validation gate); default true for try")
	cmd.Flags().Bool("once", true, "Single-shot iteration (always true for try; flag retained for parity with work)")
	cmd.Flags().Duration("poll-interval", 0, "Unused for try (single-shot); flag retained for parity with work")
	cmd.Flags().Bool("json", false, "Output result as JSON")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review")
	cmd.Flags().String("review-harness", "", "Harness for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart tier)")
	cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, "Stop when accumulated billed cost exceeds USD; 0 = unlimited")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout; overrides project config and model-class defaults")
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained); passed to agent routing unchanged")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained); passed to agent routing unchanged")

	return cmd
}

// runTry is the RunE for "ddx try <bead-id>". Pre-flight gates are evaluated
// first against the bead store; on success the call delegates to the shared
// execute-loop impl with TargetBeadID set so the picker only sees the named
// bead.
func (f *CommandFactory) runTry(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	opaque, _ := cmd.Flags().GetBool("opaque-passthrough")

	if msg := preflightTryBead(store, beadID); msg != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
		return NewExitError(ExitCodeGeneralError, "")
	}
	return f.runAgentExecuteLoopImpl(cmd, opaque, beadID)
}

// preflightTryBead evaluates the three try-gate conditions against the bead
// store. It returns the operator-facing stderr message when a gate fails, or
// empty string when the bead is dispatchable. Pattern-matchable messages:
//   - "bead not found: <id>"
//   - "bead is not claimable (status=<status>)"
//   - "bead has unmet dependencies: <id>, <id>, ..."
func preflightTryBead(store *bead.Store, beadID string) string {
	b, err := store.Get(beadID)
	if err != nil {
		return fmt.Sprintf("bead not found: %s", beadID)
	}
	switch b.Status {
	case bead.StatusOpen, bead.StatusInProgress:
		// claimable (StatusInProgress only when heartbeat is stale; the
		// underlying Claim call enforces the staleness check atomically).
	default:
		return fmt.Sprintf("bead is not claimable (status=%s)", b.Status)
	}

	// Compute unmet dependencies. A bead is ready iff every dep id resolves to
	// a closed bead. Missing deps (dangling IDs) and non-closed deps both
	// block.
	all, err := store.ReadAll()
	if err != nil {
		return fmt.Sprintf("read bead store: %v", err)
	}
	statusByID := make(map[string]string, len(all))
	for _, x := range all {
		statusByID[x.ID] = x.Status
	}
	var unmet []string
	for _, depID := range b.DepIDs() {
		if statusByID[depID] != bead.StatusClosed {
			unmet = append(unmet, depID)
		}
	}
	if len(unmet) > 0 {
		sort.Strings(unmet)
		return fmt.Sprintf("bead has unmet dependencies: %s", joinUnmetDeps(unmet))
	}
	return ""
}

func joinUnmetDeps(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += ", " + id
	}
	return out
}

// formatTryResult renders the outcome of `ddx try <bead-id>` and returns an
// ExitError with a status-mapped exit code (0 success/already_satisfied,
// 1 preserved/no_changes/blocked, 2 execution_failed). The returned err is
// nil for success so cobra does not print anything; non-success returns an
// ExitError carrying just the exit code (Message="" suppresses the default
// "Error: ..." preamble in main.go).
func formatTryResult(cmd *cobra.Command, projectRoot, beadID string, result *agent.ExecuteBeadLoopResult, asJSON bool) error {
	if asJSON {
		payload := struct {
			ProjectRoot string                       `json:"project_root"`
			BeadID      string                       `json:"bead_id"`
			Result      *agent.ExecuteBeadLoopResult `json:"result"`
			Report      *agent.ExecuteBeadReport     `json:"report,omitempty"`
		}{
			ProjectRoot: projectRoot,
			BeadID:      beadID,
			Result:      result,
		}
		if len(result.Results) > 0 {
			payload.Report = &result.Results[0]
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "project: %s\n", projectRoot)
		fmt.Fprintf(cmd.OutOrStdout(), "bead:    %s\n", beadID)
		if result.NoReadyWork || len(result.Results) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "status:  no_attempt")
			fmt.Fprintln(cmd.ErrOrStderr(), "bead vanished from execution-ready set between pre-flight and dispatch")
			return NewExitError(ExitCodeGeneralError, "")
		}
		report := result.Results[0]
		fmt.Fprintf(cmd.OutOrStdout(), "status:  %s\n", report.Status)
		if report.Detail != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "detail:  %s\n", report.Detail)
		}
		if report.PreserveRef != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "preserve_ref: %s\n", report.PreserveRef)
		}
		if report.SessionID != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "session: %s\n", report.SessionID)
		}
		if report.DurationMS > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "duration: %s\n", time.Duration(report.DurationMS)*time.Millisecond)
		}
	}

	if result.NoReadyWork || len(result.Results) == 0 {
		return NewExitError(ExitCodeGeneralError, "")
	}
	switch result.Results[0].Status {
	case agent.ExecuteBeadStatusSuccess, agent.ExecuteBeadStatusAlreadySatisfied:
		return nil
	case agent.ExecuteBeadStatusExecutionFailed, agent.ExecuteBeadStatusStructuralValidationFailed:
		return NewExitError(2, "")
	default:
		// preserved_needs_review, no_changes, no_evidence_produced, post_run_check_failed,
		// land_conflict*, push_failed, declined_needs_decomposition, review_*, etc.
		return NewExitError(ExitCodeGeneralError, "")
	}
}
