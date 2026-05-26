package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	workguard "github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/spf13/cobra"
)

// newWorkCommand creates the top-level "ddx work" command: the FEAT-010 layer-3
// queue drain. ddx work treats --harness,
// --provider, and --model as opaque passthrough constraints that are forwarded
// to the agent unchanged. DDx does not validate or branch on their values.
// Retry-power policy between attempts is owned here, not in the agent service.
func (f *CommandFactory) newWorkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Drain the bead execution queue",
		Long: `work drains the execution-ready bead queue. It is the FEAT-010 layer-3
queue drain: it iterates ddx try (layer 2) across ready beads until a stop
condition is met and owns retry-power policy between attempts.

ddx work treats --harness, --provider, --model, and --profile as opaque
passthrough constraints forwarded to Fizeau unchanged. DDx does not validate
these values or branch on them; Fizeau owns routing within the requested power
bounds.

Review is on by default. --no-review is a break-glass override and
requires --no-review-i-know-what-im-doing. A bead label of review:skip
is only honored when it also carries a sibling review:skip-reason:*
label; otherwise the label is ignored.

Stop conditions (evaluated between attempts):
  drained     — no ready beads remain
  blocked     — every remaining bead has produced a terminal non-success outcome
  deferred    — configured budget exhausted
  no_progress — N consecutive attempts produced no commit (default N=3)
  signal      — SIGINT/SIGTERM received between attempts

work runs inline in the current process; per ADR-022 there is no separate
"submit to server" mode. The legacy --local flag is accepted but ignored
(deprecation warning printed) and will be removed in a future release.
`,
		Example: `  # Drain the current execution-ready queue and exit
  ddx work

  # Pick one ready bead, execute it, and stop
  ddx work --once

  # Watch for newly-ready work after the current queue drains
  ddx work --watch

  # Watch with a shorter idle scan interval
  ddx work --watch --idle-interval 15s

  # Forward harness/model as passthrough constraints (ddx does not validate these)
  ddx work --once --harness agent --model minimax/minimax-m2.7

  # Skip review only with the break-glass acknowledgement flag
  ddx work --once --no-review --no-review-i-know-what-im-doing

  # Constrain power powerClass (retry-power policy is owned by ddx work)
  ddx work --once --min-power 40 --max-power 90`,
		Args: cobra.NoArgs,
		RunE: f.runWork,
	}

	// Routing knobs — passthrough only. ddx work forwards these to the agent
	// unchanged and does not validate or branch on their string values.
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness constraint (passthrough; ddx work does not validate)")
	cmd.Flags().String("model", "", "Model constraint (passthrough; ddx work does not validate)")
	cmd.Flags().String("profile", "", "Routing profile: default, cheap, fast, or smart (empty = unconstrained; let the agent service choose)")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx work does not validate)")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().String("attempt-backend", "", "Attempt backend: worktree, local-clone, or docker-clone (default: executions.attempt_backend)")
	cmd.Flags().Bool("ignore-cooldown", false, "Ignore retry cooldown across this worker session (requires --reason)")
	cmd.Flags().String("reason", "", "Operator reason required by --ignore-cooldown")
	cmd.Flags().Bool("once", false, "Process at most one ready bead")
	cmd.Flags().Bool("watch", false, "Keep watching for newly-ready beads after the current queue drains")
	cmd.Flags().Duration("idle-interval", 30*time.Second, "Sleep duration between empty-queue scans in watch mode")
	cmd.Flags().Bool("json", false, "Output loop result as JSON")
	cmd.Flags().Bool("local", false, "Deprecated: no-op; ddx work always runs inline (ADR-022)")
	_ = cmd.Flags().MarkDeprecated("local", "ddx work always runs inline; the flag is a no-op (ADR-022)")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review (break-glass: requires --no-review-i-know-what-im-doing)")
	cmd.Flags().Bool("no-review-i-know-what-im-doing", false, "Break-glass acknowledgement required when using --no-review")
	cmd.Flags().String("review-harness", "", "Harness for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart powerClass)")
	cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, "Stop when accumulated billed cost exceeds USD; 0 = unlimited")
	cmd.Flags().Float64("max-bead-cost", escalation.DefaultMaxBeadCostUSD, "Per-bead cost budget in USD; stop escalating when this bead's billed cost exceeds this amount (0 = unlimited); overridden per-bead by a budget:<USD> label")
	cmd.Flags().Float64("max-recovery-cost", escalation.DefaultMaxRecoveryCostUSD, "Per-bead automated recovery budget in USD for reframe/decompose attempts after repeated ladder exhaustion")
	cmd.Flags().Duration("preclaim-timeout", workguard.DefaultPreClaimTimeout, "Pre-claim readiness timeout for preflight/readiness hooks")
	cmd.Flags().Duration("route-resolution-timeout", agent.DefaultRouteResolutionTimeout, "Timeout bounding routing preflight and the resolveRoute viability check; on expiry the lease is released and the bead is flagged for operator attention")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout; overrides project config and model-class defaults")
	// Per-bead rate-limit retry budget (ddx-c6e3db02 / TD-031 §8.4).
	cmd.Flags().Duration("rate-limit-max-wait", agent.RateLimitRetryDefaultBudget,
		"Per-bead total wait budget for HTTP 429 / rate-limit retries (default 5m). 0 keeps the default; negative disables retry.")
	// Power bounds: ddx work owns retry-power policy — these set the envelope
	// for operator-constrained power within which the agent selects the route.
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained); passed to agent routing unchanged")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained); passed to agent routing unchanged")

	// Register "ddx work plan", "ddx work focus", "ddx work clear-cooldowns",
	// "ddx work metrics", "ddx work analyze", and "ddx work status" as
	// subcommands.
	cmd.AddCommand(f.newWorkPlanCommand())
	cmd.AddCommand(f.newWorkFocusCommand())
	cmd.AddCommand(f.newWorkClearCooldownsCommand())
	cmd.AddCommand(f.newWorkMetricsCommand())
	cmd.AddCommand(f.newWorkAnalyzeCommand())
	cmd.AddCommand(f.newWorkStatusCommand())

	return cmd
}

