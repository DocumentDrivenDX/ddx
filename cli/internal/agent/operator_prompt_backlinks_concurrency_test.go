package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// AC: "two operator prompts mutating same bead exercises land-coordinator
// conflict path with no silent corruption". This test runs two parallel
// recordOperatorPromptBacklinks calls — one per concurrent operator-prompt
// originator — both targeting the same affected bead. The bead store's
// per-bead serialisation (Update + WithLock) is the agent-side analogue of
// the land coordinator: both back-link events MUST land in order, neither
// origin's audit trail may be lost or interleaved into a malformed entry.
func TestRecordOperatorPromptBacklinks_ConcurrentTargetSameBead(t *testing.T) {
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

	originatorA := bead.NewOperatorPromptBead("prompt A", bead.DefaultPriority)
	originatorB := bead.NewOperatorPromptBead("prompt B", bead.DefaultPriority)
	if err := store.Create(originatorA); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(originatorB); err != nil {
		t.Fatal(err)
	}
	target := &bead.Bead{
		ID:        "op-shared-target",
		Title:     "shared",
		Status:    bead.StatusOpen,
		IssueType: "task",
	}
	if err := store.Create(target); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	now := time.Now().UTC()
	errs := make([]error, 2)
	go func() {
		defer wg.Done()
		errs[0] = recordOperatorPromptBacklinks(store, originatorA.ID, operatorPromptAffected{
			BeadIDs: []string{target.ID},
		}, "actor-a", now)
	}()
	go func() {
		defer wg.Done()
		errs[1] = recordOperatorPromptBacklinks(store, originatorB.ID, operatorPromptAffected{
			BeadIDs: []string{target.ID},
		}, "actor-b", now)
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent backlink %d: %v", i, err)
		}
	}

	// Target must carry exactly two origin_operator_prompt_id events — one
	// per originator. Neither may have silently dropped (corruption).
	events, err := store.EventsByKind(target.ID, OperatorPromptBacklinkEventKind)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("target must carry 2 backlink events under concurrent origins, got %d", len(events))
	}
	bodies := []string{events[0].Body, events[1].Body}
	hasA, hasB := false, false
	for _, body := range bodies {
		if body == "origin_operator_prompt_id="+originatorA.ID {
			hasA = true
		}
		if body == "origin_operator_prompt_id="+originatorB.ID {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("both originators must appear in target events; bodies=%v", bodies)
	}

	// Each originator must carry its own blast-radius summary event.
	for _, origID := range []string{originatorA.ID, originatorB.ID} {
		summaries, err := store.EventsByKind(origID, OperatorPromptBacklinkEventKind)
		if err != nil {
			t.Fatal(err)
		}
		if len(summaries) != 1 {
			t.Errorf("originator %s must carry exactly 1 summary event, got %d", origID, len(summaries))
		}
	}
}
