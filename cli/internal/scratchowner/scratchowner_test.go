package scratchowner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOwnedScratchMarkerRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kind := KindFixtureBinary
	before := time.Now().UTC().Add(-time.Second)

	written, err := WriteForCurrentProcess(dir, kind)
	if err != nil {
		t.Fatalf("WriteForCurrentProcess: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	// Recorded fields: kind, owner PID, creation time, available start identity.
	if written.Kind != kind {
		t.Fatalf("kind: got %q want %q", written.Kind, kind)
	}
	if written.OwnerPID != os.Getpid() {
		t.Fatalf("owner_pid: got %d want %d", written.OwnerPID, os.Getpid())
	}
	if written.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}
	if written.CreatedAt.Before(before) || written.CreatedAt.After(after) {
		t.Fatalf("created_at %v outside write window [%v, %v]", written.CreatedAt, before, after)
	}
	if written.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at location: got %v want UTC", written.CreatedAt.Location())
	}

	availableIdentity := processStartIdentity(os.Getpid())
	if availableIdentity != "" {
		if written.ProcessStartIdentity != availableIdentity {
			t.Fatalf("process_start_identity: got %q want %q", written.ProcessStartIdentity, availableIdentity)
		}
	} else if runtime.GOOS == "linux" {
		t.Fatal("linux must expose process-start identity for the current process")
	}

	// Atomic write: final marker present, no leftover temp siblings.
	finalPath := Path(dir)
	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("marker file missing after write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == MarkerFileName {
			continue
		}
		if strings.HasPrefix(name, MarkerFileName+".tmp-") || strings.HasSuffix(name, ".tmp") {
			t.Fatalf("leftover temp file after atomic write: %s", name)
		}
	}

	// Round-trip through read.
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Kind != written.Kind {
		t.Fatalf("round-trip kind: got %q want %q", got.Kind, written.Kind)
	}
	if got.OwnerPID != written.OwnerPID {
		t.Fatalf("round-trip owner_pid: got %d want %d", got.OwnerPID, written.OwnerPID)
	}
	if !got.CreatedAt.Equal(written.CreatedAt) {
		t.Fatalf("round-trip created_at: got %v want %v", got.CreatedAt, written.CreatedAt)
	}
	if got.ProcessStartIdentity != written.ProcessStartIdentity {
		t.Fatalf("round-trip process_start_identity: got %q want %q", got.ProcessStartIdentity, written.ProcessStartIdentity)
	}

	// Live owner for the current process.
	status, evalMarker, err := Evaluate(dir)
	if err != nil {
		t.Fatalf("Evaluate live: %v", err)
	}
	if status != StatusLive {
		t.Fatalf("Evaluate live status: got %q want %q", status, StatusLive)
	}
	if evalMarker.OwnerPID != written.OwnerPID {
		t.Fatalf("Evaluate marker owner_pid: got %d want %d", evalMarker.OwnerPID, written.OwnerPID)
	}

	// Missing marker → unowned (not an ownership claim).
	emptyDir := t.TempDir()
	status, evalMarker, err = Evaluate(emptyDir)
	if err != nil {
		t.Fatalf("Evaluate missing: %v", err)
	}
	if status != StatusUnowned {
		t.Fatalf("missing marker status: got %q want %q", status, StatusUnowned)
	}
	if evalMarker.OwnerPID != 0 || evalMarker.Kind != "" {
		t.Fatalf("missing marker must not invent ownership fields: %+v", evalMarker)
	}

	// Malformed marker → uncertain (not an ownership claim).
	malformedDir := t.TempDir()
	if err := os.WriteFile(Path(malformedDir), []byte("not-json{{{"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	status, _, err = Evaluate(malformedDir)
	if err != nil {
		t.Fatalf("Evaluate malformed: %v", err)
	}
	if status != StatusUncertain {
		t.Fatalf("malformed marker status: got %q want %q", status, StatusUncertain)
	}

	// Incomplete marker (missing required fields) → uncertain.
	incompleteDir := t.TempDir()
	incomplete := Marker{Kind: kind} // no PID, no CreatedAt
	raw, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatalf("marshal incomplete: %v", err)
	}
	if err := os.WriteFile(Path(incompleteDir), raw, 0o644); err != nil {
		t.Fatalf("write incomplete: %v", err)
	}
	status, _, err = Evaluate(incompleteDir)
	if err != nil {
		t.Fatalf("Evaluate incomplete: %v", err)
	}
	if status != StatusUncertain {
		t.Fatalf("incomplete marker status: got %q want %q", status, StatusUncertain)
	}

	// Dead owner PID → dead (reclaimable by cleanup; not live ownership).
	deadDir := t.TempDir()
	dead := Marker{
		Kind:                 kind,
		OwnerPID:             1<<30 - 1, // extremely unlikely to be live
		CreatedAt:            time.Now().UTC(),
		ProcessStartIdentity: "linux-startticks:1",
	}
	if processAlive(dead.OwnerPID) {
		t.Skipf("pid %d unexpectedly alive; cannot prove dead-owner path", dead.OwnerPID)
	}
	if err := Write(deadDir, dead); err != nil {
		t.Fatalf("Write dead marker: %v", err)
	}
	status, _, err = Evaluate(deadDir)
	if err != nil {
		t.Fatalf("Evaluate dead: %v", err)
	}
	if status != StatusDead {
		t.Fatalf("dead owner status: got %q want %q", status, StatusDead)
	}

	// Directory name / prefix alone is never ownership: empty dir with a
	// DDx-looking basename still evaluates unowned.
	hostRoot := t.TempDir()
	prefixOnly := filepath.Join(hostRoot, "ddx-fixture-bin-fake")
	if err := os.MkdirAll(prefixOnly, 0o755); err != nil {
		t.Fatalf("mkdir prefix-only: %v", err)
	}
	status, _, err = Evaluate(prefixOnly)
	if err != nil {
		t.Fatalf("Evaluate prefix-only: %v", err)
	}
	if status != StatusUnowned {
		t.Fatalf("prefix-only status: got %q want %q (prefix must not claim ownership)", status, StatusUnowned)
	}
}
