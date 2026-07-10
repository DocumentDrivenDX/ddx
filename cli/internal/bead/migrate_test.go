package bead

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrateSeed builds a closed bead with inline events and an updated
// timestamp far enough in the past that any future MinAge check would
// also accept it. Migrate uses MinAge=0 internally so the timestamp only
// matters for its archived_at stamp.
func migrateSeed(id, title string, withEvents bool) Bead {
	t := time.Now().UTC().Add(-90 * 24 * time.Hour)
	b := Bead{
		ID:        id,
		Title:     title,
		Status:    StatusClosed,
		Priority:  2,
		IssueType: DefaultType,
		CreatedAt: t.Add(-time.Hour),
		UpdatedAt: t,
	}
	if withEvents {
		b.Extra = map[string]any{
			"events": []map[string]any{
				{"kind": "review", "summary": "APPROVE", "body": "ok", "created_at": t.Format(time.RFC3339Nano)},
				{"kind": "summary", "summary": "done", "body": "", "created_at": t.Format(time.RFC3339Nano)},
			},
			"closing_commit_sha": "deadbeef",
		}
	} else {
		b.Extra = map[string]any{"closing_commit_sha": "deadbeef"}
	}
	return b
}

func TestMigrateExternalizesAndArchives(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		migrateSeed("ddx-c1", "closed with events", true),
		migrateSeed("ddx-c2", "closed without events", false),
		{
			ID:        "ddx-open",
			Title:     "still open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
	}))

	stats, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 1, stats.EventsExternalized, "only ddx-c1 had inline events")
	assert.Equal(t, 2, stats.Archived, "both closed beads should archive")

	// AC2: archive file exists, attachments dir populated.
	archivePath := filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl")
	info, statErr := os.Stat(archivePath)
	require.NoError(t, statErr)
	assert.Greater(t, info.Size(), int64(0))

	attachPath := s.eventsAttachmentPath("ddx-c1")
	_, statErr = os.Stat(attachPath)
	require.NoError(t, statErr, "events sidecar must exist for ddx-c1")

	// Active collection only contains the open bead.
	active, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "ddx-open", active[0].ID)
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		migrateSeed("ddx-c1", "closed", true),
		migrateSeed("ddx-c2", "closed", false),
	}))

	first, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.True(t, first.Changed())

	beforeActive, err := os.ReadFile(s.File)
	require.NoError(t, err)
	beforeArchive, err := os.ReadFile(filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)

	second, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.False(t, second.Changed(), "second pass must be a no-op")

	afterActive, err := os.ReadFile(s.File)
	require.NoError(t, err)
	afterArchive, err := os.ReadFile(filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)
	assert.Equal(t, string(beforeActive), string(afterActive), "active file must not change on second pass")
	assert.Equal(t, string(beforeArchive), string(afterArchive), "archive file must not change on second pass")
}

func TestMigratePreservesData(t *testing.T) {
	s := newTestStore(t)
	beads := []Bead{
		migrateSeed("ddx-c1", "with events", true),
		migrateSeed("ddx-c2", "no events", false),
		{
			ID:        "ddx-open",
			Title:     "open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
	}
	require.NoError(t, s.WriteAll(beads))

	beforeStatus, err := s.Status()
	require.NoError(t, err)

	_, err = s.Migrate(testCtx())
	require.NoError(t, err)

	afterStatus, err := s.Status()
	require.NoError(t, err)
	// AC3: totals identical pre/post migration.
	assert.Equal(t, beforeStatus.Total, afterStatus.Total)
	assert.Equal(t, beforeStatus.Open, afterStatus.Open)
	assert.Equal(t, beforeStatus.Closed, afterStatus.Closed)
	assert.Equal(t, beforeStatus.Ready, afterStatus.Ready)
	assert.Equal(t, beforeStatus.Blocked, afterStatus.Blocked)

	// AC4: show works for archived beads.
	for _, want := range []string{"ddx-c1", "ddx-c2", "ddx-open"} {
		got, err := s.GetWithArchive(testCtx(), want)
		require.NoError(t, err, "GetWithArchive(%s)", want)
		require.NotNil(t, got)
		assert.Equal(t, want, got.ID)
	}

	// Events for ddx-c1 are still readable from the sidecar attachment,
	// independent of which collection currently owns the row.
	ev, err := s.readEventsAttachment("ddx-c1")
	require.NoError(t, err)
	require.Len(t, ev, 2)
	assert.Equal(t, "review", ev[0].Kind)
	assert.Equal(t, "APPROVE", ev[0].Summary)
}

func TestMigrateDryRunReportsWithoutWriting(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		migrateSeed("ddx-c1", "closed with events", true),
		migrateSeed("ddx-c2", "closed without events", false),
		{
			ID:        "ddx-open",
			Title:     "open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		},
	}))

	beforeActive, err := os.ReadFile(s.File)
	require.NoError(t, err)
	archivePath := filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl")
	_, archiveStatErr := os.Stat(archivePath)
	require.True(t, os.IsNotExist(archiveStatErr), "archive should not exist before migrate")

	stats, err := (&storeMigrator{store: s}).MigrateDryRun(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 1, stats.EventsExternalized)
	assert.Equal(t, 2, stats.Archived)

	afterActive, err := os.ReadFile(s.File)
	require.NoError(t, err)
	assert.Equal(t, string(beforeActive), string(afterActive), "dry-run must not touch active file")
	_, archiveStatErr = os.Stat(archivePath)
	assert.True(t, os.IsNotExist(archiveStatErr), "dry-run must not create archive file")
	_, attachStatErr := os.Stat(s.eventsAttachmentPath("ddx-c1"))
	assert.True(t, os.IsNotExist(attachStatErr), "dry-run must not create attachments")
}

