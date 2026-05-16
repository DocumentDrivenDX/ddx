package bead

import (
	"fmt"
	"sort"
	"strings"
)

// BlockReasonCode is the closed taxonomy of reasons an open bead is non-ready
// in the idle-path diagnosis pass. The full set is defined by FEAT-010
// §Idle-Path Diagnosis and Auto-Remediation; new codes require that spec to
// be updated.
type BlockReasonCode string

const (
	ReasonSupersededPendingClose              BlockReasonCode = "superseded_pending_close"
	ReasonClosureCandidateMisclassified       BlockReasonCode = "closure_candidate_misclassified"
	ReasonDeadIntermediateAllChildrenClosed   BlockReasonCode = "dead_intermediate_all_children_closed"
	ReasonDeadIntermediateOpenChildrenPending BlockReasonCode = "dead_intermediate_open_children_pending"
	ReasonEpicOfEpic                          BlockReasonCode = "epic_of_epic"
	ReasonDepBlockedBy                        BlockReasonCode = "dep_blocked_by"
	ReasonGenuinelyNeedsDecomposition         BlockReasonCode = "genuinely_needs_decomposition"
	ReasonParentChildStateConflict            BlockReasonCode = "parent_child_state_conflict"
	ReasonClaimedInProgress                   BlockReasonCode = "claimed_in_progress"
	ReasonProviderRouteUnavailable            BlockReasonCode = "provider_route_unavailable"
	ReasonGatedByBudgetOrCooldown             BlockReasonCode = "gated_by_budget_or_cooldown"
	ReasonMalformedParentOrDepRef             BlockReasonCode = "malformed_parent_or_dep_ref"
	ReasonDependencyCycle                     BlockReasonCode = "dependency_cycle"
	ReasonClosedOrMissingParent               BlockReasonCode = "closed_or_missing_parent"
	ReasonStaleGraphIndex                     BlockReasonCode = "stale_graph_index"
	ReasonAutoRemediationExhausted            BlockReasonCode = "auto_remediation_exhausted"
	ReasonNoDiagnosis                         BlockReasonCode = "no_diagnosis"
)

// BlockReason is a per-bead diagnosis of why a bead is non-ready. Detail is a
// human-readable string suitable for surfacing to operators; CitedIDs lists
// other bead IDs the reason references (deps, supersession targets, parents,
// cycle participants).
type BlockReason struct {
	Code     BlockReasonCode
	Detail   string
	CitedIDs []string
}

