package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeBlockedByRefConfig(t *testing.T, dir string) {
	t.Helper()

	ddxDir := filepath.Join(dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
known_repos:
  upstream:
    path: ../upstream
`), 0o644))
}

func TestBeadUpdate_BlockedByRef_Validation(t *testing.T) {
	workDir := t.TempDir()
	writeBlockedByRefConfig(t, workDir)

	factory := newBeadTestRoot(t, workDir)

	tests := []struct {
		name            string
		blockedByRef    string
		externalReason  string
		wantErrContains string
	}{
		{
			name:           "valid ref",
			blockedByRef:   "upstream#upstream-123",
			externalReason: "waiting on upstream",
		},
		{
			name:            "malformed ref missing hash",
			blockedByRef:    "upstream",
			externalReason:  "waiting on upstream",
			wantErrContains: "expected <repo>#<bead-id>",
		},
		{
			name:            "malformed ref empty repo",
			blockedByRef:    "#upstream-123",
			externalReason:  "waiting on upstream",
			wantErrContains: "repo is required",
		},
		{
			name:            "malformed ref empty bead id",
			blockedByRef:    "upstream#",
			externalReason:  "waiting on upstream",
			wantErrContains: "bead id is required",
		},
		{
			name:            "unknown repo",
			blockedByRef:    "missing#upstream-123",
			externalReason:  "waiting on upstream",
			wantErrContains: "unknown known-repo \"missing\"",
		},
		{
			name:            "missing external blocker reason",
			blockedByRef:    "upstream#upstream-123",
			wantErrContains: "transition_requires_external_blocker",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := factory.NewRootCommand()

			createOut, err := executeCommand(rootCmd, "bead", "create", "Blocked ref target", "--type", "task")
			require.NoError(t, err)
			id := strings.TrimSpace(createOut)
			require.NotEmpty(t, id)

			args := []string{"bead", "update", id, "--status", "blocked"}
			if tt.externalReason != "" {
				args = append(args, "--external-blocker-reason", tt.externalReason)
			}
			if tt.blockedByRef != "" {
				args = append(args, "--blocked-by-ref", tt.blockedByRef)
			}

			output, err := executeCommand(rootCmd, args...)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains, "output: "+output)
				return
			}

			require.NoError(t, err, "output: "+output)

			showOut, err := executeCommand(rootCmd, "bead", "show", id, "--json")
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(showOut), &got))
			assert.Equal(t, "blocked", got["status"])

			rawRef, ok := got[bead.ExtraLifecycleCrossRepoBlockerRef]
			require.True(t, ok, "show --json missing structured blocker ref: "+showOut)
			ref, ok := rawRef.(map[string]any)
			require.True(t, ok, "structured blocker ref should be a JSON object")
			assert.Equal(t, "upstream", ref["repo"])
			assert.Equal(t, "upstream-123", ref["bead"])
		})
	}
}

func TestBeadRecheckBlockersCommand(t *testing.T) {
	root := t.TempDir()
	blockedRoot := filepath.Join(root, "blocked")
	upstreamRoot := filepath.Join(root, "upstream")

	require.NoError(t, os.MkdirAll(blockedRoot, 0o755))
	require.NoError(t, os.MkdirAll(upstreamRoot, 0o755))
	writeBlockedByRefConfig(t, blockedRoot)

	factory := newBeadTestRoot(t, blockedRoot)
	blockedStore := bead.NewStore(filepath.Join(blockedRoot, ddxroot.DirName))
	require.NoError(t, blockedStore.Init(context.Background()))
	upstreamStore := bead.NewStore(filepath.Join(upstreamRoot, ddxroot.DirName))
	require.NoError(t, upstreamStore.Init(context.Background()))

	closedID := "upstream-closed"
	require.NoError(t, upstreamStore.Create(context.Background(), &bead.Bead{
		ID:    closedID,
		Title: "closed target",
	}))
	require.NoError(t, upstreamStore.Close(context.Background(), closedID))

	reopenedID := "blocked-recheck-one"
	reopenedRef, err := bead.NewCrossRepoBlockerRef("upstream", closedID)
	require.NoError(t, err)
	require.NoError(t, blockedStore.Create(context.Background(), &bead.Bead{
		ID:     reopenedID,
		Title:  "needs upstream",
		Status: bead.StatusBlocked,
		Extra: map[string]any{
			bead.ExtraLifecycleExternalBlockerReason: "waiting on upstream",
			bead.ExtraLifecycleCrossRepoBlockerRef:   reopenedRef,
		},
	}))

	out, err := executeCommand(factory.NewRootCommand(), "bead", "recheck-blockers", "--id", reopenedID, "--json")
	require.NoError(t, err)

	var firstRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &firstRows))
	require.Len(t, firstRows, 1)
	assert.Equal(t, reopenedID, firstRows[0]["bead_id"])
	assert.Equal(t, "open", firstRows[0]["status"])
	assert.Equal(t, "reopened", firstRows[0]["outcome"])

	showOut, err := executeCommand(factory.NewRootCommand(), "bead", "show", reopenedID, "--json")
	require.NoError(t, err)
	var reopened map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &reopened))
	assert.Equal(t, "open", reopened["status"])
	assert.NotContains(t, reopened, bead.ExtraLifecycleExternalBlockerReason)
	assert.NotContains(t, reopened, bead.ExtraLifecycleCrossRepoBlockerRef)

	openID := "upstream-open"
	require.NoError(t, upstreamStore.Create(context.Background(), &bead.Bead{
		ID:    openID,
		Title: "still open",
	}))

	blockedID := "blocked-recheck-two"
	blockedRef, err := bead.NewCrossRepoBlockerRef("upstream", openID)
	require.NoError(t, err)
	require.NoError(t, blockedStore.Create(context.Background(), &bead.Bead{
		ID:     blockedID,
		Title:  "still waiting",
		Status: bead.StatusBlocked,
		Extra: map[string]any{
			bead.ExtraLifecycleExternalBlockerReason: "waiting on upstream",
			bead.ExtraLifecycleCrossRepoBlockerRef:   blockedRef,
		},
	}))

	out, err = executeCommand(factory.NewRootCommand(), "bead", "recheck-blockers", "--json")
	require.NoError(t, err)

	var secondRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &secondRows))
	require.Len(t, secondRows, 1)
	assert.Equal(t, blockedID, secondRows[0]["bead_id"])
	assert.Equal(t, "blocked", secondRows[0]["status"])
	assert.Equal(t, "blocked", secondRows[0]["outcome"])

	showOut, err = executeCommand(factory.NewRootCommand(), "bead", "show", blockedID, "--json")
	require.NoError(t, err)
	var stillBlocked map[string]any
	require.NoError(t, json.Unmarshal([]byte(showOut), &stillBlocked))
	assert.Equal(t, "blocked", stillBlocked["status"])
	assert.Contains(t, stillBlocked, bead.ExtraLifecycleExternalBlockerReason)
	assert.Contains(t, stillBlocked, bead.ExtraLifecycleCrossRepoBlockerRef)
}
