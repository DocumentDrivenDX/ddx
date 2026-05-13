package bead

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lifecycleMigrateSeed(id, status string, labels []string, extra map[string]any) Bead {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return Bead{
		ID:        id,
		Title:     id,
		Status:    status,
		Priority:  1,
		IssueType: DefaultType,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Labels:    labels,
		Extra:     extra,
	}
}

func TestLifecycleMigrate_NeedsHumanLabelBecomesProposed(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		lifecycleMigrateSeed("ddx-human", StatusOpen, []string{LabelNeedsHuman, "area:cli"}, map[string]any{
			ExtraNeedsHumanReason: "review returned terminal BLOCK",
		}),
	}))

	stats, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LegacyNeedsHumanLabels)
	assert.Equal(t, 1, stats.ToProposed)
	assert.True(t, stats.MarkerWritten)

	got, err := s.Get(testCtx(), "ddx-human")
	require.NoError(t, err)
	assert.Equal(t, StatusProposed, got.Status)
	assert.NotContains(t, got.Labels, LabelNeedsHuman)
	assert.Contains(t, got.Labels, "area:cli")
	meta := GetNeedsHumanMeta(*got)
	assert.Equal(t, "review returned terminal BLOCK", meta.Reason)
	assert.Equal(t, "ddx bead migrate --lifecycle", meta.Source)

	events, err := s.Events(got.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, "lifecycle_migrated", events[len(events)-1].Kind)
	assert.FileExists(t, s.LifecycleSchemaMarkerPath())
}

func TestLifecycleMigrate_NeedsInvestigationSmartRunnableRemainsOpen(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		lifecycleMigrateSeed("ddx-smart", StatusOpen, []string{LabelNeedsInvestigation}, map[string]any{
			ExtraRetryAfter: "2026-01-01T00:00:00Z",
			ExtraLastStatus: "no_changes",
			ExtraLastDetail: "rerun with a smarter agent; autonomous work remains possible",
		}),
	}))

	stats, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LegacyNeedsInvestigationLabels)
	assert.Equal(t, 1, stats.LegacyNoChangesMetadataRows)
	assert.Equal(t, 1, stats.ToOpen)

	got, err := s.Get(testCtx(), "ddx-smart")
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.NotContains(t, got.Labels, LabelNeedsInvestigation)
	assert.NotContains(t, got.Extra, ExtraRetryAfter)
	assert.NotContains(t, got.Extra, ExtraLastStatus)
	assert.NotContains(t, got.Extra, ExtraLastDetail)

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, got.ID, ready[0].ID)
}

func TestLifecycleMigrate_NeedsInvestigationOperatorRequiredBecomesProposed(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		lifecycleMigrateSeed("ddx-operator", StatusOpen, []string{LabelNeedsInvestigation}, map[string]any{
			ExtraLastStatus: "no_changes",
			ExtraLastDetail: "operator decision required before this can proceed",
		}),
	}))

	stats, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ToProposed)

	got, err := s.Get(testCtx(), "ddx-operator")
	require.NoError(t, err)
	assert.Equal(t, StatusProposed, got.Status)
	assert.NotContains(t, got.Labels, LabelNeedsInvestigation)
	assert.Equal(t, "legacy lifecycle metadata required operator attention", GetNeedsHumanMeta(*got).Reason)
}

func TestLifecycleMigrate_ExternalBlockerBecomesBlocked(t *testing.T) {
	s := newTestStore(t)
	dep := lifecycleMigrateSeed("ddx-open-dep", StatusOpen, nil, nil)
	waiting := lifecycleMigrateSeed("ddx-waiting", StatusOpen, []string{LabelNeedsInvestigation}, map[string]any{
		ExtraLastStatus: "no_changes",
		ExtraLastDetail: "rerun with smart agent",
	})
	waiting.Dependencies = []Dependency{{IssueID: waiting.ID, DependsOnID: dep.ID, Type: "blocks"}}
	external := lifecycleMigrateSeed("ddx-external", StatusOpen, []string{LabelNeedsInvestigation}, map[string]any{
		ExtraLastStatus: "blocked",
		ExtraLastDetail: "blocked on upstream API credentials",
	})
	require.NoError(t, s.WriteAll([]Bead{dep, waiting, external}))

	stats, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.ToBlocked)
	assert.Equal(t, 1, stats.ToOpen)

	gotExternal, err := s.Get(testCtx(), external.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusBlocked, gotExternal.Status)
	assert.Equal(t, "blocked on upstream API credentials", gotExternal.Extra[ExtraLifecycleExternalBlockerReason])
	assert.NotContains(t, gotExternal.Labels, LabelNeedsInvestigation)

	gotWaiting, err := s.Get(testCtx(), waiting.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, gotWaiting.Status)
	blocked, err := s.BlockedAll()
	require.NoError(t, err)
	byID := map[string]BlockedBead{}
	for _, row := range blocked {
		byID[row.ID] = row
	}
	require.Contains(t, byID, waiting.ID)
	assert.Equal(t, BlockerKindDependency, byID[waiting.ID].Blocker.Kind)
	require.Contains(t, byID, external.ID)
	assert.Equal(t, BlockerKindBlockedStatus, byID[external.ID].Blocker.Kind)
}

func TestLifecycleMigrate_ApplyWritesSchemaMarkerAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		lifecycleMigrateSeed("ddx-pseudo", "needs_investigation", nil, map[string]any{
			ExtraLastDetail: "rerun with a stronger model",
		}),
	}))

	dry, err := s.MigrateLifecycleDryRun()
	require.NoError(t, err)
	assert.True(t, dry.DryRun)
	assert.True(t, dry.SchemaMarkerMissing)
	assert.Equal(t, 1, dry.LegacyNeedsInvestigationPseudoStatuses)
	assert.Equal(t, 1, dry.ToOpen)
	_, statErr := os.Stat(s.LifecycleSchemaMarkerPath())
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write marker")

	first, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.True(t, first.Changed())
	assert.Equal(t, 1, first.RowsChanged)
	assert.True(t, first.MarkerWritten)
	before, err := os.ReadFile(s.LifecycleSchemaMarkerPath())
	require.NoError(t, err)

	second, err := s.MigrateLifecycle()
	require.NoError(t, err)
	assert.False(t, second.Changed(), "second lifecycle migration must be a no-op")
	assert.False(t, second.SchemaMarkerMissing)
	assert.False(t, second.MarkerWritten)
	after, err := os.ReadFile(s.LifecycleSchemaMarkerPath())
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "marker must be preserved on idempotent apply")

	got, err := s.Get(testCtx(), "ddx-pseudo")
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
}