// Diagnose returns the first matching BlockReason for b according to the
// FEAT-010 §Idle-Path Diagnosis and Auto-Remediation taxonomy. The caller
// must pre-build the indexes:
//
//   - childIndex: parent_id -> every child bead (any status). Root beads are
//     keyed under the empty string. Diagnose also uses childIndex as the
//     global bead lookup; every referenced bead (supersession target, dep
//     target, parent) must appear in some entry for Diagnose to verify its
//     state.
//   - openChildIndex: parent_id -> open child beads only.
//   - depIndex: bead_id -> outgoing Dependency entries for that bead. Used
//     for global cycle detection across the dep DAG.
//
// Diagnose is pure: it performs no I/O, does not consult global state, and
// does not mutate its inputs.
func Diagnose(
	b Bead,
	childIndex map[string][]Bead,
	openChildIndex map[string][]Bead,
	depIndex map[string][]Dependency,
) BlockReason {
	// 1. superseded_pending_close
	if y := supersedingTargetID(b); y != "" {
		if target, ok := lookupBead(childIndex, y); ok && target.Status == StatusClosed {
			return BlockReason{
				Code:     ReasonSupersededPendingClose,
				Detail:   fmt.Sprintf("superseded by closed bead %s", y),
				CitedIDs: []string{y},
			}
		}
	}

	total := len(childIndex[b.ID])
	open := len(openChildIndex[b.ID])

	// 2. closure_candidate_misclassified — epic with closure-ready child set
	if isEpicBead(b) && total > 0 && open == 0 {
		return BlockReason{
			Code:   ReasonClosureCandidateMisclassified,
			Detail: fmt.Sprintf("epic %s has %d children, none open", b.ID, total),
		}
	}

	// 3 & 4. dead_intermediate_* — bead opted out of execution
	if eligible, set := lifecycleExecutionEligible(b); set && !eligible {
		if total > 0 && open == 0 {
			return BlockReason{
				Code:   ReasonDeadIntermediateAllChildrenClosed,
				Detail: "execution-eligible=false; all children closed",
			}
		}
		if open > 0 {
			ids := beadIDsFrom(openChildIndex[b.ID])
			return BlockReason{
				Code:     ReasonDeadIntermediateOpenChildrenPending,
				Detail:   fmt.Sprintf("execution-eligible=false; %d open child(ren) pending", open),
				CitedIDs: ids,
			}
		}
	}

	// 5. epic_of_epic — only open descendants are themselves epics
	if open > 0 {
		openChildren := openChildIndex[b.ID]
		if allEpicBeads(openChildren) {
			return BlockReason{
				Code:     ReasonEpicOfEpic,
				Detail:   "only open children are themselves epics",
				CitedIDs: beadIDsFrom(openChildren),
			}
		}
	}

	// 6. dep_blocked_by — unresolved outgoing deps
	if blockers := openDepBlockers(b, childIndex); len(blockers) > 0 {
		ids := make([]string, 0, len(blockers))
		details := make([]string, 0, len(blockers))
		for _, blk := range blockers {
			ids = append(ids, blk.ID)
			details = append(details, fmt.Sprintf("%s(status=%s)", blk.ID, blk.Status))
		}
		return BlockReason{
			Code:     ReasonDepBlockedBy,
			Detail:   "dep_blocked_by:" + strings.Join(details, ","),
			CitedIDs: ids,
		}
	}

	// 7. genuinely_needs_decomposition
	if isEpicBead(b) && total == 0 && !hasDecompositionOverrideLabel(b) {
		return BlockReason{
			Code:   ReasonGenuinelyNeedsDecomposition,
			Detail: "epic with no children and no manual-hold/no-auto-decompose/container override",
		}
	}

	// 8. parent_child_state_conflict
	if msg := parentChildConflict(b, childIndex); msg != "" {
		return BlockReason{
			Code:   ReasonParentChildStateConflict,
			Detail: msg,
		}
	}

	// 9. claimed_in_progress
	if b.Status == StatusInProgress {
		return BlockReason{
			Code:   ReasonClaimedInProgress,
			Detail: "bead currently claimed by an active worker",
		}
	}

	// 10. provider_route_unavailable
	if isProviderRouteUnavailable(b) {
		return BlockReason{
			Code:   ReasonProviderRouteUnavailable,
			Detail: "last dispatch: " + providerRouteHint(b),
		}
	}

	// 11. gated_by_budget_or_cooldown
	if gate := budgetOrCooldownGate(b); gate != "" {
		return BlockReason{
			Code:   ReasonGatedByBudgetOrCooldown,
			Detail: gate,
		}
	}

	// 12. malformed_parent_or_dep_ref
	if msg, cited := malformedRef(b, childIndex); msg != "" {
		return BlockReason{
			Code:     ReasonMalformedParentOrDepRef,
			Detail:   msg,
			CitedIDs: cited,
		}
	}

	// 13. dependency_cycle
	if cycle := detectDepCycle(b.ID, depIndex); len(cycle) > 0 {
		return BlockReason{
			Code:     ReasonDependencyCycle,
			Detail:   "cycle: " + strings.Join(cycle, "->"),
			CitedIDs: cycle,
		}
	}

	// 14. closed_or_missing_parent
	if b.Parent != "" {
		if parent, ok := lookupBead(childIndex, b.Parent); ok && parent.Status == StatusClosed {
			return BlockReason{
				Code:     ReasonClosedOrMissingParent,
				Detail:   fmt.Sprintf("parent %s is closed", b.Parent),
				CitedIDs: []string{b.Parent},
			}
		}
	}

	// 15. stale_graph_index
	if msg := staleIndexDisagreement(b, childIndex, openChildIndex); msg != "" {
		return BlockReason{
			Code:   ReasonStaleGraphIndex,
			Detail: msg,
		}
	}

	// 16. auto_remediation_exhausted
	if isAutoRemediationExhausted(b) {
		return BlockReason{
			Code:   ReasonAutoRemediationExhausted,
			Detail: "auto-remediation attempts cap hit or cooldown active",
		}
	}

	// 17. no_diagnosis (fallback)
	return BlockReason{
		Code:   ReasonNoDiagnosis,
		Detail: "no diagnosis matched; surface as bug",
	}
}

