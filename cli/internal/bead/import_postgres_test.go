package bead

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAxonImportCorpus(t *testing.T, sourceDir string, active []Bead, archive []Bead) {
	t.Helper()
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	if len(active) > 0 {
		writeBeadJSONLFile(t, filepath.Join(sourceDir, DefaultCollection+".jsonl"), active)
	}
	if len(archive) > 0 {
		writeBeadJSONLFile(t, filepath.Join(sourceDir, BeadsArchiveCollection+".jsonl"), archive)
	}
}

func writeBeadJSONLFile(t *testing.T, path string, beads []Bead) {
	t.Helper()
	var data []byte
	for i, bead := range beads {
		line, err := marshalBead(bead)
		require.NoError(t, err)
		data = append(data, line...)
		if i < len(beads)-1 {
			data = append(data, '\n')
		}
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func writeEventsSidecar(t *testing.T, sourceDir, beadID string, events []BeadEvent) {
	t.Helper()
	path := filepath.Join(sourceDir, "attachments", beadID, EventsAttachmentFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	var data []byte
	for i, event := range events {
		row := map[string]any{
			"kind":       event.Kind,
			"summary":    event.Summary,
			"body":       event.Body,
			"actor":      event.Actor,
			"created_at": event.CreatedAt.UTC().Format(time.RFC3339Nano),
			"source":     event.Source,
		}
		line, err := json.Marshal(row)
		require.NoError(t, err)
		data = append(data, line...)
		if i < len(events)-1 {
			data = append(data, '\n')
		}
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestImporter_DryRun_CountsOnly(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 12, 0, 0, 0, time.UTC)
	beads := []Bead{
		{
			ID:        "ddx-imp-1",
			Title:     "open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
			Extra: map[string]any{
				"spec-id": "FEAT-001",
			},
		},
		{
			ID:        "ddx-imp-2",
			Title:     "attached",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
			Extra: map[string]any{
				"events_attachment": eventsAttachmentRelPath("ddx-imp-2"),
				"custom-key":        "keep-me",
			},
		},
	}
	writeAxonImportCorpus(t, sourceDir, beads, nil)
	writeEventsSidecar(t, sourceDir, "ddx-imp-2", []BeadEvent{
		{Kind: "closed", Summary: "done", CreatedAt: now.Add(time.Minute)},
		{Kind: "review", Summary: "reviewed", CreatedAt: now.Add(2 * time.Minute)},
	})

	target := newAxonStore(t)
	snapshotBefore, err := target.ReadAll(testCtx())
	require.NoError(t, err)
	beforeCorpus, err := os.ReadFile(filepath.Join(sourceDir, DefaultCollection+".jsonl"))
	require.NoError(t, err)
	beforeAttachment, err := os.ReadFile(filepath.Join(sourceDir, "attachments", "ddx-imp-2", EventsAttachmentFileName))
	require.NoError(t, err)

	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		DryRun:          true,
		CopyAttachments: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, stats.BeadsMigrated)
	assert.Equal(t, 2, stats.EventsMigrated)
	assert.Equal(t, 1, stats.AttachmentsMigrated)

	afterCorpus, err := os.ReadFile(filepath.Join(sourceDir, DefaultCollection+".jsonl"))
	require.NoError(t, err)
	afterAttachment, err := os.ReadFile(filepath.Join(sourceDir, "attachments", "ddx-imp-2", EventsAttachmentFileName))
	require.NoError(t, err)
	assert.Equal(t, string(beforeCorpus), string(afterCorpus))
	assert.Equal(t, string(beforeAttachment), string(afterAttachment))

	snapshotAfter, err := target.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Equal(t, snapshotBefore, snapshotAfter)
}

func TestImporter_Source_LoadsActiveAndArchiveJSONL(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 12, 15, 0, 0, time.UTC)
	active := []Bead{
		{
			ID:        "ddx-src-1",
			Title:     "active only",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ddx-src-dup",
			Title:     "active version wins",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	archive := []Bead{
		{
			ID:        "ddx-src-dup",
			Title:     "archive version loses",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ddx-src-archived",
			Title:     "archive only",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	writeAxonImportCorpus(t, sourceDir, active, archive)

	loaded, err := loadImportCorpusForAxon(sourceDir)
	require.NoError(t, err)
	require.Len(t, loaded, 3)

	byID := make(map[string]Bead, len(loaded))
	for _, b := range loaded {
		byID[b.ID] = b
	}
	assert.Contains(t, byID, "ddx-src-1")
	assert.Contains(t, byID, "ddx-src-archived")
	require.Contains(t, byID, "ddx-src-dup")
	assert.Equal(t, "active version wins", byID["ddx-src-dup"].Title, "active copy must win over archive on duplicate ID")
}

func TestBeadMigrateAxon_Limit(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 12, 30, 0, 0, time.UTC)
	beads := []Bead{
		{
			ID:        "ddx-limit-1",
			Title:     "one",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "ddx-limit-2",
			Title:     "two",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
			Extra: map[string]any{
				"events": []map[string]any{
					{"kind": "summary", "summary": "done", "created_at": now.Add(time.Minute).Format(time.RFC3339Nano)},
				},
			},
		},
		{
			ID:        "ddx-limit-3",
			Title:     "three",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	writeAxonImportCorpus(t, sourceDir, beads, nil)

	target := newAxonStore(t)
	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		Limit:           2,
		CopyAttachments: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, stats.BeadsMigrated)
	assert.Equal(t, 1, stats.EventsMigrated)

	got, err := target.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []string{"ddx-limit-1", "ddx-limit-2"}, []string{got[0].ID, got[1].ID})
}

func TestImporter_PreservesExtras(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 13, 0, 0, 0, time.UTC)
	sourceBead := Bead{
		ID:        "ddx-extra-1",
		Title:     "extras",
		Status:    StatusOpen,
		Priority:  3,
		IssueType: DefaultType,
		CreatedAt: now,
		UpdatedAt: now,
		Extra: map[string]any{
			"spec-id":            "FEAT-042",
			"execution-eligible": true,
			"custom-key":         "custom-val",
		},
	}
	writeAxonImportCorpus(t, sourceDir, []Bead{sourceBead}, nil)

	target := newAxonStore(t)
	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		CopyAttachments: true,
		Verify:          true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.BeadsMigrated)

	got, err := target.Get(testCtx(), sourceBead.ID)
	require.NoError(t, err)
	assert.Equal(t, "FEAT-042", got.Extra["spec-id"])
	assert.Equal(t, true, got.Extra["execution-eligible"])
	assert.Equal(t, "custom-val", got.Extra["custom-key"])
	_, hasEvents := sourceBead.Extra["events"]
	assert.False(t, hasEvents, "import must not mutate the source bead extras map")
	assert.Equal(t, 3, len(sourceBead.Extra))
}

func TestImporter_MigratesAttachments(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 14, 0, 0, 0, time.UTC)
	sourceBead := Bead{
		ID:        "ddx-attach-1",
		Title:     "attachments",
		Status:    StatusClosed,
		Priority:  1,
		IssueType: DefaultType,
		CreatedAt: now,
		UpdatedAt: now,
		Extra: map[string]any{
			"events_attachment": eventsAttachmentRelPath("ddx-attach-1"),
		},
	}
	writeAxonImportCorpus(t, sourceDir, []Bead{sourceBead}, nil)
	events := []BeadEvent{
		{Kind: "closed", Summary: "first", CreatedAt: now.Add(time.Minute)},
		{Kind: "summary", Summary: "second", CreatedAt: now.Add(2 * time.Minute)},
	}
	writeEventsSidecar(t, sourceDir, sourceBead.ID, events)

	target := newAxonStore(t)
	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		CopyAttachments: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.AttachmentsMigrated)

	targetPath := filepath.Join(target.Dir, AxonDirName, "attachments", sourceBead.ID, EventsAttachmentFileName)
	got, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join(sourceDir, "attachments", sourceBead.ID, EventsAttachmentFileName))
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestBeadMigrateAxon_VerifyDetectsDrift(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 15, 0, 0, 0, time.UTC)
	sourceBeads := []Bead{
		{ID: "ddx-drift-1", Title: "one", Status: StatusOpen, Priority: 2, IssueType: DefaultType, CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-drift-2", Title: "two", Status: StatusOpen, Priority: 2, IssueType: DefaultType, CreatedAt: now, UpdatedAt: now},
		{ID: "ddx-drift-3", Title: "three", Status: StatusClosed, Priority: 2, IssueType: DefaultType, CreatedAt: now, UpdatedAt: now},
	}
	writeAxonImportCorpus(t, sourceDir, sourceBeads, nil)

	target := newAxonStore(t)
	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		CopyAttachments: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, stats.BeadsMigrated)

	transport := target.backend.(*AxonBackend).GraphQLTransport.(*fakeAxonGraphQLTransport)
	transport.mu.Lock()
	drift := transport.beads["ddx-drift-2"]
	drift.Title = "drifted"
	transport.beads["ddx-drift-2"] = drift
	transport.mu.Unlock()

	err = verifyImportedAxonCorpus(testCtx(), target, sourceDir, sourceBeads)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ddx-drift-2")
	assert.Contains(t, err.Error(), "drift")
}

func TestImporter_Verify_RoundTripPassesOnCleanImport(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), ddxroot.DirName)
	now := time.Date(2026, time.January, 7, 16, 0, 0, 0, time.UTC)
	sourceBeads := []Bead{
		{
			ID:        "ddx-round-1",
			Title:     "round one",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
			Extra: map[string]any{
				"custom-key": "custom-val",
			},
		},
		{
			ID:        "ddx-round-2",
			Title:     "round two",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: now,
			UpdatedAt: now,
			Extra: map[string]any{
				"events_attachment": eventsAttachmentRelPath("ddx-round-2"),
			},
		},
	}
	writeAxonImportCorpus(t, sourceDir, sourceBeads, nil)
	writeEventsSidecar(t, sourceDir, "ddx-round-2", []BeadEvent{
		{Kind: "closed", Summary: "done", CreatedAt: now.Add(time.Minute)},
	})

	target := newAxonStore(t)
	stats, err := importJSONLCorpusToAxon(testCtx(), target, sourceDir, MigrateAxonOptions{
		CopyAttachments: true,
		Verify:          true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, stats.BeadsMigrated)
	assert.Equal(t, 1, stats.AttachmentsMigrated)

	got, err := target.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "custom-val", got[0].Extra["custom-key"])
}
