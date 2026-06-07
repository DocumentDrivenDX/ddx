package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadReconcileCommandDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	b := &bead.Bead{
		ID:     "ddx-stale",
		Title:  "stale",
		Labels: []string{bead.LabelNoChangesUnjustified},
		Extra: map[string]any{
			bead.ExtraLastStatus: "no_changes",
			"events": []any{
				map[string]any{"kind": "execute-bead", "summary": "success", "created_at": "2026-01-01T00:00:00Z"},
			},
		},
	}
	require.NoError(t, store.Create(context.Background(), b))

	out, err := executeCommand(NewCommandFactory(dir).NewRootCommand(), "bead", "reconcile", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "would repair")
	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Extra, bead.ExtraLastStatus)
}

func TestBeadReconcileCommandApplyMutatesThroughStore(t *testing.T) {
	dir := t.TempDir()
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	b := &bead.Bead{
		ID:     "ddx-stale",
		Title:  "stale",
		Labels: []string{bead.LabelNoChangesUnjustified},
		Extra: map[string]any{
			bead.ExtraLastStatus: "no_changes",
			"events": []any{
				map[string]any{"kind": "execute-bead", "summary": "success", "created_at": "2026-01-01T00:00:00Z"},
			},
		},
	}
	require.NoError(t, store.Create(context.Background(), b))

	out, err := executeCommand(NewCommandFactory(dir).NewRootCommand(), "bead", "reconcile", "--apply")
	require.NoError(t, err)
	assert.Contains(t, out, "repaired")
	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Extra, bead.ExtraLastStatus)
	assert.NotContains(t, got.Labels, bead.LabelNoChangesUnjustified)
}