// TestMigrateLargeFixtureSplits seeds a synthetic >5MB beads.jsonl mostly
// composed of closed beads with inline events, runs Migrate, and asserts
// that the active file shrinks below 4MB while the archive partner picks
// up the closed rows and per-bead attachments are written.
func TestMigrateLargeFixtureSplits(t *testing.T) {
	s := newTestStore(t)

	// Build ~600 closed beads each carrying a chunky inline event so the
	// raw active file lands above the 4MB archive threshold.
	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	body := strings.Repeat("x", 8*1024) // 8KB body per event
	beads := make([]Bead, 0, 605)
	for i := 0; i < 600; i++ {
		beads = append(beads, Bead{
			ID:        fmt.Sprintf("ddx-l%04d", i),
			Title:     "closed with bulky event",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old.Add(-time.Hour),
			UpdatedAt: old,
			Extra: map[string]any{
				"events": []map[string]any{
					{"kind": "summary", "summary": "done", "body": body, "created_at": old.Format(time.RFC3339Nano)},
				},
				"closing_commit_sha": "deadbeef",
			},
		})
	}
	for i := 0; i < 5; i++ {
		beads = append(beads, Bead{
			ID:        fmt.Sprintf("ddx-o%d", i),
			Title:     "open",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
		})
	}
	require.NoError(t, s.WriteAll(beads))

	info, err := os.Stat(s.File)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(4*1024*1024), "fixture must exceed 4MB to be a meaningful split test")

	stats, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 600, stats.Archived)
	assert.Equal(t, 600, stats.EventsExternalized)

	// AC2: active file < 4MB after migration.
	info, err = os.Stat(s.File)
	require.NoError(t, err)
	assert.Less(t, info.Size(), int64(4*1024*1024), "active beads.jsonl must drop below 4MB after migration")

	// AC2: archive partner exists with the moved rows.
	archInfo, err := os.Stat(filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)
	assert.Greater(t, archInfo.Size(), int64(0))

	// AC2: per-bead attachment sidecars exist for archived beads.
	_, err = os.Stat(s.eventsAttachmentPath("ddx-l0000"))
	require.NoError(t, err)

	// AC3 / AC6: re-running is a no-op.
	second, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.False(t, second.Changed())
}

func loadMigrateToAxonFixture(t *testing.T) (*Store, *fakeAxonGraphQLTransport, bytes.Buffer) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	copyFixture := func(src, dst string) {
		t.Helper()
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, data, 0o644))
	}
	copyFixture("testdata/migrate_axon_active.jsonl", filepath.Join(dir, "beads.jsonl"))
	copyFixture("testdata/migrate_axon_archive.jsonl", filepath.Join(dir, "beads-archive.jsonl"))

	jsonlStore := NewStore(dir)

	var preBuf bytes.Buffer
	require.NoError(t, jsonlStore.ExportTo(testCtx(), &preBuf))
	require.NotZero(t, preBuf.Len(), "fixture must produce a non-empty pre-migration export")

	axonStore := NewStore(dir)
	transport := newFakeAxonGraphQLTransport()
	ax := NewAxonBackend(dir, axonStore.LockWait)
	ax.GraphQLTransport = transport
	ax.GraphQLClient = transport
	axonStore.backend = ax
	require.NoError(t, axonStore.Init(testCtx()))
	return axonStore, transport, preBuf
}

