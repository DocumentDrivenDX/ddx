package bead

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBeadDoctor_CleanFileReportsNothing covers ddx-b695e162 AC #1: a
// beads.jsonl whose fields all fit under MaxFieldBytes produces a clean
// report. The scan must be a no-op on healthy trees — false positives here
// would spam every operator's CI.
func TestBeadDoctor_CleanFileReportsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.jsonl")
	line := `{"id":"ddx-ok","title":"fine","type":"task","status":"open","priority":2,"description":"normal","labels":[],"deps":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(path, []byte(line+"\n"), 0o644))

	report, err := BeadDoctor(path)
	require.NoError(t, err)
	assert.True(t, report.Clean())
	assert.Empty(t, report.Findings)
}

// TestBeadDoctor_DetectsOversizedFields covers ddx-b695e162 AC #1: scanner
// reports every field on every bead that exceeds the cap. Synthesizes a
// description longer than MaxFieldBytes and an event body ditto so both
// code paths (top-level fields + nested event fields) are exercised.
func TestBeadDoctor_DetectsOversizedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.jsonl")
	huge := strings.Repeat("x", MaxFieldBytes+100)
	bead := map[string]any{
		"id":          "ddx-oversized",
		"title":       "oversized",
		"type":        "task",
		"status":      "open",
		"priority":    2,
		"description": huge,
		"labels":      []string{},
		"deps":        []string{},
		"events": []any{
			map[string]any{"kind": "review", "summary": "APPROVE", "body": huge},
		},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
	}
	encoded, err := json.Marshal(bead)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(encoded, '\n'), 0o644))

	report, err := BeadDoctor(path)
	require.NoError(t, err)
	require.Len(t, report.Findings, 2, "both description and events[0].body must be flagged — partial detection would hide the tracker-corruption surface")

	paths := []string{report.Findings[0].FieldPath, report.Findings[1].FieldPath}
	assert.Contains(t, paths, "description")
	assert.Contains(t, paths, "events[0].body")
	for _, f := range report.Findings {
		assert.Equal(t, "ddx-oversized", f.BeadID)
		assert.Greater(t, f.SizeBytes, MaxFieldBytes, "reported size must exceed the cap — the whole point of the finding")
	}
}

// TestBeadDoctorFix_RewriteIsIdempotent covers ddx-b695e162 AC #3: a tree
// that has been repaired stays clean on subsequent --fix runs. Non-idempotent
// repair would re-process the already-capped field and accumulate artifact
// sidecars on every run.
func TestBeadDoctorFix_RewriteIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	path := filepath.Join(ddxDir, "beads.jsonl")

	huge := strings.Repeat("A", MaxFieldBytes+10)
	bead := map[string]any{
		"id":          "ddx-fix-idempotent",
		"title":       "fix me",
		"type":        "task",
		"status":      "open",
		"priority":    2,
		"description": huge,
		"labels":      []string{},
		"deps":        []string{},
		"created_at":  "2026-01-01T00:00:00Z",
		"updated_at":  "2026-01-01T00:00:00Z",
	}
	encoded, err := json.Marshal(bead)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(encoded, '\n'), 0o644))

	fixedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	report1, err := BeadDoctorFix(path, func() time.Time { return fixedAt })
	require.NoError(t, err)
	require.False(t, report1.Clean(), "first fix must find the oversized field")

	report2, err := BeadDoctorFix(path, func() time.Time { return fixedAt.Add(time.Hour) })
	require.NoError(t, err)
	assert.True(t, report2.Clean(), "after a --fix pass the tree must be clean so repeated runs do not re-repair the same rows")
}

// TestBeadDoctorFix_WritesBackupAndArtifact covers ddx-b695e162 AC #2: a
// --fix run writes a backup before touching the source and persists full
// overflow content as an artifact sidecar. Operators MUST be able to recover
// the pre-fix state and audit the truncated payload.
func TestBeadDoctorFix_WritesBackupAndArtifact(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	path := filepath.Join(ddxDir, "beads.jsonl")

	huge := strings.Repeat("B", MaxFieldBytes+100)
	bead := map[string]any{
		"id":          "ddx-artifact-test",
		"title":       "with artifact",
		"type":        "task",
		"status":      "open",
		"priority":    2,
		"description": huge,
		"labels":      []string{},
		"deps":        []string{},
		"created_at":  "2026-01-01T00:00:00Z",
		"updated_at":  "2026-01-01T00:00:00Z",
	}
	encoded, err := json.Marshal(bead)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(encoded, '\n'), 0o644))

	fixedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	_, err = BeadDoctorFix(path, func() time.Time { return fixedAt })
	require.NoError(t, err)

	// Backup exists with the original huge content intact.
	backups, err := os.ReadDir(filepath.Join(ddxDir, "backups"))
	require.NoError(t, err)
	require.Len(t, backups, 1, "exactly one backup must be written per --fix invocation")
	backupContents, err := os.ReadFile(filepath.Join(ddxDir, "backups", backups[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(backupContents), huge[:200],
		"backup must preserve the pre-fix payload byte-for-byte — otherwise recovery is impossible")

	// Artifact sidecar holds the full original field.
	artifactDir := filepath.Join(ddxDir, "executions", "ddx-artifact-test", "repair-20260420T120000")
	entries, err := os.ReadDir(artifactDir)
	require.NoError(t, err, "repair dir must be created per bead per --fix invocation")
	require.Len(t, entries, 1, "one sidecar per oversized field")
	artifactContents, err := os.ReadFile(filepath.Join(artifactDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Equal(t, huge, string(artifactContents),
		"sidecar must preserve the original oversized field verbatim — this is the forensic trail that makes truncation safe")

	// Repaired source has a repair event recording what was done.
	repaired, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(repaired, &parsed))
	events, ok := parsed["events"].([]any)
	require.True(t, ok, "repair event must be appended to the rewritten bead")
	require.Len(t, events, 1)
	ev := events[0].(map[string]any)
	assert.Equal(t, "repair", ev["kind"])
	assert.Equal(t, "ddx bead doctor", ev["actor"])
	assert.Contains(t, ev["body"], "description",
		"repair event body must name the repaired field so operator audit can correlate")
}

// TestBeadDoctorFix_OversizedLineFromFixture covers ddx-b695e162 AC #4 using
// the byte-frozen fixture from ddx-004eccbc. A fixture captured from a real
// 2026-04-18 incident (axon's oversized beads.jsonl row) must be
// re-readable by the doctor and repairable — proving the whole pipeline
// works against a non-synthesized case.
func TestBeadDoctorFix_OversizedLineFromFixture(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	path := filepath.Join(ddxDir, "beads.jsonl")

	// Use the ddx-004eccbc fixture. The fixture is a single bead row; doctor
	// should either (a) flag one or more oversized fields and repair them, or
	// (b) decide the row is unparseable and flag line-level. Either outcome
	// is acceptable — the invariant is "doctor does not crash, and the file
	// is parseable afterward."
	fixtureCandidates := []string{
		"../agent/testdata/reviewer/bead-jsonl-oversized-line.jsonl",
		"../../internal/agent/testdata/reviewer/bead-jsonl-oversized-line.jsonl",
	}
	var fixtureBytes []byte
	for _, p := range fixtureCandidates {
		if b, err := os.ReadFile(p); err == nil {
			fixtureBytes = b
			break
		}
	}
	if len(fixtureBytes) == 0 {
		t.Skip("ddx-004eccbc fixture not present at expected path; skip")
	}
	require.NoError(t, os.WriteFile(path, fixtureBytes, 0o644))

	fixedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	_, err := BeadDoctorFix(path, func() time.Time { return fixedAt })
	require.NoError(t, err, "doctor must not crash on the real-incident fixture")

	// After repair the tree must be clean.
	report, err := BeadDoctor(path)
	require.NoError(t, err)
	assert.True(t, report.Clean(),
		"post-repair scan must find no remaining oversized fields — otherwise the repair didn't actually address the finding")
}
