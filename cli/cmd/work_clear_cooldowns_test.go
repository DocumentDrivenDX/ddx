package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCooldownEnv(t *testing.T, beads ...*bead.Bead) (*TestEnvironment, *bead.Store) {
	t.Helper()
	env := NewTestEnvironment(t)
	store := bead.NewStore(filepath.Join(env.Dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return env, store
}

func setupConventionCooldownProject(t *testing.T, beads ...*bead.Bead) (string, *bead.Store) {
	t.Helper()
	projectRoot := minimalProjectDir(t)
	store := bead.NewStore(ddxroot.Path(context.Background(), projectRoot))
	require.NoError(t, store.Init(context.Background()))
	for _, b := range beads {
		require.NoError(t, store.Create(context.Background(), b))
	}
	return projectRoot, store
}

func TestClearCooldowns_All(t *testing.T) {
	b1 := &bead.Bead{ID: "ddx-cc-1", Title: "Bead one"}
	b2 := &bead.Bead{ID: "ddx-cc-2", Title: "Bead two"}
	b3 := &bead.Bead{ID: "ddx-cc-3", Title: "Bead three (no cooldown)"}

	env, store := setupCooldownEnv(t, b1, b2, b3)

	until := time.Now().Add(24 * time.Hour)
	require.NoError(t, store.SetExecutionCooldown(b1.ID, until, "push_failed", "push rejected", ""))
	require.NoError(t, store.SetExecutionCooldown(b2.ID, until, "no_changes_unjustified", "no changes", ""))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "work", "clear-cooldowns", "--all")
	require.NoError(t, err)
	assert.Contains(t, out, "cleared 2 cooldown(s)")

	got1, err := store.Get(b1.ID)
	require.NoError(t, err)
	assert.Empty(t, got1.Extra[bead.ExtraRetryAfter], "retry-after should be cleared on b1")
	assert.Empty(t, got1.Extra[bead.ExtraLastStatus], "last-status should be cleared on b1")
	assert.Empty(t, got1.Extra[bead.ExtraLastDetail], "last-detail should be cleared on b1")

	got2, err := store.Get(b2.ID)
	require.NoError(t, err)
	assert.Empty(t, got2.Extra[bead.ExtraRetryAfter], "retry-after should be cleared on b2")

	got3, err := store.Get(b3.ID)
	require.NoError(t, err)
	assert.Empty(t, got3.Extra[bead.ExtraRetryAfter], "b3 had no cooldown; still empty")
}

func TestClearCooldowns_ByStatus(t *testing.T) {
	b1 := &bead.Bead{ID: "ddx-cs-1", Title: "Push failed bead"}
	b2 := &bead.Bead{ID: "ddx-cs-2", Title: "No changes bead"}
	b3 := &bead.Bead{ID: "ddx-cs-3", Title: "Another push failed bead"}

	env, store := setupCooldownEnv(t, b1, b2, b3)

	until := time.Now().Add(24 * time.Hour)
	require.NoError(t, store.SetExecutionCooldown(b1.ID, until, "push_failed", "detail a", ""))
	require.NoError(t, store.SetExecutionCooldown(b2.ID, until, "no_changes_unjustified", "detail b", ""))
	require.NoError(t, store.SetExecutionCooldown(b3.ID, until, "push_failed", "detail c", ""))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "work", "clear-cooldowns", "--status", "push_failed")
	require.NoError(t, err)
	assert.Contains(t, out, "cleared 2 cooldown(s)")

	got1, err := store.Get(b1.ID)
	require.NoError(t, err)
	assert.Empty(t, got1.Extra[bead.ExtraRetryAfter], "b1 push_failed cooldown should be cleared")

	got2, err := store.Get(b2.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, got2.Extra[bead.ExtraRetryAfter], "b2 no_changes cooldown should remain")

	got3, err := store.Get(b3.ID)
	require.NoError(t, err)
	assert.Empty(t, got3.Extra[bead.ExtraRetryAfter], "b3 push_failed cooldown should be cleared")
}

func TestClearCooldowns_DryRun(t *testing.T) {
	b1 := &bead.Bead{ID: "ddx-dr-1", Title: "Dry run bead one"}
	b2 := &bead.Bead{ID: "ddx-dr-2", Title: "Dry run bead two"}

	env, store := setupCooldownEnv(t, b1, b2)

	until := time.Now().Add(24 * time.Hour)
	require.NoError(t, store.SetExecutionCooldown(b1.ID, until, "push_failed", "detail", ""))
	require.NoError(t, store.SetExecutionCooldown(b2.ID, until, "push_failed", "detail", ""))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "work", "clear-cooldowns", "--all", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "would clear 2 cooldown(s)")

	// Verify no mutation occurred
	got1, err := store.Get(b1.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, got1.Extra[bead.ExtraRetryAfter], "b1 cooldown must not be cleared in dry-run")

	got2, err := store.Get(b2.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, got2.Extra[bead.ExtraRetryAfter], "b2 cooldown must not be cleared in dry-run")

	// Verify the output says "would clear" not "cleared"
	assert.False(t, strings.Contains(out, "cleared "), "dry-run must not say 'cleared'")
}

func TestWorkClearCooldownsUsesDDxRootPath(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	beadWithCooldown := &bead.Bead{ID: "ddx-convention-cooldown", Title: "Convention cooldown"}
	projectRoot, store := setupConventionCooldownProject(t, beadWithCooldown)

	_, statErr := os.Stat(filepath.Join(projectRoot, ddxroot.DirName))
	require.True(t, os.IsNotExist(statErr), "project root must stay in convention mode for this test")

	until := time.Now().Add(24 * time.Hour)
	require.NoError(t, store.SetExecutionCooldown(beadWithCooldown.ID, until, "push_failed", "detail", ""))

	out, err := executeCommand(NewCommandFactory(projectRoot).NewRootCommand(), "work", "clear-cooldowns", "--all")
	require.NoError(t, err)
	assert.Contains(t, out, "cleared 1 cooldown(s)")

	got, err := store.Get(beadWithCooldown.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Extra[bead.ExtraRetryAfter])
}
