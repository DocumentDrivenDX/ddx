package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %s: %v", strings.Join(args, " "), dir, string(out), err)
	}
	return strings.TrimSpace(string(out))
}

// setupBacklinkRepo creates a git repo with a base commit (one bead in
// .ddx/beads.jsonl, the originator), then commits a tip that adds a second
// bead and a non-bead artifact. Returns the repo dir, base SHA, tip SHA,
// originator ID, and the affected bead ID (the new one added on tip).
func setupBacklinkRepo(t *testing.T) (dir, baseSHA, tipSHA, originatorID, affectedBeadID string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@test.local")
	if err := os.MkdirAll(filepath.Join(dir, ".ddx"), 0o755); err != nil {
		t.Fatal(err)
	}

	originatorID = "op-originator"
	affectedBeadID = "op-affected"

	baseLine := `{"id":"` + originatorID + `","title":"originator","status":"open","issue_type":"operator-prompt"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".ddx", "beads.jsonl"), []byte(baseLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "base")
	baseSHA = runGit(t, dir, "rev-parse", "HEAD")

	// Tip: adds a second bead + an artifact change.
	addedLine := `{"id":"` + affectedBeadID + `","title":"affected","status":"open"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".ddx", "beads.jsonl"), []byte(baseLine+addedLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "concerns.md"), []byte("- operator-prompt-test\n"), 0o644); err == nil {
		// docs/ may not exist yet
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "concerns.md"), []byte("- operator-prompt-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "tip")
	tipSHA = runGit(t, dir, "rev-parse", "HEAD")
	return
}

// AC: every bead created/changed by an operator-prompt execution gets an
// origin_operator_prompt_id event referencing the originator. Artifacts
// touched by the same execution show up in the originator's blast-radius
// summary event.
func TestComputeOperatorPromptAffected_DiffsBeadsAndArtifacts(t *testing.T) {
	dir, baseSHA, tipSHA, originatorID, affectedID := setupBacklinkRepo(t)

	got, err := computeOperatorPromptAffected(dir, baseSHA, tipSHA, originatorID)
	if err != nil {
		t.Fatalf("compute affected: %v", err)
	}
	if len(got.BeadIDs) != 1 || got.BeadIDs[0] != affectedID {
		t.Errorf("affected bead IDs: want [%q], got %v", affectedID, got.BeadIDs)
	}
	// Originator must be filtered out even if its line appears in the diff.
	for _, id := range got.BeadIDs {
		if id == originatorID {
			t.Errorf("originator must not be back-linked to itself")
		}
	}
	wantArtifact := filepath.ToSlash("docs/concerns.md")
	found := false
	for _, a := range got.Artifacts {
		if a == wantArtifact {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("want artifact %q in %v", wantArtifact, got.Artifacts)
	}
}

// AC: recordOperatorPromptBacklinks appends an origin_operator_prompt_id
// event on every affected bead and a blast-radius summary event on the
// originator. Both are queryable via the bead store afterwards.
func TestRecordOperatorPromptBacklinks_AppendsEventsOnAffectedAndOriginator(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"op\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	originator := bead.NewOperatorPromptBead("test prompt", bead.DefaultPriority)
	if err := store.Create(originator); err != nil {
		t.Fatal(err)
	}
	affected := &bead.Bead{
		ID:        "op-aaaa1111",
		Title:     "affected",
		Status:    bead.StatusOpen,
		IssueType: "task",
	}
	if err := store.Create(affected); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := recordOperatorPromptBacklinks(store, originator.ID, operatorPromptAffected{
		BeadIDs:   []string{affected.ID},
		Artifacts: []string{"docs/concerns.md"},
	}, "test-actor", now); err != nil {
		t.Fatalf("record backlinks: %v", err)
	}

	affEvents, err := store.EventsByKind(affected.ID, OperatorPromptBacklinkEventKind)
	if err != nil {
		t.Fatalf("events on affected: %v", err)
	}
	if len(affEvents) != 1 {
		t.Fatalf("affected bead must carry exactly 1 origin_operator_prompt_id event, got %d", len(affEvents))
	}
	if !strings.Contains(affEvents[0].Body, originator.ID) {
		t.Errorf("affected event body must reference originator %q, got %q", originator.ID, affEvents[0].Body)
	}

	origEvents, err := store.EventsByKind(originator.ID, OperatorPromptBacklinkEventKind)
	if err != nil {
		t.Fatalf("events on originator: %v", err)
	}
	if len(origEvents) != 1 {
		t.Fatalf("originator must carry exactly 1 blast-radius summary event, got %d", len(origEvents))
	}
	body := origEvents[0].Body
	if !strings.Contains(body, affected.ID) {
		t.Errorf("originator summary body must list affected bead %q, got %q", affected.ID, body)
	}
	if !strings.Contains(body, "docs/concerns.md") {
		t.Errorf("originator summary body must list artifact path, got %q", body)
	}
}

// AC: empty affected set is a no-op (no events appended). Used to verify
// that an operator-prompt bead whose execution touched nothing does not
// pollute the bead-event ledger.
func TestRecordOperatorPromptBacklinks_EmptyAffectedNoEvents(t *testing.T) {
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"op\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := bead.NewStore(ddxDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	originator := bead.NewOperatorPromptBead("noop prompt", bead.DefaultPriority)
	if err := store.Create(originator); err != nil {
		t.Fatal(err)
	}
	if err := recordOperatorPromptBacklinks(store, originator.ID, operatorPromptAffected{}, "test", time.Now().UTC()); err != nil {
		t.Fatalf("noop backlinks: %v", err)
	}
	events, err := store.EventsByKind(originator.ID, OperatorPromptBacklinkEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("empty affected must append no events, got %d", len(events))
	}
}
