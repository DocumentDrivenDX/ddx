package bead

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateStats reports what Migrate did in a single pass.
type MigrateStats struct {
	// EventsExternalized is the number of beads whose inline events were
	// moved to the .ddx/attachments/<id>/events.jsonl sidecar.
	EventsExternalized int
	// EventRecordsExternalized is the total number of individual event rows
	// moved out of inline bead storage and into attachment sidecars.
	EventRecordsExternalized int
	// AttachmentsTouched is the number of per-bead attachment files written.
	AttachmentsTouched int
	// Archived is the number of closed beads moved from the active
	// collection to beads-archive.
	Archived int
}

// Changed reports whether Migrate mutated on-disk state in this pass.
func (m MigrateStats) Changed() bool {
	return m.EventsExternalized > 0 || m.Archived > 0
}

const (
	// LifecycleSchemaMarkerFile is written after the one-way lifecycle
	// migration has converted legacy queue-driving labels into status-owned
	// state. The startup gate added by a later bead can use this sentinel
	// without scanning for historical transient metadata on every launch.
	LifecycleSchemaMarkerFile    = "lifecycle-schema.json"
	LifecycleSchemaMarkerVersion = 1
)

// LifecycleMigrationStats reports what the one-way lifecycle migration did or
// would do. Target counts are row counts by post-migration persisted status.
type LifecycleMigrationStats struct {
	LegacyNeedsHumanLabels                 int  `json:"legacy_needs_human_labels"`
	LegacyNeedsInvestigationLabels         int  `json:"legacy_needs_investigation_labels"`
	LegacyNeedsInvestigationPseudoStatuses int  `json:"legacy_needs_investigation_pseudo_statuses"`
	LegacyNoChangesMetadataRows            int  `json:"legacy_no_changes_metadata_rows"`
	ToProposed                             int  `json:"to_proposed"`
	ToOpen                                 int  `json:"to_open"`
	ToBlocked                              int  `json:"to_blocked"`
	ToClosed                               int  `json:"to_closed"`
	ToCancelled                            int  `json:"to_cancelled"`
	RowsChanged                            int  `json:"rows_changed"`
	SchemaMarkerMissing                    bool `json:"schema_marker_missing"`
	MarkerWritten                          bool `json:"marker_written,omitempty"`
	DryRun                                 bool `json:"dry_run,omitempty"`
}

// Changed reports whether MigrateLifecycle mutated durable state.
func (m LifecycleMigrationStats) Changed() bool {
	return m.RowsChanged > 0 || m.MarkerWritten
}

type lifecycleMigrationPlan struct {
	index                 int
	fromStatus            string
	targetStatus          string
	reason                string
	externalBlockerReason string
	removeLabels          []string
	clearFields           []string
}

// LifecycleSchemaMarkerPath returns the marker file path under the store's
// .ddx directory.
func (s *Store) LifecycleSchemaMarkerPath() string {
	return filepath.Join(s.Dir, LifecycleSchemaMarkerFile)
}

