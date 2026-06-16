package agent

import (
	"context"
	"sort"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// IdleAutoRemediationConfig controls the idle-path diagnose→remediate→rescan
// pass (FEAT-010 §Idle-Path Diagnosis and Auto-Remediation). The zero value
// disables every remediation, so callers that do not populate it keep the
// legacy idle behavior (diagnose, emit loop.idle, sleep/return). `ddx work`
// populates it from work.autoRemediations config (defaulting each flag to
// true) with CLI --no-auto-* flags overriding config.
type IdleAutoRemediationConfig struct {
	// AutoSupersedeClose gates cascade-closing beads diagnosed as
	// superseded_pending_close.
	AutoSupersedeClose bool
	// AutoEpicDecompose gates dispatching AutoDecomposeEpic for beads
	// diagnosed as genuinely_needs_decomposition.
	AutoEpicDecompose bool
	// AutoClosureReclassify gates closing beads diagnosed as
	// closure_candidate_misclassified or dead_intermediate_all_children_closed.
	AutoClosureReclassify bool
	// MaxRecoveryCostUSD is the per-bead automated recovery budget honored by
	// the idle-path auto-decompose dispatch. A value <= 0 means no recovery
	// budget is available and auto-decompose is gated.
	MaxRecoveryCostUSD float64
	// Decompose dispatches AutoDecomposeEpic for the named bead. When nil,
	// auto-decompose is treated as unavailable and skipped.
	Decompose func(ctx context.Context, beadID string) (DecomposeResult, error)
}

// enabled reports whether any idle-path remediation is turned on.
func (c IdleAutoRemediationConfig) enabled() bool {
	return c.AutoSupersedeClose || c.AutoEpicDecompose || c.AutoClosureReclassify
}

// allBeadsReader is the optional store capability used by the idle-path
// diagnosis pass to read the full bead graph for index construction.
type allBeadsReader interface {
	ReadAll(ctx context.Context) ([]bead.Bead, error)
}

// supersededCascader is the optional store capability used to cascade-close
// beads superseded by a closed superseder, out of band from Close().
type supersededCascader interface {
	RunSupersededCascade(supersederID string) (int, error)
}

// idleRemediationGate records a non-ready bead whose remediation was
// suppressed (by an operator override flag or by an exhausted recovery
// budget). It is surfaced via the loop.auto_remediation_gated event.
type idleRemediationGate struct {
	BeadID string `json:"bead_id"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// runIdleAutoRemediation diagnoses every non-ready bead reported by breakdown
// and fires the matching idle-path remediator (cascade-close, closure
// evaluation, or auto-decompose). It returns true when at least one
// remediation succeeded, signalling the caller to clear transient skips and
// re-scan the queue without sleeping. When nothing succeeds but some beads
// were gated by an override flag or budget, it emits loop.auto_remediation_gated
// and returns false so the caller falls through to the normal idle path.
//
// recoverySpent accumulates auto-decompose cost across loop iterations so the
// per-bead recovery budget is honored for the whole run.
func (w *ExecuteBeadWorker) runIdleAutoRemediation(
	ctx context.Context,
	runtime ExecuteBeadLoopRuntime,
	breakdown bead.ReadyExecutionBreakdown,
	emit func(string, map[string]any),
	recoverySpent *float64,
) bool {
	cfg := runtime.IdleAutoRemediation
	if !cfg.enabled() {
		return false
	}
	reader, ok := w.Store.(allBeadsReader)
	if !ok {
		return false
	}
	all, err := reader.ReadAll(ctx)
	if err != nil || len(all) == 0 {
		return false
	}

	byID := make(map[string]bead.Bead, len(all))
	for _, b := range all {
		byID[b.ID] = b
	}
	childIndex := buildChildIndex(all)
	openChildIndex := buildOpenChildIndex(all)
	depIndex := buildDepIndex(all)

	closer, _ := w.Store.(epicCloser)
	cascader, _ := w.Store.(supersededCascader)

	counts := map[string]int{}
	gated := []idleRemediationGate{}
	totalAttempted := 0
	totalSucceeded := 0
	decomposeDispatched := false

	for _, id := range idleRemediationWalkIDs(breakdown) {
		b, ok := byID[id]
		if !ok {
			continue
		}
		reason := bead.Diagnose(b, childIndex, openChildIndex, depIndex)
		switch reason.Code {
		case bead.ReasonSupersededPendingClose:
			if !cfg.AutoSupersedeClose {
				continue
			}
			if cascader == nil || len(reason.CitedIDs) == 0 {
				continue
			}
			totalAttempted++
			closedCount, cErr := cascader.RunSupersededCascade(reason.CitedIDs[0])
			if cErr == nil && closedCount > 0 {
				counts[string(reason.Code)]++
				totalSucceeded++
			}

		case bead.ReasonClosureCandidateMisclassified, bead.ReasonDeadIntermediateAllChildrenClosed:
			if !cfg.AutoClosureReclassify {
				continue
			}
			if closer == nil {
				continue
			}
			totalAttempted++
			if cErr := closer.Close(ctx, b.ID); cErr == nil {
				counts[string(reason.Code)]++
				totalSucceeded++
			}

		case bead.ReasonGenuinelyNeedsDecomposition:
			if !cfg.AutoEpicDecompose {
				gated = append(gated, idleRemediationGate{
					BeadID: b.ID,
					Reason: string(bead.ReasonGatedByBudgetOrCooldown),
					Detail: "no-auto-epic-decompose",
				})
				continue
			}
			if decomposeDispatched {
				// At most one auto-decompose dispatch per loop iteration; the
				// remaining undecomposed epics are handled on later iterations.
				continue
			}
			if cfg.MaxRecoveryCostUSD <= 0 || *recoverySpent >= cfg.MaxRecoveryCostUSD {
				gated = append(gated, idleRemediationGate{
					BeadID: b.ID,
					Reason: string(bead.ReasonGatedByBudgetOrCooldown),
					Detail: "max-recovery-cost-exhausted",
				})
				continue
			}
			if cfg.Decompose == nil {
				continue
			}
			decomposeDispatched = true
			totalAttempted++
			res, dErr := cfg.Decompose(ctx, b.ID)
			*recoverySpent += res.CostUSD
			if dErr == nil && !res.Failed {
				counts[string(reason.Code)]++
				totalSucceeded++
			}

		default:
			// dep_blocked_by, parent_child_state_conflict, claimed_in_progress,
			// provider_route_unavailable, gated_by_budget_or_cooldown,
			// malformed_parent_or_dep_ref, dependency_cycle,
			// closed_or_missing_parent, epic_of_epic,
			// dead_intermediate_open_children_pending, stale_graph_index,
			// auto_remediation_exhausted, no_diagnosis: no idle-path action.
			// These surface via ddx work focus.
		}
	}

	if totalSucceeded > 0 {
		data := map[string]any{
			"remediation_counts": counts,
			"total_attempted":    totalAttempted,
			"total_succeeded":    totalSucceeded,
		}
		if len(gated) > 0 {
			data["gated"] = gated
		}
		emit("loop.auto_remediated", data)
		return true
	}

	if len(gated) > 0 {
		emit("loop.auto_remediation_gated", map[string]any{
			"gated":           gated,
			"total_attempted": totalAttempted,
		})
	}
	return false
}

// idleRemediationWalkIDs returns the de-duplicated set of non-ready bead IDs
// the idle-path pass diagnoses: every bucket except the execution-ready queue
// and the retry-cooldown lane (which has its own infra-cooldown handling).
func idleRemediationWalkIDs(b bead.ReadyExecutionBreakdown) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(ids []string) {
		for _, id := range ids {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	add(b.Epics)
	add(b.EpicClosureCandidates)
	add(b.DependencyWaiting)
	add(b.ProposedOperatorAttention)
	add(b.Superseded)
	add(b.NotEligible)
	add(b.ExternalBlocked)
	sort.Strings(out)
	return out
}