// runWork is the RunE for "ddx work". It drives the FEAT-010 layer-3 queue
// drain with harness/provider/model treated as opaque passthrough constraints —
// ValidateForExecuteLoopViaService is not called; DDx does not inspect or
// branch on those values. Retry-power policy (min-power / max-power) is owned
// by this layer.
func (f *CommandFactory) runWork(cmd *cobra.Command, args []string) error {
	if err := f.checkLifecycleMigrationGate(cmd); err != nil {
		return err
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	f.warnIfInstalledBinaryBehindSource(cmd)

	// Preflight: warn once per process for degraded project-local skill layout.
	// Suppressed in JSON mode to avoid corrupting machine-readable output
	// (stderr and stdout are merged by some test helpers and log aggregators).
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		preflightResult := checkProjectRuntimePreflight(projectRoot)
		f.preflightWarnOnce.Do(func() {
			emitPreflightWarning(cmd.ErrOrStderr(), preflightResult)
		})
	}

	return f.runAgentExecuteLoopImpl(cmd, true, "")
}

func writeExecuteLoopResult(w io.Writer, projectRoot string, result *agent.ExecuteBeadLoopResult, asJSON bool) error {
	if asJSON {
		payload := struct {
			ProjectRoot string `json:"project_root"`
			*agent.ExecuteBeadLoopResult
		}{
			ProjectRoot:           projectRoot,
			ExecuteBeadLoopResult: result,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if result.NoReadyWork && result.Attempts == 0 {
		writeNoReadyWorkSummary(w, projectRoot, result.NoReadyWorkDetail)
		writeQueueSnapshotTerminalSummary(w, result.QueueSnapshot)
		return nil
	}

	fmt.Fprintf(w, "\nproject: %s\n", projectRoot)
	writeWorkTerminalSummary(w, result)
	writeOperatorAttentionSummary(w, result.OperatorAttention)
	if result.Failures > 0 {
		fmt.Fprintf(w, "\nfailed:\n")
		for _, attempt := range result.Results {
			if attempt.Status != "success" {
				fmt.Fprintf(w, "  - %s: %s", attempt.BeadID, attempt.Detail)
				if attempt.PreserveRef != "" {
					fmt.Fprintf(w, " (preserved)")
				}
				fmt.Fprintln(w)
			}
		}
	}
	if result.NoReadyWork {
		fmt.Fprintln(w)
		writeNoReadyWorkSummary(w, "", result.NoReadyWorkDetail)
		writeQueueSnapshotTerminalSummary(w, result.QueueSnapshot)
	}
	return nil
}

func writeWorkTerminalSummary(w io.Writer, result *agent.ExecuteBeadLoopResult) {
	closed, changed, alreadySatisfied := countWorkTerminalOutcomes(result)
	fmt.Fprintf(w, "worker exited: %s\n", workExitSummary(result))
	fmt.Fprintf(w, "attempts: %d  |  closed: %d  |  changed: %d  |  already-satisfied: %d  |  failures: %d\n",
		result.Attempts, closed, changed, alreadySatisfied, result.Failures)
}

func writeOperatorAttentionSummary(w io.Writer, stop *agent.OperatorAttentionStop) {
	if stop == nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "operator attention:")
	if stop.BeadID != "" {
		fmt.Fprintf(w, "  released bead: %s\n", stop.BeadID)
	}
	if stop.Message != "" {
		fmt.Fprintf(w, "  %s\n", stop.Message)
	}
	if stop.ProjectRoot != "" {
		fmt.Fprintf(w, "  project root: %s\n", stop.ProjectRoot)
	}
	if len(stop.DirtyPaths) > 0 {
		fmt.Fprintf(w, "  dirty paths: %s\n", strings.Join(stop.DirtyPaths, ", "))
	}
}

func countWorkTerminalOutcomes(result *agent.ExecuteBeadLoopResult) (closed, changed, alreadySatisfied int) {
	for _, attempt := range result.Results {
		switch attempt.Status {
		case agent.ExecuteBeadStatusSuccess:
			closed++
			changed++
		case agent.ExecuteBeadStatusAlreadySatisfied:
			closed++
			alreadySatisfied++
		}
	}
	if closed == 0 && result.Successes > 0 {
		closed = result.Successes
	}
	return closed, changed, alreadySatisfied
}

func workExitSummary(result *agent.ExecuteBeadLoopResult) string {
	switch result.ExitReason {
	case "drained":
		return "drained current execution-ready queue"
	case "once_complete":
		return "once complete"
	case "budget":
		return "budget reached"
	case "no_progress":
		return "no-progress policy stopped work"
	case "blocked":
		return "blocked waiting for external action"
	case "operator_attention":
		return "operator attention required"
	case "sigint", "sigterm", "context_cancelled":
		return "stopped by signal"
	case "fatal_config":
		return "fatal configuration error"
	case "preflight_failed":
		return "preflight failed"
	case "resource_exhausted":
		return "resource exhausted"
	case "routing_unavailable":
		return "routing unavailable"
	}
	if result.StopCondition != "" {
		return strings.ToLower(strings.ReplaceAll(result.StopCondition, "_", " "))
	}
	return "completed"
}

func writeQueueSnapshotTerminalSummary(w io.Writer, snapshot *agent.QueueSnapshot) {
	if snapshot == nil {
		return
	}
	retry := ""
	if snapshot.NextRetryAfter != "" {
		retry = " next-retry=" + snapshot.NextRetryAfter
	}
	fmt.Fprintf(w, "remaining queue: execution-ready=%d blocked=%d operator-attention=%d needs-human/investigation=%d cooldown/deferred=%d%s execution-ineligible=%d superseded=%d epics=%d epic-closure-candidates=%d\n",
		snapshot.ExecutionReadyCount,
		snapshot.BlockedCount,
		snapshot.ProposedOperatorAttentionCount,
		snapshot.HumanReviewBlockerCount,
		snapshot.RetryCooldownCount,
		retry,
		snapshot.ExecutionIneligibleCount,
		snapshot.SupersededCount,
		snapshot.SkippedEpicsCount,
		snapshot.EpicClosureCandidatesCount,
	)
	if snapshot.HumanReviewBlockedTotal <= 0 || snapshot.HumanReviewBlockerCount <= 0 {
		return
	}
	fmt.Fprintf(w, "%d beads blocked behind %d needs-human blockers:\n",
		snapshot.HumanReviewBlockedTotal, snapshot.HumanReviewBlockerCount)
	for i, blocker := range snapshot.HumanReviewBlockers {
		fmt.Fprintf(w, "  %d. %s %s (%d downstream)\n", i+1, blocker.ID, blocker.Title, blocker.DownstreamBlockedCount)
	}
}

func writeNoReadyWorkSummary(w io.Writer, projectRoot string, d agent.NoReadyWorkBreakdown) {
	if projectRoot != "" {
		fmt.Fprintf(w, "project: %s\n", projectRoot)
	}
	fmt.Fprintln(w, "No execution-ready beads.")
	if noReadyWorkBreakdownEmpty(d) {
		fmt.Fprintln(w, "  queue drained: no open work remains in lifecycle queues.")
	}
	if len(d.ProposedOperatorAttention) > 0 {
		fmt.Fprintf(w, "  operator attention: %d proposed bead(s) stop autonomous work and may block downstream dependents: %s\n",
			len(d.ProposedOperatorAttention), strings.Join(d.ProposedOperatorAttention, ", "))
	}
	if len(d.DependencyWaiting) > 0 {
		fmt.Fprintf(w, "  waiting on dependencies: %d open bead(s): %s\n",
			len(d.DependencyWaiting), strings.Join(d.DependencyWaiting, ", "))
	}
	if len(d.ExternalBlocked) > 0 {
		fmt.Fprintf(w, "  external blocked: %d bead(s) with explicit blocked status: %s\n",
			len(d.ExternalBlocked), strings.Join(d.ExternalBlocked, ", "))
	}
	if len(d.RetryCooldown) > 0 {
		retryHint := ""
		if d.NextRetryAfter != "" {
			retryHint = " (next retry-after: " + d.NextRetryAfter + ")"
		}
		fmt.Fprintf(w, "  retry cooldown: %d bead(s)%s: %s\n",
			len(d.RetryCooldown), retryHint, strings.Join(d.RetryCooldown, ", "))
	}
	if len(d.NotEligible) > 0 {
		fmt.Fprintf(w, "  not execution eligible: %d bead(s): %s\n",
			len(d.NotEligible), strings.Join(d.NotEligible, ", "))
	}
	if len(d.Superseded) > 0 {
		fmt.Fprintf(w, "  superseded: %d bead(s): %s\n",
			len(d.Superseded), strings.Join(d.Superseded, ", "))
	}
	if len(d.Epics) > 0 {
		fmt.Fprintf(w, "  epic containers: %d ready epic(s) with open children (decompose into tasks): %s\n",
			len(d.Epics), strings.Join(d.Epics, ", "))
	}
	if len(d.EpicClosureCandidates) > 0 {
		fmt.Fprintf(w, "  completed epic closure candidate(s) (all direct children closed; surfaced for closure evaluation): %s\n",
			strings.Join(d.EpicClosureCandidates, ", "))
	}
}

func noReadyWorkBreakdownEmpty(d agent.NoReadyWorkBreakdown) bool {
	return len(d.ExecutionReady) == 0 &&
		len(d.DependencyWaiting) == 0 &&
		len(d.ProposedOperatorAttention) == 0 &&
		len(d.RetryCooldown) == 0 &&
		len(d.ExternalBlocked) == 0 &&
		len(d.NotEligible) == 0 &&
		len(d.Superseded) == 0 &&
		len(d.Epics) == 0 &&
		len(d.EpicClosureCandidates) == 0 &&
		d.NextRetryAfter == ""
}