// HasLifecycleSchemaMarker reports whether the lifecycle migration sentinel is
// present.
func (s *Store) HasLifecycleSchemaMarker() (bool, error) {
	_, err := os.Stat(s.LifecycleSchemaMarkerPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// WriteLifecycleSchemaMarker records the lifecycle migration sentinel.
func (s *Store) WriteLifecycleSchemaMarker(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: lifecycle marker mkdir: %w", err)
	}
	payload := map[string]any{
		"schema":     "bead-lifecycle",
		"version":    LifecycleSchemaMarkerVersion,
		"applied_at": now.UTC().Format(time.RFC3339),
		"source":     "ddx bead migrate --lifecycle",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("bead: lifecycle marker marshal: %w", err)
	}
	data = append(data, '\n')
	return writeAtomicFile(s.LifecycleSchemaMarkerPath(), data)
}

func (s *Store) migrateLifecycle(apply bool, now time.Time) (LifecycleMigrationStats, error) {
	var stats LifecycleMigrationStats
	stats.DryRun = !apply
	if s.Collection != DefaultCollection {
		return stats, fmt.Errorf("bead: lifecycle migration only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}
	markerPresent, err := s.HasLifecycleSchemaMarker()
	if err != nil {
		return stats, fmt.Errorf("bead: lifecycle marker: %w", err)
	}
	stats.SchemaMarkerMissing = !markerPresent

	var plans []lifecycleMigrationPlan
	err = s.WithLock(func() error {
		beads, _, rerr := s.readAllLatestRaw()
		if rerr != nil {
			return rerr
		}
		plans = planLifecycleMigrations(beads, &stats)
		if !apply {
			return nil
		}
		for _, plan := range plans {
			if err := applyLifecycleMigrationPlan(&beads[plan.index], plan, now); err != nil {
				return err
			}
			if err := s.validateBead(&beads[plan.index]); err != nil {
				return err
			}
		}
		if len(plans) > 0 {
			if err := s.writeAllLocked(beads); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	stats.RowsChanged = len(plans)
	if apply && stats.SchemaMarkerMissing {
		if err := s.WriteLifecycleSchemaMarker(now); err != nil {
			return stats, err
		}
		stats.MarkerWritten = true
	}
	return stats, nil
}

func planLifecycleMigrations(beads []Bead, stats *LifecycleMigrationStats) []lifecycleMigrationPlan {
	var plans []lifecycleMigrationPlan
	for i, b := range beads {
		plan, ok := planLifecycleMigration(i, b)
		if !ok {
			continue
		}
		countLifecycleMigrationFacts(b, stats)
		countLifecycleMigrationTarget(plan.targetStatus, stats)
		plans = append(plans, plan)
	}
	return plans
}

func planLifecycleMigration(index int, b Bead) (lifecycleMigrationPlan, bool) {
	events := decodeBeadEvents(b.Extra["events"])
	latest := latestLifecycleEvent(events)
	legacyLabels := legacyLifecycleLabelsPresent(b)
	legacyFields := noChangesFieldsPresent(b)
	pseudoStatus := !IsCanonicalStatus(b.Status)
	latestNeedsMigration := latest.Kind == "no_changes_needs_investigation" || latest.Kind == "no_changes_blocked"
	needsMigration := pseudoStatus || len(legacyLabels) > 0 || len(legacyFields) > 0 || latestNeedsMigration || blockedMissingExternalReason(b)
	if !needsMigration {
		return lifecycleMigrationPlan{}, false
	}

	target := canonicalLifecycleMigrationBaseStatus(b.Status)
	reason := ""
	externalReason := ""

	if isTerminalLifecycleStatus(target) {
		reason = "terminal bead legacy lifecycle metadata archived as explanatory history"
	} else if external, why := lifecycleMigrationExternalBlocker(b, latest); external {
		target = StatusBlocked
		externalReason = why
		reason = "legacy lifecycle metadata represented an external recheckable blocker"
	} else if lifecycleMigrationOperatorRequired(b, latest) {
		target = StatusProposed
		reason = "legacy lifecycle metadata required operator attention"
	} else if target == StatusOpen {
		target = StatusOpen
		reason = "legacy lifecycle metadata was retryable autonomous work"
	} else {
		reason = "legacy lifecycle metadata removed without lifecycle status change"
	}

	return lifecycleMigrationPlan{
		index:                 index,
		fromStatus:            b.Status,
		targetStatus:          target,
		reason:                reason,
		externalBlockerReason: externalReason,
		removeLabels:          legacyLabels,
		clearFields:           legacyFields,
	}, true
}

func countLifecycleMigrationFacts(b Bead, stats *LifecycleMigrationStats) {
	if hasLabel(b, LabelNeedsHuman) {
		stats.LegacyNeedsHumanLabels++
	}
	if hasLabel(b, LabelNeedsInvestigation) {
		stats.LegacyNeedsInvestigationLabels++
	}
	if b.Status == "needs_investigation" {
		stats.LegacyNeedsInvestigationPseudoStatuses++
	}
	if len(noChangesFieldsPresent(b)) > 0 {
		stats.LegacyNoChangesMetadataRows++
	}
}

func countLifecycleMigrationTarget(status string, stats *LifecycleMigrationStats) {
	switch status {
	case StatusProposed:
		stats.ToProposed++
	case StatusOpen:
		stats.ToOpen++
	case StatusBlocked:
		stats.ToBlocked++
	case StatusClosed:
		stats.ToClosed++
	case StatusCancelled:
		stats.ToCancelled++
	}
}

func applyLifecycleMigrationPlan(b *Bead, plan lifecycleMigrationPlan, now time.Time) error {
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	fromForTransition := b.Status
	if !IsCanonicalStatus(fromForTransition) {
		b.Status = StatusOpen
		fromForTransition = StatusOpen
	}
	opts := LifecycleTransitionOptions{
		OperatorRequired:      plan.targetStatus == StatusProposed,
		ExternalBlockerReason: plan.externalBlockerReason,
		Reason:                plan.reason,
		Actor:                 "ddx",
		Source:                "ddx bead migrate --lifecycle",
	}
	if b.Status != plan.targetStatus {
		if err := transitionLifecycleInPlace(b, plan.targetStatus, opts); err != nil {
			return err
		}
	} else if plan.targetStatus == StatusBlocked {
		applyLifecycleTransitionMetadata(b, StatusBlocked, StatusBlocked, opts)
	}
	if fromForTransition == StatusInProgress && b.Status != StatusInProgress {
		clearClaimMetadata(b)
	}

	for _, label := range plan.removeLabels {
		removeLabel(b, label)
	}
	for _, key := range plan.clearFields {
		delete(b.Extra, key)
	}
	if plan.targetStatus != StatusBlocked {
		delete(b.Extra, ExtraLifecycleExternalBlockerReason)
	}
	if plan.targetStatus == StatusProposed {
		meta := GetNeedsHumanMeta(*b)
		if strings.TrimSpace(meta.Reason) == "" {
			meta.Reason = plan.reason
		}
		if strings.TrimSpace(meta.Since) == "" {
			meta.Since = now.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(meta.Source) == "" {
			meta.Source = "ddx bead migrate --lifecycle"
		}
		if strings.TrimSpace(meta.SuggestedAction) == "" {
			meta.SuggestedAction = "review and accept, split, block, or cancel this proposed work"
		}
		SetNeedsHumanMeta(b, meta)
	}
	b.Extra["lifecycle-migrated-at"] = now.UTC().Format(time.RFC3339)
	b.Extra["lifecycle-migrated-from-status"] = plan.fromStatus
	b.Extra["lifecycle-migrated-to-status"] = plan.targetStatus
	appendInlineEvent(b, BeadEvent{
		Kind:      "lifecycle_migrated",
		Summary:   plan.reason,
		Body:      lifecycleMigrationEventBody(plan),
		Actor:     "ddx",
		Source:    "ddx bead migrate --lifecycle",
		CreatedAt: now.UTC(),
	})
	return nil
}

func lifecycleMigrationEventBody(plan lifecycleMigrationPlan) string {
	var parts []string
	parts = append(parts, "from_status="+plan.fromStatus)
	parts = append(parts, "to_status="+plan.targetStatus)
	if plan.externalBlockerReason != "" {
		parts = append(parts, "external_blocker_reason="+plan.externalBlockerReason)
	}
	if len(plan.removeLabels) > 0 {
		parts = append(parts, "removed_labels="+strings.Join(plan.removeLabels, ","))
	}
	if len(plan.clearFields) > 0 {
		parts = append(parts, "cleared_fields="+strings.Join(plan.clearFields, ","))
	}
	return strings.Join(parts, "\n")
}

func legacyLifecycleLabelsPresent(b Bead) []string {
	var labels []string
	for _, label := range []string{
		LabelNeedsHuman,
		LabelNeedsInvestigation,
		LabelNoChangesUnverified,
		LabelNoChangesUnjustified,
	} {
		if hasLabel(b, label) {
			labels = append(labels, label)
		}
	}
	return labels
}

func canonicalLifecycleMigrationBaseStatus(status string) string {
	if IsCanonicalStatus(status) {
		return status
	}
	switch status {
	case "needs_human", "needs_investigation":
		return StatusOpen
	default:
		return StatusOpen
	}
}

func isTerminalLifecycleStatus(status string) bool {
	return status == StatusClosed || status == StatusCancelled
}

func blockedMissingExternalReason(b Bead) bool {
	return b.Status == StatusBlocked && strings.TrimSpace(extraStringVal(b.Extra, ExtraLifecycleExternalBlockerReason)) == ""
}

func lifecycleMigrationExternalBlocker(b Bead, latest BeadEvent) (bool, string) {
	if b.Status == StatusBlocked {
		return true, firstNonEmpty(
			extraStringVal(b.Extra, ExtraLifecycleExternalBlockerReason),
			extraStringVal(b.Extra, ExtraLastDetail),
			latest.Body,
			"legacy blocked status",
		)
	}
	if latest.Kind == "no_changes_blocked" {
		return true, firstNonEmpty(latest.Body, latest.Summary, "legacy no_changes blocked outcome")
	}
	lastStatus := strings.ToLower(strings.TrimSpace(extraStringVal(b.Extra, ExtraLastStatus)))
	detail := firstNonEmpty(extraStringVal(b.Extra, ExtraLastDetail), latest.Body)
	if lastStatus == "blocked" || lastStatus == "external_blocked" {
		return true, firstNonEmpty(detail, "legacy blocked no_changes outcome")
	}
	lower := strings.ToLower(detail)
	for _, marker := range []string{
		"external blocker",
		"blocked on upstream",
		"blocked by upstream",
		"waiting on upstream",
		"requires upstream",
		"blocked by external",
	} {
		if strings.Contains(lower, marker) {
			return true, firstNonEmpty(detail, marker)
		}
	}
	return false, ""
}

func lifecycleMigrationOperatorRequired(b Bead, latest BeadEvent) bool {
	if hasLabel(b, LabelNeedsHuman) || b.Status == "needs_human" {
		return true
	}
	if hasAnyExtra(b, ExtraNeedsHumanReason, ExtraNeedsHumanSuggestedAction, ExtraNeedsHumanSummary) {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		extraStringVal(b.Extra, ExtraLastDetail),
		latest.Body,
		latest.Summary,
	}, "\n"))
	if latest.Kind == "no_changes_needs_investigation" && !containsSmartRunnableMarker(text) {
		return true
	}
	for _, marker := range []string{
		"operator",
		"human",
		"manual",
		"approval",
		"approve",
		"clarification",
		"decision",
		"non-scope",
		"not in scope",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsSmartRunnableMarker(text string) bool {
	for _, marker := range []string{
		"smart agent",
		"smarter agent",
		"stronger agent",
		"stronger model",
		"retry autonomously",
		"rerun",
		"re-run",
		"autonomous",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func clearClaimMetadata(b *Bead) {
	b.Owner = ""
	if b.Extra == nil {
		return
	}
	for _, key := range ClaimMetadataExtraKeys {
		delete(b.Extra, key)
	}
	delete(b.Extra, ClaimHeartbeatExtraKey)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// Migrate splits the existing active beads collection into the modern
// layout: closed beads' inline events are externalized to the attachment
// store (per ADR-004), and eligible closed beads are moved to the
// beads-archive partner (per TD-027). It is idempotent — a second call
// with no further changes is a no-op.
//
// Migrate uses a permissive archival policy (MinAge=0, MinActiveCount=0)
// so it drains the historical backlog. Routine archival (after Close)
// uses the policy assembled by `ddx bead archive` from its flag set.
func (s *Store) Migrate(ctx context.Context) (MigrateStats, error) {
	return s.ArchiveWithEvents(ctx, migratePolicy())
}

// migrateDryRun reports what Migrate would do without mutating disk. It uses
// the same eligibility logic as Migrate (Statuses=[closed], no MinAge floor,
// preserve_dependencies retention).
func (s *Store) migrateDryRun() (MigrateStats, error) {
	var stats MigrateStats
	if s.Collection != DefaultCollection {
		return stats, fmt.Errorf("bead: migrate dry-run only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}
	policy := migratePolicy()

	var beads []Bead
	err := s.WithLock(func() error {
		all, _, rerr := s.readAllLatestRaw()
		if rerr != nil {
			return rerr
		}
		beads = all
		return nil
	})
	if err != nil {
		return stats, err
	}

	referenced := make(map[string]bool)
	for _, b := range beads {
		if b.Status == StatusClosed {
			continue
		}
		for _, dep := range b.DepIDs() {
			referenced[dep] = true
		}
	}

	for _, b := range beads {
		if !containsString(policy.Statuses, b.Status) {
			continue
		}
		if hasInlineEvents(&b) {
			stats.EventsExternalized++
			stats.EventRecordsExternalized += inlineEventCount(&b)
			stats.AttachmentsTouched++
		}
		if referenced[b.ID] {
			continue
		}
		ts := b.UpdatedAt
		if raw, ok := b.Extra["closed_at"].(string); ok {
			if parsed, perr := time.Parse(time.RFC3339, raw); perr == nil {
				ts = parsed
			}
		}
		if policy.MinAge > 0 && time.Now().UTC().Sub(ts) < policy.MinAge {
			continue
		}
		stats.Archived++
	}
	return stats, nil
}

func migratePolicy() ArchivePolicy {
	return ArchivePolicy{
		Statuses:       []string{StatusClosed},
		MinAge:         0,
		MinActiveCount: 0,
		BatchSize:      0,
	}
}

// ArchiveWithEvents externalizes inline events for every bead whose status
// matches policy.Statuses, then archives eligible beads under the same
// policy. This is the operator-facing path used by `ddx bead archive` and
// the internal path used by `ddx bead migrate`.
func (s *Store) ArchiveWithEvents(ctx context.Context, policy ArchivePolicy) (MigrateStats, error) {
	var stats MigrateStats
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	if s.Collection != DefaultCollection {
		return stats, fmt.Errorf("bead: archive only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}

	// Step 1: externalize inline events on every eligible-status bead under
	// the active store's lock. This shrinks the row size before we try to
	// archive, so the archive partner inherits already-thin rows.
	err := s.WithLock(func() error {
		beads, _, rerr := s.readAllLatestRaw()
		if rerr != nil {
			return rerr
		}
		dirty := false
		for i := range beads {
			if !containsString(policy.Statuses, beads[i].Status) {
				continue
			}
			if !hasInlineEvents(&beads[i]) {
				continue
			}
			stats.EventsExternalized++
			stats.EventRecordsExternalized += inlineEventCount(&beads[i])
			stats.AttachmentsTouched++
			if eerr := s.externalizeEventsInPlace(&beads[i]); eerr != nil {
				return eerr
			}
			dirty = true
		}
		if !dirty {
			return nil
		}
		return s.writeAllLocked(beads)
	})
	if err != nil {
		return stats, fmt.Errorf("bead: archive externalize: %w", err)
	}

	moved, err := s.Archive(ctx, policy)
	if err != nil {
		return stats, fmt.Errorf("bead: archive: %w", err)
	}
	stats.Archived = len(moved)
	return stats, nil
}

// MigrateAxonStats reports what MigrateToAxon copied in a single pass.
type MigrateAxonStats struct {
	// BeadsMigrated is the total number of distinct bead IDs written into
	// the axon backend (active + archive, deduped on ID).
	BeadsMigrated int
	// EventsMigrated is the total number of inline events written into the
	// ddx_bead_events collection. Externalized events (referenced via
	// Extra[events_attachment]) are not counted because they are not
	// rewritten by the migration — the attachment file remains canonical.
	EventsMigrated int
	// AttachmentsMigrated is the number of externalized attachment sidecars
	// copied into the importer target tree during migration.
	AttachmentsMigrated int
}

// hasInlineEvents reports whether a bead carries any inline events that
// can still be externalized. An empty array counts as "no inline events"
// so a second migrate pass on an already-externalized bead is a no-op.
func hasInlineEvents(b *Bead) bool {
	if b == nil || b.Extra == nil {
		return false
	}
	raw, ok := b.Extra["events"]
	if !ok {
		return false
	}
	return len(decodeBeadEvents(raw)) > 0
}

func inlineEventCount(b *Bead) int {
	if b == nil || b.Extra == nil {
		return 0
	}
	raw, ok := b.Extra["events"]
	if !ok {
		return 0
	}
	return len(decodeBeadEvents(raw))
}
