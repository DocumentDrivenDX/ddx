package cmd

import (
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/spf13/cobra"
)

// newWorkCommand creates the top-level "ddx work" command: the FEAT-010 layer-3
// queue drain. Unlike "ddx agent execute-loop", ddx work treats --harness,
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

Unlike "ddx agent execute-loop", ddx work treats --harness, --provider, and
--model as opaque passthrough constraints forwarded to the agent unchanged.
DDx does not validate these values or branch on them; the agent owns routing
within the requested power bounds.

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
		Example: `  # Drain the current execution-ready queue once and exit
  ddx work

  # Pick one ready bead, execute it, and stop
  ddx work --once

  # Run continuously as a bounded queue worker
  ddx work --poll-interval 30s

  # Forward harness/model as passthrough constraints (ddx does not validate these)
  ddx work --once --harness agent --model minimax/minimax-m2.7

  # Constrain power tier (retry-power policy is owned by ddx work)
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
	cmd.Flags().String("profile", agent.DefaultRoutingProfile, "Routing profile: default, cheap, fast, or smart")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx work does not validate)")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("once", false, "Process at most one ready bead")
	cmd.Flags().Duration("poll-interval", 30*time.Second, "Poll interval for continuous scanning; zero drains current ready work and exits (legacy opt-out). Default 30s keeps the worker alive across empty polls.")
	cmd.Flags().Bool("json", false, "Output loop result as JSON")
	cmd.Flags().Bool("local", false, "Deprecated: no-op; ddx work always runs inline (ADR-022)")
	_ = cmd.Flags().MarkDeprecated("local", "ddx work always runs inline; the flag is a no-op (ADR-022)")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review")
	cmd.Flags().String("review-harness", "", "Harness for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart tier)")
	cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, "Stop when accumulated billed cost exceeds USD; 0 = unlimited")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout; overrides project config and model-class defaults")
	// Power bounds: ddx work owns retry-power policy — these set the envelope
	// for operator-constrained power within which the agent selects the route.
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained); passed to agent routing unchanged")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained); passed to agent routing unchanged")

	return cmd
}

// runWork is the RunE for "ddx work". It drives the FEAT-010 layer-3 queue
// drain with harness/provider/model treated as opaque passthrough constraints —
// ValidateForExecuteLoopViaService is not called; DDx does not inspect or
// branch on those values. Retry-power policy (min-power / max-power) is owned
// by this layer.
func (f *CommandFactory) runWork(cmd *cobra.Command, args []string) error {
	return f.runAgentExecuteLoopImpl(cmd, true, "")
}
