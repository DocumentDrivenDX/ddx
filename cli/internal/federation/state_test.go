package federation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleSpoke(id, name string, hb time.Time) SpokeRecord {
	return SpokeRecord{
		NodeID:        id,
		Name:          name,
		URL:           "https://" + name + ":7743",
		DDxVersion:    "0.42.0",
		SchemaVersion: "1",
		Capabilities:  []string{"beads", "runs"},
		RegisteredAt:  time.Date(2026, 5, 2, 19, 58, 33, 0, time.UTC),
		LastHeartbeat: hb,
		Status:        StatusActive,
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "federation-state.json")

	in := NewRegistry()
	hb := time.Date(2026, 5, 2, 20, 0, 1, 0, time.UTC)
	if err := in.UpsertSpoke(sampleSpoke("node-1", "bragi", hb)); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := in.UpsertSpoke(sampleSpoke("node-2", "eitri", hb)); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := SaveStateTo(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	// File contents must be valid JSON and carry schema_version.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("file is not valid json: %v", err)
	}
	if generic["schema_version"] != CurrentSchemaVersion {
		t.Fatalf("missing/wrong schema_version field in file: %v", generic["schema_version"])
	}

	// File mode must be 0600 per FEAT-026 NFR.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}

	out, err := LoadStateFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema version round-trip: got %q", out.SchemaVersion)
	}
	if len(out.Spokes) != 2 {
		t.Fatalf("spoke count: got %d", len(out.Spokes))
	}
	if got := out.FindSpoke("node-2"); got == nil || got.Name != "eitri" {
		t.Fatalf("spoke lookup failed: %+v", got)
	}
}

func TestLoadMissingFileReturnsFreshRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	r, err := LoadStateFrom(path)
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if r.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("fresh registry schema_version: got %q", r.SchemaVersion)
	}
	if len(r.Spokes) != 0 {
		t.Fatalf("fresh registry should be empty, got %d", len(r.Spokes))
	}
}

func TestLoadMalformedFileRecovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "federation-state.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	r, err := LoadStateFrom(path)
	if err != nil {
		t.Fatalf("load malformed: %v", err)
	}
	if len(r.Spokes) != 0 || r.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("expected fresh registry, got %+v", r)
	}

	// Corrupt file should have been quarantined, not the live path.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var foundQuarantine bool
	for _, e := range entries {
		if strings.Contains(e.Name(), ".corrupt-") {
			foundQuarantine = true
		}
	}
	if !foundQuarantine {
		t.Fatalf("expected quarantined .corrupt-* file, got %v", entries)
	}

	// And a subsequent save+load works cleanly.
	r2 := NewRegistry()
	_ = r2.UpsertSpoke(sampleSpoke("node-x", "x", time.Now().UTC()))
	if err := SaveStateTo(path, r2); err != nil {
		t.Fatalf("save after recovery: %v", err)
	}
	r3, err := LoadStateFrom(path)
	if err != nil || len(r3.Spokes) != 1 {
		t.Fatalf("post-recovery load: %v %+v", err, r3)
	}
}

func TestStatusTransitions(t *testing.T) {
	r := NewRegistry()
	now := time.Date(2026, 5, 2, 20, 0, 0, 0, time.UTC)

	// Fresh register, no heartbeat yet.
	rec := sampleSpoke("node-1", "bragi", time.Time{})
	rec.Status = StatusRegistered
	if err := r.UpsertSpoke(rec); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	r.ReconcileLiveness(now, 2*time.Minute)
	if got := r.FindSpoke("node-1").Status; got != StatusRegistered {
		t.Fatalf("zero heartbeat → registered, got %q", got)
	}

	// Recent heartbeat → active.
	r.FindSpoke("node-1").LastHeartbeat = now.Add(-30 * time.Second)
	r.ReconcileLiveness(now, 2*time.Minute)
	if got := r.FindSpoke("node-1").Status; got != StatusActive {
		t.Fatalf("recent hb → active, got %q", got)
	}

	// Heartbeat older than staleAfter → stale.
	r.FindSpoke("node-1").LastHeartbeat = now.Add(-3 * time.Minute)
	r.ReconcileLiveness(now, 2*time.Minute)
	if got := r.FindSpoke("node-1").Status; got != StatusStale {
		t.Fatalf("old hb → stale, got %q", got)
	}

	// Offline status is sticky against time-based reconciliation.
	if err := r.SetStatus("node-1", StatusOffline); err != nil {
		t.Fatalf("set offline: %v", err)
	}
	r.FindSpoke("node-1").LastHeartbeat = now.Add(-1 * time.Second)
	r.ReconcileLiveness(now, 2*time.Minute)
	if got := r.FindSpoke("node-1").Status; got != StatusOffline {
		t.Fatalf("offline should be sticky, got %q", got)
	}

	// Degraded status is sticky too.
	_ = r.SetStatus("node-1", StatusDegraded)
	r.ReconcileLiveness(now, 2*time.Minute)
	if got := r.FindSpoke("node-1").Status; got != StatusDegraded {
		t.Fatalf("degraded should be sticky, got %q", got)
	}

	// SetStatus rejects unknown values.
	if err := r.SetStatus("node-1", SpokeStatus("bogus")); err == nil {
		t.Fatalf("expected error for invalid status")
	}
	// SetStatus reports unknown node.
	if err := r.SetStatus("nope", StatusActive); err == nil {
		t.Fatalf("expected error for unknown node")
	}
}

