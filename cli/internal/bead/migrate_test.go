package bead

import (
	"os"
	"path/filepath"
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