// supersedingTargetID returns the ID of the bead that supersedes b, either
// from the canonical `superseded-by` extra field or from a
// `superseded-by:<id>` label. Empty string means not superseded.
func supersedingTargetID(b Bead) string {
	if id := lifecycleSupersededBy(b); id != "" {
		return id
	}
	for _, l := range b.Labels {
		if strings.HasPrefix(l, "superseded-by:") {
			if id := strings.TrimSpace(strings.TrimPrefix(l, "superseded-by:")); id != "" {
				return id
			}
		}
	}
	return ""
}

// lookupBead scans the childIndex (which holds every bead, keyed by parent
// ID with roots under "") for the bead with the given ID.
func lookupBead(childIndex map[string][]Bead, id string) (Bead, bool) {
	if id == "" {
		return Bead{}, false
	}
	for _, beads := range childIndex {
		for _, b := range beads {
			if b.ID == id {
				return b, true
			}
		}
	}
	return Bead{}, false
}

func beadIDsFrom(beads []Bead) []string {
	out := make([]string, 0, len(beads))
	for _, b := range beads {
		out = append(out, b.ID)
	}
	sort.Strings(out)
	return out
}

// isEpicBead matches the bead-container classifier used by the lifecycle
// queue: explicit issue_type==epic or a title prefix of "epic:" or "epic ".
func isEpicBead(b Bead) bool {
	issueType := strings.ToLower(strings.TrimSpace(b.IssueType))
	if issueType == "epic" {
		return true
	}
	title := strings.ToLower(strings.TrimSpace(b.Title))
	return strings.HasPrefix(title, "epic:") || strings.HasPrefix(title, "epic ")
}

func allEpicBeads(beads []Bead) bool {
	if len(beads) == 0 {
		return false
	}
	for _, b := range beads {
		if !isEpicBead(b) {
			return false
		}
	}
	return true
}

func hasDecompositionOverrideLabel(b Bead) bool {
	for _, l := range b.Labels {
		switch l {
		case "manual-hold", "no-auto-decompose", "container":
			return true
		}
	}
	return false
}

// openDepBlockers returns the dep-target beads that are not yet closed.
// A dep whose target is absent from the index is NOT treated as a blocker
// here; that case is classified by malformed_parent_or_dep_ref later.
func openDepBlockers(b Bead, childIndex map[string][]Bead) []Bead {
	var out []Bead
	for _, d := range b.Dependencies {
		if d.DependsOnID == "" {
			continue
		}
		target, ok := lookupBead(childIndex, d.DependsOnID)
		if !ok {
			continue
		}
		if target.Status != StatusClosed && target.Status != StatusCancelled {
			out = append(out, target)
		}
	}
	return out
}

// parentChildConflict detects state mismatches the classifier does not
// expect: e.g., a child whose parent is closed while the child is open and
// not eligible, or a bead that is itself closed but appears in the
// open-child index for its parent.
func parentChildConflict(b Bead, childIndex map[string][]Bead) string {
	if b.Parent == "" {
		return ""
	}
	parent, ok := lookupBead(childIndex, b.Parent)
	if !ok {
		return ""
	}
	if b.Status == StatusClosed && parent.Status != StatusClosed {
		eligible, set := lifecycleExecutionEligible(parent)
		parentMarkedDead := set && !eligible
		if !parentMarkedDead {
			return fmt.Sprintf("child %s closed but parent %s open and execution-eligible", b.ID, parent.ID)
		}
	}
	if b.Status == StatusInProgress && parent.Status == StatusClosed {
		return fmt.Sprintf("bead %s in_progress but parent %s already closed", b.ID, parent.ID)
	}
	return ""
}

