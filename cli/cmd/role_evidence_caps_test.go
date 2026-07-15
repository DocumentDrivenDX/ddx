package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

func TestReviewerUsesRoleEvidenceCapsInCLIAndServer(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)
	beadStoreRoot := filepath.Join(root, ddxroot.DirName)
	store := bead.NewStore(beadStoreRoot)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeBeadReviewConfig(t, root, `evidence_caps:
  max_prompt_bytes: 999999
  per_role:
    reviewer:
      max_prompt_bytes: 0
      max_inlined_file_bytes: 0
      max_diff_bytes: 0
      max_governing_doc_bytes: 0
`)
	b := &bead.Bead{
		ID:          "ddx-review-role-cap",
		Title:       "prove reviewer role caps reach prompt assembly",
		Description: "non-empty pinned review evidence",
		Acceptance:  "1. reviewer prompt is bounded",
		Extra:       map[string]any{"closing_commit_sha": "HEAD"},
	}
	if err := store.Create(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), b.ID, func(current *bead.Bead) {
		if current.Extra == nil {
			current.Extra = map[string]any{}
		}
		current.Extra["closing_commit_sha"] = "HEAD"
	}); err != nil {
		t.Fatal(err)
	}

	rcfg, err := config.LoadAndResolve(root, config.CLIOverrides{Harness: "opaque-harness", Provider: "opaque-provider", Model: "opaque-model"})
	if err != nil {
		t.Fatal(err)
	}
	reviewer := newCommandReviewer(root, beadStoreRoot, "elevated", &rcfg)
	if !reviewer.CapsConfigured || reviewer.Caps != (evidence.Caps{}) {
		t.Fatalf("CLI reviewer caps = %+v configured=%v, want explicit all-zero reviewer caps", reviewer.Caps, reviewer.CapsConfigured)
	}
	if _, err := reviewer.ReviewBead(context.Background(), b.ID, "HEAD", agent.ImplementerRouting{}); err == nil || !strings.Contains(err.Error(), "context_overflow") {
		t.Fatalf("post-merge reviewer did not apply constructor caps to bounded prompt: %v", err)
	}

	cmd := NewCommandFactory(root).newBeadReviewCommand()
	cmd.SetArgs([]string{b.ID})
	if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "reviewer role cap") {
		t.Fatalf("ddx bead review did not apply resolved reviewer caps: %v", err)
	}
}