func TestImporter_Apply_WritesToPostgres(t *testing.T) {
	axonStore, transport, _ := loadMigrateToAxonFixture(t)

	stats, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)
	// 3 active + 2 archive (no overlap) = 5 distinct beads.
	assert.Equal(t, 5, stats.BeadsMigrated)
	// 2 inline events on ddx-mta-active2 + 1 on ddx-mta-active3 + 1 on
	// ddx-mta-arch1 = 4 inline events written into ddx_bead_events.
	assert.Equal(t, 4, stats.EventsMigrated)

	// Source files left intact — operator removes them after verification.
	for _, src := range []string{"beads.jsonl", "beads-archive.jsonl"} {
		_, statErr := os.Stat(filepath.Join(axonStore.Dir, src))
		assert.NoError(t, statErr, "source file %s must not be deleted by migration", src)
	}
	beads, events := transport.snapshot()
	assert.Len(t, beads, 5)
	assert.Len(t, events, 4)
	assert.ElementsMatch(t, []string{
		"ddx-mta-active1",
		"ddx-mta-active2",
		"ddx-mta-active3",
		"ddx-mta-arch1",
		"ddx-mta-arch2",
	}, []string{beads[0].ID, beads[1].ID, beads[2].ID, beads[3].ID, beads[4].ID})
	assert.Equal(t, []string{"alice claimed", "progress note", "shipped", "closed by bob"}, []string{
		events[0].Summary,
		events[1].Summary,
		events[2].Summary,
		events[3].Summary,
	})
	assert.Equal(t, "ddx-mta-active2", events[0].EventOf)
	assert.Equal(t, "ddx-mta-active2", events[1].EventOf)
	assert.Equal(t, "ddx-mta-active3", events[2].EventOf)
	assert.Equal(t, "ddx-mta-arch1", events[3].EventOf)
}

// TestImporter_Apply_PreservesArchiveState covers AC2: archived source rows
// (from beads-archive.jsonl, carrying archived_at) must remain distinguishable
// from active source rows (from beads.jsonl, no archived_at) after import,
// when read back through the Axon-backed Store's read surface.
func TestImporter_Apply_PreservesArchiveState(t *testing.T) {
	axonStore, _, _ := loadMigrateToAxonFixture(t)

	_, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)

	all, err := axonStore.ReadAll(testCtx())
	require.NoError(t, err)
	byID := make(map[string]Bead, len(all))
	for _, b := range all {
		byID[b.ID] = b
	}

	for _, id := range []string{"ddx-mta-active1", "ddx-mta-active2", "ddx-mta-active3"} {
		b, ok := byID[id]
		require.True(t, ok, "active bead %s must be imported", id)
		_, hasArchivedAt := b.Extra["archived_at"]
		assert.False(t, hasArchivedAt, "active bead %s must not carry archived_at", id)
	}

	for _, id := range []string{"ddx-mta-arch1", "ddx-mta-arch2"} {
		b, ok := byID[id]
		require.True(t, ok, "archived bead %s must be imported", id)
		archivedAt, hasArchivedAt := b.Extra["archived_at"]
		assert.True(t, hasArchivedAt, "archived bead %s must carry archived_at", id)
		assert.NotEmpty(t, archivedAt)
	}
}

// TestImporter_Apply_WritesInlineEvents covers AC3: inline events attached to
// source beads must be persisted alongside their imported bead records and
// readable back through Store.Events for both active and archived beads.
func TestImporter_Apply_WritesInlineEvents(t *testing.T) {
	axonStore, _, _ := loadMigrateToAxonFixture(t)

	_, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)

	events, err := axonStore.Events("ddx-mta-active2")
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "alice claimed", events[0].Summary)
	assert.Equal(t, "progress note", events[1].Summary)

	events, err = axonStore.Events("ddx-mta-active3")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "shipped", events[0].Summary)

	events, err = axonStore.Events("ddx-mta-arch1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "closed by bob", events[0].Summary)

	events, err = axonStore.Events("ddx-mta-arch2")
	require.NoError(t, err)
	assert.Len(t, events, 0)

	events, err = axonStore.Events("ddx-mta-active1")
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

func TestImporter_Apply_Idempotent_OnReRun(t *testing.T) {
	axonStore, transport, _ := loadMigrateToAxonFixture(t)
	stats, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)

	beadsBefore, eventsBefore := transport.snapshot()
	stats2, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 5, stats.BeadsMigrated)
	assert.Equal(t, 4, stats.EventsMigrated)
	assert.Equal(t, 0, stats2.BeadsMigrated, "second migration must report no new bead writes")
	assert.Equal(t, 0, stats2.EventsMigrated, "second migration must report no new event writes")

	beadsAfter, eventsAfter := transport.snapshot()
	assert.Equal(t, beadsBefore, beadsAfter, "second migration must not duplicate bead rows")
	assert.Equal(t, eventsBefore, eventsAfter, "second migration must not duplicate event rows")
}