func isProviderRouteUnavailable(b Bead) bool {
	last := extraStringVal(b.Extra, ExtraLastStatus)
	detail := extraStringVal(b.Extra, ExtraLastDetail)
	if strings.Contains(strings.ToLower(last), "no live provider") {
		return true
	}
	if strings.Contains(strings.ToLower(detail), "no live provider") {
		return true
	}
	if strings.Contains(detail, "ResolveRoute:") {
		return true
	}
	return false
}

func providerRouteHint(b Bead) string {
	if d := extraStringVal(b.Extra, ExtraLastDetail); d != "" {
		return d
	}
	return extraStringVal(b.Extra, ExtraLastStatus)
}

// budgetOrCooldownGate returns a non-empty descriptor when a retry-after
// cooldown or auto-remediation budget gate is recorded on the bead.
func budgetOrCooldownGate(b Bead) string {
	if v := extraStringVal(b.Extra, ExtraRetryAfter); v != "" {
		return "retry-after cooldown active until " + v
	}
	if v := extraStringVal(b.Extra, "auto-remediation-gate"); v != "" {
		return "auto-remediation gated: " + v
	}
	return ""
}

func malformedRef(b Bead, childIndex map[string][]Bead) (string, []string) {
	if b.Parent != "" {
		if _, ok := lookupBead(childIndex, b.Parent); !ok {
			return fmt.Sprintf("parent %q does not resolve in index", b.Parent), []string{b.Parent}
		}
	}
	for _, d := range b.Dependencies {
		if strings.TrimSpace(d.DependsOnID) == "" {
			return "dependency entry has empty target id", nil
		}
		if _, ok := lookupBead(childIndex, d.DependsOnID); !ok {
			return fmt.Sprintf("dependency target %q does not resolve in index", d.DependsOnID), []string{d.DependsOnID}
		}
	}
	return "", nil
}

// detectDepCycle returns the IDs of a cycle reachable from start through
// the dep DAG (depIndex maps bead_id -> outgoing deps). Returns nil if no
// cycle reaches back to start.
func detectDepCycle(start string, depIndex map[string][]Dependency) []string {
	if start == "" || len(depIndex) == 0 {
		return nil
	}
	visited := map[string]bool{}
	var path []string
	var dfs func(node string) []string
	dfs = func(node string) []string {
		for _, p := range path {
			if p == node {
				idx := 0
				for i, q := range path {
					if q == node {
						idx = i
						break
					}
				}
				cyc := append([]string(nil), path[idx:]...)
				cyc = append(cyc, node)
				return cyc
			}
		}
		if visited[node] {
			return nil
		}
		visited[node] = true
		path = append(path, node)
		for _, d := range depIndex[node] {
			if d.DependsOnID == "" {
				continue
			}
			if cyc := dfs(d.DependsOnID); cyc != nil {
				return cyc
			}
		}
		path = path[:len(path)-1]
		return nil
	}
	return dfs(start)
}

// staleIndexDisagreement reports when childIndex and openChildIndex are
// inconsistent for b: an entry appears in openChildIndex but its Status is
// closed/cancelled, or an open child in childIndex is missing from the
// open-child bucket.
func staleIndexDisagreement(b Bead, childIndex, openChildIndex map[string][]Bead) string {
	open := openChildIndex[b.ID]
	all := childIndex[b.ID]
	for _, c := range open {
		if c.Status == StatusClosed || c.Status == StatusCancelled {
			return fmt.Sprintf("open-child index lists %s with closed status", c.ID)
		}
	}
	openIDs := map[string]bool{}
	for _, c := range open {
		openIDs[c.ID] = true
	}
	for _, c := range all {
		isOpenStatus := c.Status != StatusClosed && c.Status != StatusCancelled
		if isOpenStatus && !openIDs[c.ID] {
			return fmt.Sprintf("child %s has open status but missing from open-child index", c.ID)
		}
	}
	return ""
}

func isAutoRemediationExhausted(b Bead) bool {
	if extraStringVal(b.Extra, "auto-remediation-exhausted") == "true" {
		return true
	}
	for _, l := range b.Labels {
		if l == "auto-remediation:exhausted" {
			return true
		}
	}
	return false
}