func TestUpsertAndRemove(t *testing.T) {
	r := NewRegistry()
	if err := r.UpsertSpoke(SpokeRecord{NodeID: "", Name: "x"}); err == nil {
		t.Fatalf("expected error for empty node_id")
	}
	if err := r.UpsertSpoke(SpokeRecord{NodeID: "a", Status: SpokeStatus("???")}); err == nil {
		t.Fatalf("expected error for bad status on upsert")
	}

	// Default status when omitted on upsert.
	if err := r.UpsertSpoke(SpokeRecord{NodeID: "a", Name: "a"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := r.FindSpoke("a").Status; got != StatusRegistered {
		t.Fatalf("default status: got %q", got)
	}

	// Replace by node_id.
	_ = r.UpsertSpoke(SpokeRecord{NodeID: "a", Name: "a2", Status: StatusActive})
	if r.FindSpoke("a").Name != "a2" {
		t.Fatalf("upsert replace failed")
	}
	if len(r.Spokes) != 1 {
		t.Fatalf("upsert duplicated row")
	}

	if !r.RemoveSpoke("a") {
		t.Fatalf("remove existing returned false")
	}
	if r.RemoveSpoke("a") {
		t.Fatalf("remove missing returned true")
	}
}

func TestConcurrentWriteSafety(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "federation-state.json")

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			r := NewRegistry()
			_ = r.UpsertSpoke(SpokeRecord{
				NodeID:        "node-" + string(rune('a'+i)),
				Name:          "n",
				URL:           "https://x",
				Status:        StatusActive,
				LastHeartbeat: time.Now().UTC(),
			})
			if err := SaveStateTo(path, r); err != nil {
				t.Errorf("concurrent save: %v", err)
			}
		}()
	}
	wg.Wait()

	// File must be parseable after the storm — atomic rename guarantees this.
	got, err := LoadStateFrom(path)
	if err != nil {
		t.Fatalf("load after concurrent writes: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema_version corrupted under concurrency: %q", got.SchemaVersion)
	}
	// No leftover tmpfiles.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover tmpfile after concurrent saves: %s", e.Name())
		}
	}
}

func TestSchemaMigrationStub(t *testing.T) {
	// Files written without schema_version (pre-versioned) load as
	// CurrentSchemaVersion. This is the migration stub: the read path is
	// where future version dispatch will hook in.
	dir := t.TempDir()
	path := filepath.Join(dir, "federation-state.json")
	legacy := []byte(`{"spokes":[{"node_id":"n1","name":"old","url":"https://o","status":"active","registered_at":"2026-05-02T19:58:33Z","last_heartbeat":"2026-05-02T20:00:00Z"}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	r, err := LoadStateFrom(path)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if r.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("legacy file should default to current schema_version, got %q", r.SchemaVersion)
	}
	if len(r.Spokes) != 1 || r.Spokes[0].NodeID != "n1" {
		t.Fatalf("legacy spokes not preserved: %+v", r.Spokes)
	}
}

func TestDefaultStatePathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-fake")
	p, err := DefaultStatePath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	want := filepath.Join("/tmp/xdg-fake", "ddx", "federation-state.json")
	if p != want {
		t.Fatalf("path: got %q want %q", p, want)
	}

	t.Setenv("XDG_DATA_HOME", "")
	p, err = DefaultStatePath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !strings.HasSuffix(p, filepath.Join(".local", "share", "ddx", "federation-state.json")) {
		t.Fatalf("default path missing ~/.local/share/ddx suffix: %q", p)
	}
}
