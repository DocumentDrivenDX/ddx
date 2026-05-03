package bead

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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

	stats, err := s.Migrate()
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
	active, err := s.ReadAll()
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

	first, err := s.Migrate()
	require.NoError(t, err)
	assert.True(t, first.Changed())

	beforeActive, err := os.ReadFile(s.File)
	require.NoError(t, err)
	beforeArchive, err := os.ReadFile(filepath.Join(s.Dir, BeadsArchiveCollection+".jsonl"))
	require.NoError(t, err)

	second, err := s.Migrate()
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

	_, err = s.Migrate()
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
		got, err := s.GetWithArchive(want)
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

	stats, err := s.MigrateDryRun()
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

	stats, err := s.Migrate()
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
	second, err := s.Migrate()
	require.NoError(t, err)
	assert.False(t, second.Changed())
}

// TestMigrateToAxonRoundTripFromFixture loads the cli/internal/bead/testdata
// fixture pair (.ddx/beads.jsonl + .ddx/beads-archive.jsonl), migrates to
// the axon backend, and verifies that an export from the post-migration
// axon-backed store matches the pre-migration JSONL export bead-for-bead
// (sorted to ignore active-then-archive ordering quirks in ExportTo).
func TestMigrateToAxonRoundTripFromFixture(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ddx")
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
	require.NoError(t, jsonlStore.ExportTo(&preBuf))
	require.NotZero(t, preBuf.Len(), "fixture must produce a non-empty pre-migration export")

	stats, err := jsonlStore.MigrateToAxon()
	require.NoError(t, err)
	// 3 active + 2 archive (no overlap) = 5 distinct beads.
	assert.Equal(t, 5, stats.BeadsMigrated)
	// 2 inline events on ddx-mta-active2 + 1 on ddx-mta-active3 + 1 on
	// ddx-mta-arch1 = 4 inline events written into ddx_bead_events.
	assert.Equal(t, 4, stats.EventsMigrated)

	// Source files left intact — operator removes them after verification.
	for _, src := range []string{"beads.jsonl", "beads-archive.jsonl"} {
		_, statErr := os.Stat(filepath.Join(dir, src))
		assert.NoError(t, statErr, "source file %s must not be deleted by migration", src)
	}
	// Axon collection files exist.
	for _, dst := range []string{"axon/ddx_beads.jsonl", "axon/ddx_bead_events.jsonl"} {
		_, statErr := os.Stat(filepath.Join(dir, dst))
		assert.NoError(t, statErr, "axon file %s must exist after migration", dst)
	}

	// Axon-backed export must match the JSONL export.
	axStore := NewStore(dir)
	axStore.backend = NewAxonBackend(dir, axStore.LockWait)
	var postBuf bytes.Buffer
	require.NoError(t, axStore.ExportTo(&postBuf))

	pre := sortedNonEmptyLines(preBuf.String())
	post := sortedNonEmptyLines(postBuf.String())
	assert.Equal(t, pre, post, "post-migration export must round-trip the pre-migration export")

	// Idempotent: a second migration produces byte-identical axon files.
	axBeadsBefore, err := os.ReadFile(filepath.Join(dir, "axon", "ddx_beads.jsonl"))
	require.NoError(t, err)
	axEventsBefore, err := os.ReadFile(filepath.Join(dir, "axon", "ddx_bead_events.jsonl"))
	require.NoError(t, err)

	stats2, err := jsonlStore.MigrateToAxon()
	require.NoError(t, err)
	assert.Equal(t, stats, stats2, "second migration reports the same counts")

	axBeadsAfter, err := os.ReadFile(filepath.Join(dir, "axon", "ddx_beads.jsonl"))
	require.NoError(t, err)
	axEventsAfter, err := os.ReadFile(filepath.Join(dir, "axon", "ddx_bead_events.jsonl"))
	require.NoError(t, err)
	assert.Equal(t, string(axBeadsBefore), string(axBeadsAfter), "second migration must not duplicate bead rows")
	assert.Equal(t, string(axEventsBefore), string(axEventsAfter), "second migration must not duplicate event rows")
}

// TestMigrateToAxonNoSources is a defensive sanity test: an empty .ddx with
// no source files must not error and produces an empty axon snapshot.
func TestMigrateToAxonNoSources(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ddx")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	s := NewStore(dir)
	stats, err := s.MigrateToAxon()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.BeadsMigrated)
	assert.Equal(t, 0, stats.EventsMigrated)
}

func sortedNonEmptyLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l)
	}
	sort.Strings(out)
	return out
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

	stats, err := s.Migrate()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Archived, "closed dep referenced by open bead must not archive")

	// Ready() must still surface ddx-open (its closed dep is satisfied).
	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, "ddx-open", ready[0].ID)
}