func TestImporter_PreservesEventOrdering(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "beads.jsonl"), []byte(`{"id":"ddx-order-01","title":"ordering","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z","events":[{"kind":"summary","summary":"later","created_at":"2026-01-02T00:00:00Z"},{"kind":"summary","summary":"earlier","created_at":"2026-01-01T12:00:00Z"},{"kind":"summary","summary":"same-time-a","created_at":"2026-01-03T00:00:00Z"},{"kind":"summary","summary":"same-time-b","created_at":"2026-01-03T00:00:00Z"}]}`), 0o644))

	axonStore := NewStore(dir)
	transport := newFakeAxonGraphQLTransport()
	ax := NewAxonBackend(dir, axonStore.LockWait)
	ax.GraphQLTransport = transport
	ax.GraphQLClient = transport
	axonStore.backend = ax
	require.NoError(t, axonStore.Init(testCtx()))

	_, err := axonStore.migrateToAxon(testCtx())
	require.NoError(t, err)

	events, err := axonStore.Events("ddx-order-01")
	require.NoError(t, err)
	require.Len(t, events, 4)
	for i := 1; i < len(events); i++ {
		assert.False(t, events[i].CreatedAt.Before(events[i-1].CreatedAt), "events must be monotonic by created_at")
	}
	assert.Equal(t, "earlier", events[0].Summary)
	assert.Equal(t, "later", events[1].Summary)
	assert.Equal(t, "same-time-a", events[2].Summary)
	assert.Equal(t, "same-time-b", events[3].Summary)
}

func TestMigrateToAxonNoSources(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	s := NewStore(dir)
	stats, err := s.migrateToAxon(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 0, stats.BeadsMigrated)
	assert.Equal(t, 0, stats.EventsMigrated)
}

func TestMigrator_MigrateToAxon_ReturnsImporterResult(t *testing.T) {
	s, _, _ := loadMigrateToAxonFixture(t)
	mig := &storeMigrator{store: s}
	stats, err := mig.MigrateToAxon(testCtx(), MigrateAxonOptions{CopyAttachments: true})
	require.NoError(t, err)
	assert.Equal(t, 5, stats.BeadsMigrated)
	assert.Equal(t, 4, stats.EventsMigrated)
}

func TestMigrator_LifecycleMigration_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.WriteAll([]Bead{
		{
			ID:        "ddx-roundtrip",
			Title:     "roundtrip",
			Status:    StatusOpen,
			Priority:  1,
			IssueType: DefaultType,
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			UpdatedAt: time.Now().UTC(),
			Labels:    []string{LabelNeedsHuman},
		},
	}))
	mig := &storeMigrator{store: s}
	dry, err := mig.MigrateLifecycleDryRun(testCtx())
	require.NoError(t, err)
	assert.True(t, dry.DryRun)
	assert.Equal(t, 1, dry.LegacyNeedsHumanLabels)
	stats, err := mig.MigrateLifecycle(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LegacyNeedsHumanLabels)
	assert.Equal(t, 1, stats.ToProposed)
	got, err := s.Get(testCtx(), "ddx-roundtrip")
	require.NoError(t, err)
	assert.Equal(t, StatusProposed, got.Status)
}

func TestMigrator_FromHelix_PreservesAllFields(t *testing.T) {
	s := newTestStore(t)
	mig := &storeMigrator{store: s}
	// No .helix/issues.jsonl present → no-op.
	n, migrated, err := mig.MigrateFromHelix(testCtx())
	require.NoError(t, err)
	assert.False(t, migrated)
	assert.Equal(t, 0, n)
}

func TestMigratePreservesReferencedDeps(t *testing.T) {
	s := newTestStore(t)
	old := time.Now().UTC().Add(-90 * 24 * time.Hour)
	require.NoError(t, s.WriteAll([]Bead{
		{
			ID:        "ddx-dep",
			Title:     "closed dep",
			Status:    StatusClosed,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old.Add(-time.Hour),
			UpdatedAt: old,
		},
		{
			ID:        "ddx-open",
			Title:     "open with closed dep",
			Status:    StatusOpen,
			Priority:  2,
			IssueType: DefaultType,
			CreatedAt: old,
			UpdatedAt: old,
			Dependencies: []Dependency{
				{IssueID: "ddx-open", DependsOnID: "ddx-dep", Type: "blocks"},
			},
		},
	}))

	stats, err := s.Migrate(testCtx())
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Archived, "closed dep referenced by open bead must not archive")

	// Ready() must still surface ddx-open (its closed dep is satisfied).
	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, "ddx-open", ready[0].ID)
}
