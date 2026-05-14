package bead

import (
	"context"
	"strings"
	"time"
)

const (
	ExtraRetryAfter      = "execute-loop-retry-after"
	ExtraLastStatus      = "execute-loop-last-status"
	ExtraLastDetail      = "execute-loop-last-detail"
	ExtraNoChangesCount  = "execute-loop-no-changes-count"
	ExtraCooldownBaseRev = "execute-loop-cooldown-base-rev"
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
	idFilter := make(map[string]bool, len(opts.IDs))
	for _, id := range opts.IDs {
		if strings.TrimSpace(id) != "" {
			idFilter[strings.TrimSpace(id)] = true
		}
	}
	for _, b := range beads {
		if b.Parent != "" {
			childrenByParent[b.Parent]++
		}
	}

	var plans []ReconcilePlan
	for _, b := range beads {
		if len(idFilter) > 0 && !idFilter[b.ID] {
			continue
		}
		events, _ := s.eventsForBead(&b)
		if p, ok := planLifecycleReconcile(b, events, childrenByParent[b.ID], opts.Now); ok {
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

func planLifecycleReconcile(b Bead, events []BeadEvent, childCount int, now time.Time) (ReconcilePlan, bool) {
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
		p.CloseSatisfied = true
		return p, true
	}

	if isExpiredRetryAfter(b, now) {
		p.Reason = "expired retry-after metadata no longer blocks execution"
		p.ClearFields = []string{ExtraRetryAfter}
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
		return nil
	}
	var err error
	// TD-031 §5: reconcile-close is a meta-close that intentionally bypasses
	// ClosureGate — the bead has no execution session and no closing_commit_sha
	// of its own; every transitive dependency is closed, supplying evidence by reference.
	if p.CloseSatisfied {
		err = s.UpdateWithLifecycleStatus(p.BeadID, StatusClosed, LifecycleTransitionOptions{
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
	if p.CloseSatisfied {
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

func isNoViableProviderEvent(e BeadEvent) bool {
	if e.Kind != "execute-bead" || strings.TrimSpace(e.Summary) != "execution_failed" {
		return false
	}
	body := strings.ToLower(e.Body)
	return strings.Contains(body, "no viable provider") ||
		strings.Contains(body, "all powerClasses exhausted")
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

func hasRetryAfter(b Bead) bool {
	return hasAnyExtra(b, ExtraRetryAfter)
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
