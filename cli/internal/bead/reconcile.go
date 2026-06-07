package bead

import (
	"context"
	"strings"
	"time"
)

const (
	ExtraRetryAfter      = "work-retry-after"
	ExtraLastStatus      = "work-last-status"
	ExtraLastDetail      = "work-last-detail"
	ExtraNoChangesCount  = "work-no-changes-count"
	ExtraCooldownBaseRev = "work-cooldown-base-rev"
	ExtraExecutionElig   = "execution-eligible"
	ExtraExecutionReason = "execution-skip-reason"

	ExtraNeedsHumanReason          = "needs-human-reason"
	ExtraNeedsHumanSince           = "needs-human-since"
	ExtraNeedsHumanSource          = "needs-human-source"
	ExtraNeedsHumanSuggestedAction = "needs-human-suggested-action"
	ExtraNeedsHumanSummary         = "needs-human-summary"

	LabelNoChangesUnverified      = "triage:no-changes-unverified"
	LabelNoChangesUnjustified     = "triage:no-changes-unjustified"
	LabelNeedsInvestigation       = "triage:needs-investigation"
	LabelNeedsHuman               = "needs_human"
	LabelReconciledNoChangesState = "reconciled:no-changes-state"
)

// NeedsHumanMeta holds operator-attention metadata for a bead in the needs_human lane.
type NeedsHumanMeta struct {
	Reason          string `json:"reason,omitempty"`
	Since           string `json:"since,omitempty"`
	Source          string `json:"source,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
	Summary         string `json:"summary,omitempty"`
}

// GetNeedsHumanMeta reads needs-human metadata from a bead's Extra map.
func GetNeedsHumanMeta(b Bead) NeedsHumanMeta {
	if b.Extra == nil {
		return NeedsHumanMeta{}
	}
	return NeedsHumanMeta{
		Reason:          extraStringVal(b.Extra, ExtraNeedsHumanReason),
		Since:           extraStringVal(b.Extra, ExtraNeedsHumanSince),
		Source:          extraStringVal(b.Extra, ExtraNeedsHumanSource),
		SuggestedAction: extraStringVal(b.Extra, ExtraNeedsHumanSuggestedAction),
		Summary:         extraStringVal(b.Extra, ExtraNeedsHumanSummary),
	}
}

// SetNeedsHumanMeta writes needs-human metadata into a bead's Extra map.
// Empty string values delete the corresponding key.
func SetNeedsHumanMeta(b *Bead, m NeedsHumanMeta) {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	setOrDeleteExtra(b.Extra, ExtraNeedsHumanReason, m.Reason)
	setOrDeleteExtra(b.Extra, ExtraNeedsHumanSince, m.Since)
	setOrDeleteExtra(b.Extra, ExtraNeedsHumanSource, m.Source)
	setOrDeleteExtra(b.Extra, ExtraNeedsHumanSuggestedAction, m.SuggestedAction)
	setOrDeleteExtra(b.Extra, ExtraNeedsHumanSummary, m.Summary)
}

func extraStringVal(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func setOrDeleteExtra(m map[string]any, key, val string) {
	if val == "" {
		delete(m, key)
	} else {
		m[key] = val
	}
}

var noChangesManagementKeys = []string{
	ExtraRetryAfter,
	ExtraCooldownBaseRev,
	ExtraLastStatus,
	ExtraLastDetail,
	ExtraNoChangesCount,
}

var noChangesTriageLabels = []string{
	LabelNoChangesUnverified,
	LabelNoChangesUnjustified,
}

// ReconcileOptions controls stale no_changes lifecycle reconciliation.
type ReconcileOptions struct {
	Apply bool
	Now   time.Time
	IDs   []string
}

// ReconcilePlan is a single proposed or applied lifecycle repair.
type ReconcilePlan struct {
	BeadID         string         `json:"bead_id"`
	Reason         string         `json:"reason"`
	CurrentFields  map[string]any `json:"current_fields,omitempty"`
	SetFields      map[string]any `json:"set_fields,omitempty"`
	ClearFields    []string       `json:"clear_fields,omitempty"`
	RemoveLabels   []string       `json:"remove_labels,omitempty"`
	AddLabels      []string       `json:"add_labels,omitempty"`
	TargetStatus   string         `json:"target_status,omitempty"`
	CloseSatisfied bool           `json:"close_satisfied,omitempty"`
	Applied        bool           `json:"applied"`
}

// ReconcileLifecycleMetadata returns conservative repairs for stale
// no_changes management fields. With Apply=false it is a dry run; with
// Apply=true it mutates through the Store API and appends reconciliation
// evidence.
func (s *Store) ReconcileLifecycleMetadata(opts ReconcileOptions) ([]ReconcilePlan, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	beads, err := s.ReadAll(context.Background())
	if err != nil {
		return nil, err
	}
	childrenByParent := make(map[string]int)
	nonTerminalChildrenByParent := make(map[string]int)
	closedChildrenByParent := make(map[string]int)
	idFilter := make(map[string]bool, len(opts.IDs))
	for _, id := range opts.IDs {
		if strings.TrimSpace(id) != "" {
			idFilter[strings.TrimSpace(id)] = true
		}
	}
	for _, b := range beads {
		if b.Parent != "" {
			childrenByParent[b.Parent]++
			if b.Status != StatusClosed && b.Status != StatusCancelled {
				nonTerminalChildrenByParent[b.Parent]++
			}
			if b.Status == StatusClosed {
				closedChildrenByParent[b.Parent]++
			}
		}
	}

	var plans []ReconcilePlan
	for _, b := range beads {
		if len(idFilter) > 0 && !idFilter[b.ID] {
			continue
		}
		events, _ := s.eventsForBead(&b)
		if p, ok := planLifecycleReconcile(b, events, childrenByParent[b.ID], nonTerminalChildrenByParent[b.ID], closedChildrenByParent[b.ID], opts.Now); ok {
			if opts.Apply {
				if err := s.applyReconcilePlan(p); err != nil {
					return plans, err
				}
				p.Applied = true
			}
			plans = append(plans, p)
		}
	}
	return plans, nil
}

func planLifecycleReconcile(b Bead, events []BeadEvent, childCount, nonTerminalChildCount, closedChildCount int, now time.Time) (ReconcilePlan, bool) {
	p := ReconcilePlan{
		BeadID:        b.ID,
		CurrentFields: currentLifecycleFields(b),
	}
	latest := latestLifecycleEvent(events)

	if b.Status == StatusClosed && hasClosedNoChangesResidue(b) {
		p.Reason = "closed bead has stale no_changes lifecycle metadata"
		p.ClearFields = noChangesFieldsPresent(b)
		p.RemoveLabels = noChangesLabelsPresent(b)
		return p, len(p.ClearFields) > 0 || len(p.RemoveLabels) > 0
	}

	if isSuccessEvent(latest) && hasNoChangesResidue(b) {
		p.Reason = "latest terminal success supersedes stale no_changes lifecycle metadata"
		p.ClearFields = noChangesFieldsPresent(b)
		p.RemoveLabels = noChangesLabelsPresent(b)
		return p, len(p.ClearFields) > 0 || len(p.RemoveLabels) > 0
	}

	if latest.Kind == "no_changes_verified" && b.Status != StatusClosed {
		p.Reason = "verified no_changes evidence closes as already_satisfied"
		p.ClearFields = noChangesFieldsPresent(b)
		p.RemoveLabels = noChangesLabelsPresent(b)
		p.TargetStatus = StatusClosed
		p.CloseSatisfied = true
		return p, true
	}

	if isExpiredRetryAfter(b, now) {
		p.Reason = "expired retry-after metadata no longer blocks execution"
		p.ClearFields = []string{ExtraRetryAfter}
		return p, true
	}

	if isEpicBead(b) && childCount > 0 && nonTerminalChildCount == 0 && b.Status != StatusClosed && b.Status != StatusCancelled {
		if closedChildCount == 0 {
			p.Reason = "auto-cancel epic: all children cancelled, no work completed"
			p.TargetStatus = StatusCancelled
			p.CloseSatisfied = false
			return p, true
		}
		p.Reason = "auto-close epic: all children reached terminal state"
		p.TargetStatus = StatusClosed
		p.CloseSatisfied = true
		return p, true
	}

	if isContainerBead(b, childCount) && noChangesCount(b) > 0 {
		if b.Extra != nil {
			if eligible, ok := b.Extra[ExtraExecutionElig].(bool); ok && !eligible {
				return ReconcilePlan{}, false
			}
		}
		p.Reason = "parent/epic work is owned by child beads"
		p.SetFields = map[string]any{
			ExtraExecutionElig:   false,
			ExtraExecutionReason: "parent/epic container; execute child beads first",
		}
		return p, true
	}

	return ReconcilePlan{}, false
}

func (s *Store) applyReconcilePlan(p ReconcilePlan) error {
	mutate := func(b *Bead) error {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		for _, key := range p.ClearFields {
			delete(b.Extra, key)
		}
		for key, value := range p.SetFields {
			b.Extra[key] = value
		}
		for _, label := range p.RemoveLabels {
			removeLabel(b, label)
		}
		for _, label := range p.AddLabels {
			addLabel(b, label)
		}
		addLabel(b, LabelReconciledNoChangesState)
		appendInlineEvent(b, BeadEvent{
			Kind:      "lifecycle_reconciled",
			Summary:   p.Reason,
			Actor:     "ddx",
			Source:    "ddx bead reconcile",
			CreatedAt: time.Now().UTC(),
		})
		if strings.HasPrefix(p.Reason, "auto-close epic:") || strings.HasPrefix(p.Reason, "auto-cancel epic:") {
			event := BeadEvent{
				Kind:      "epic_auto_close",
				Actor:     "ddx",
				Source:    "ddx bead reconcile",
				CreatedAt: time.Now().UTC(),
			}
			if p.TargetStatus == StatusCancelled {
				event.Summary = "auto-cancelled: all children cancelled, no work completed"
				event.Body = "closed_because: all_children_cancelled"
			} else {
				event.Summary = "auto-closed: all children reached terminal state"
				event.Body = "closed_because: all_children_terminal"
			}
			appendInlineEvent(b, event)
		}
		return nil
	}
	var err error
	if p.TargetStatus != "" {
		// TD-031 §3.1: reconcile terminal transitions are meta-transitions that
		// intentionally bypass ClosureGate — the bead has no execution session and
		// no closing_commit_sha of its own. The bypass is safe because every
		// transitive dependency is terminal, so the parent's transition inherits
		// evidence by reference through dependency edges.
		err = s.UpdateWithLifecycleStatus(p.BeadID, p.TargetStatus, LifecycleTransitionOptions{
			ManualClose: true,
			Reason:      p.Reason,
			Actor:       "ddx",
			Source:      "ddx bead reconcile",
		}, mutate)
	} else {
		err = s.Update(context.Background(), p.BeadID, func(b *Bead) {
			_ = mutate(b)
		})
	}
	if err != nil {
		return err
	}
	if p.TargetStatus != "" {
		return s.externalizeEvents(p.BeadID)
	}
	return nil
}

func appendInlineEvent(b *Bead, event BeadEvent) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	events := decodeBeadEvents(b.Extra["events"])
	events = append(events, event)
	b.Extra["events"] = encodeEventsForExtra(events)
}

func currentLifecycleFields(b Bead) map[string]any {
	out := make(map[string]any)
	for _, key := range noChangesManagementKeys {
		if b.Extra != nil {
			if v, ok := b.Extra[key]; ok {
				out[key] = v
			}
		}
	}
	for _, label := range append(noChangesTriageLabels, LabelNeedsInvestigation) {
		if hasLabel(b, label) {
			out["label:"+label] = true
		}
	}
	return out
}

func latestLifecycleEvent(events []BeadEvent) BeadEvent {
	var latest BeadEvent
	for _, e := range events {
		if strings.HasPrefix(e.Kind, "no_changes_") || e.Kind == "execute-bead" {
			latest = e
		}
	}
	return latest
}

func isSuccessEvent(e BeadEvent) bool {
	if e.Kind != "execute-bead" {
		return false
	}
	summary := strings.TrimSpace(e.Summary)
	return summary == "success" || summary == "already_satisfied"
}

func hasNoChangesResidue(b Bead) bool {
	return len(noChangesFieldsPresent(b)) > 0 || len(noChangesLabelsPresent(b)) > 0
}

func hasClosedNoChangesResidue(b Bead) bool {
	return len(noChangesLabelsPresent(b)) > 0 || hasAnyExtra(b, ExtraRetryAfter, ExtraLastStatus, ExtraLastDetail)
}

func noChangesFieldsPresent(b Bead) []string {
	var keys []string
	for _, key := range noChangesManagementKeys {
		if b.Extra != nil {
			if _, ok := b.Extra[key]; ok {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func noChangesLabelsPresent(b Bead) []string {
	var labels []string
	for _, label := range noChangesTriageLabels {
		if hasLabel(b, label) {
			labels = append(labels, label)
		}
	}
	return labels
}

func hasAnyExtra(b Bead, keys ...string) bool {
	for _, key := range keys {
		if b.Extra != nil {
			if _, ok := b.Extra[key]; ok {
				return true
			}
		}
	}
	return false
}

func isExpiredRetryAfter(b Bead, now time.Time) bool {
	raw, ok := b.Extra[ExtraRetryAfter].(string)
	if !ok || raw == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, raw)
	return err == nil && !t.After(now)
}

func noChangesCount(b Bead) int {
	if b.Extra == nil {
		return 0
	}
	switch v := b.Extra[ExtraNoChangesCount].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func isContainerBead(b Bead, childCount int) bool {
	if childCount == 0 {
		return false
	}
	return b.IssueType == "epic" || strings.HasPrefix(strings.ToLower(b.Title), "epic:")
}

func hasLabel(b Bead, label string) bool {
	for _, existing := range b.Labels {
		if existing == label {
			return true
		}
	}
	return false
}

func addLabel(b *Bead, label string) {
	for _, existing := range b.Labels {
		if existing == label {
			return
		}
	}
	b.Labels = append(b.Labels, label)
}

func removeLabel(b *Bead, label string) {
	out := b.Labels[:0]
	for _, existing := range b.Labels {
		if existing != label {
			out = append(out, existing)
		}
	}
	b.Labels = out
}
