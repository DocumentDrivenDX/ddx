package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

type roleCapServerRunner struct{ calls int }

func (r *roleCapServerRunner) Run(agent.RunArgs) (*agent.Result, error) {
	r.calls++
	return &agent.Result{ExitCode: 0, Output: "unexpected dispatch"}, nil
}

func TestReviewerUsesRoleEvidenceCapsInCLIAndServer(t *testing.T) {
	t.Run("worker reviewer constructor reaches bounded prompt", func(t *testing.T) {
		root := t.TempDir()
		initGitRepo(t, root)
		storeRoot := ddxroot.JoinProject(root)
		store := bead.NewStore(storeRoot)
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		b := &bead.Bead{ID: "ddx-server-review-cap", Title: "server reviewer role cap", Description: "pinned evidence"}
		if err := store.Create(context.Background(), b); err != nil {
			t.Fatal(err)
		}
		zero := 0
		rcfg := (&config.NewConfig{EvidenceCaps: &config.EvidenceCapsConfig{
			PerRole: map[string]*config.EvidenceCapsOverride{
				config.EvidenceRoleReviewer: {
					MaxPromptBytes:       &zero,
					MaxInlinedFileBytes:  &zero,
					MaxDiffBytes:         &zero,
					MaxGoverningDocBytes: &zero,
				},
			},
		}}).Resolve(config.CLIOverrides{Harness: "another-harness", Provider: "another-provider", Model: "another-model"})

		reviewer := newServerReviewer(root, storeRoot, "routine", rcfg)
		if !reviewer.CapsConfigured || reviewer.Caps != (evidence.Caps{}) {
			t.Fatalf("server reviewer caps = %+v configured=%v, want explicit all-zero", reviewer.Caps, reviewer.CapsConfigured)
		}
		if _, err := reviewer.ReviewBead(context.Background(), b.ID, "HEAD", agent.ImplementerRouting{}); err == nil || !strings.Contains(err.Error(), "context_overflow") {
			t.Fatalf("server worker reviewer did not apply caps to prompt: %v", err)
		}
	})

	t.Run("live review dispatcher renders with reviewer cap", func(t *testing.T) {
		root := t.TempDir()
		configDir := filepath.Join(root, ddxroot.DirName)
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		contents := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.invalid/library
    branch: main
evidence_caps:
  max_prompt_bytes: 999999
  per_role:
    reviewer:
      max_prompt_bytes: 0
      max_inlined_file_bytes: 0
      max_diff_bytes: 0
      max_governing_doc_bytes: 0
`
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		runner := &roleCapServerRunner{}
		dispatcher := &ReviewDispatcher{ProjectRoot: root, Runner: runner}
		_, err := dispatcher.DispatchReviewTurn(context.Background(), ReviewSession{
			ID:           "review-role-cap",
			ArtifactID:   "artifact",
			SystemRubric: "non-empty pinned rubric",
		}, ReviewTurn{Actor: "user", Content: "review this"})
		if err == nil || !strings.Contains(err.Error(), "PROMPT_BUDGET_EXCEEDED") || runner.calls != 0 {
			t.Fatalf("dispatcher err=%v calls=%d, want reviewer cap rejection before runner", err, runner.calls)
		}
	})
}
