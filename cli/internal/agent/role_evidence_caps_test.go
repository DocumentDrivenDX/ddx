package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

func resolvedRoleCapsForTest(role string, maxPromptBytes int) config.ResolvedConfig {
	return (&config.NewConfig{
		EvidenceCaps: &config.EvidenceCapsConfig{
			PerRole: map[string]*config.EvidenceCapsOverride{
				role: {MaxPromptBytes: &maxPromptBytes},
			},
		},
		// Mocked GitOps over a synthetic non-Git project root, so the mockable
		// linked-worktree backend must be selected explicitly rather than
		// inheriting the sandbox-safe local-clone default, which shells out to
		// a real git clone (attempt_backend.go:87-96).
	}).Resolve(config.CLIOverrides{AttemptBackend: AttemptBackendWorktree})
}

func TestImplementerAndLifecycleUseRoleEvidenceCaps(t *testing.T) {
	t.Run("ExecuteBeadWithConfig initial prompt", func(t *testing.T) {
		const beadID = "ddx-role-cap-initial"
		root := setupArtifactTestProjectRoot(t)
		gitOps := &artifactTestGitOps{
			projectRoot: root,
			baseRev:     "aaaa000000000001",
			resultRev:   "aaaa000000000001",
			wtSetupFn: func(wtPath string) {
				setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
			},
		}
		calls := 0
		runner := preClaimIntakeHookRunnerFunc(func(RunArgs) (*Result, error) {
			calls++
			return &Result{ExitCode: 0}, nil
		})
		_, err := ExecuteBeadWithConfig(context.Background(), root, beadID,
			resolvedRoleCapsForTest(config.EvidenceRoleImplementer, 1),
			ExecuteBeadRuntime{AgentRunner: runner}, gitOps)
		if err == nil || calls != 0 || !strings.Contains(err.Error(), "synthesized implementer prompt exceeds prompt cap") {
			t.Fatalf("ExecuteBeadWithConfig err=%v calls=%d, want implementer cap before dispatch", err, calls)
		}
	})

	t.Run("ExecuteBeadWithConfig repair prompt", func(t *testing.T) {
		const beadID = "ddx-role-cap-repair"
		root := setupArtifactTestProjectRoot(t)
		gitOps := &artifactTestGitOps{
			projectRoot: root,
			baseRev:     "bbbb000000000001",
			resultRev:   "bbbb000000000002",
			wtSetupFn: func(wtPath string) {
				setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
			},
		}
		repairCalls := 0
		runtime := ExecuteBeadRuntime{
			AgentRunner: &artifactTestAgentRunner{},
			Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
				review := repairCycleFixableReview()
				review.Rationale = strings.Repeat("reviewer fixable gap ", 8000)
				return review, nil
			}),
			Repair: repairPassFunc(func(context.Context, CandidateResult, string) (CandidateResult, error) {
				repairCalls++
				return CandidateResult{}, nil
			}),
			RepairMaxCycles:   1,
			CandidateRefStore: &inMemoryCandidateRefStore{},
			candidateDiff: func(CandidateResult) (string, error) {
				return "diff --git a/a.go b/a.go", nil
			},
		}
		res, err := ExecuteBeadWithConfig(context.Background(), root, beadID,
			resolvedRoleCapsForTest(config.EvidenceRoleImplementer, 100*1024), runtime, gitOps)
		if err != nil {
			t.Fatal(err)
		}
		if res == nil {
			t.Fatal("ExecuteBeadWithConfig returned nil repair result")
		}
		if res.Status != ExecuteBeadStatusExecutionFailed || repairCalls != 0 || !strings.Contains(res.Error, "implementer repair prompt exceeds prompt cap") {
			t.Fatalf("ExecuteBeadWithConfig repair status=%v resultError=%q repairCalls=%d, want cap before repair dispatch", res.Status, res.Error, repairCalls)
		}
	})

	t.Run("production repair diff closure", func(t *testing.T) {
		root, baseRev := initTestGitRepo(t)
		resultRev := commitTestFile(t, root, "large.go", strings.Repeat("changed line\n", 200), "large candidate")
		caps := evidence.DefaultCaps()
		caps.MaxDiffBytes = 64
		runtime := ExecuteBeadRuntime{}
		configureImplementerRepairEvidence(&runtime, caps)
		if runtime.candidateDiff == nil {
			t.Fatal("production repair diff closure was not installed")
		}
		diff, err := runtime.candidateDiff(CandidateResult{
			WorktreePath: root,
			Report:       ExecuteBeadReport{BaseRev: baseRev, ResultRev: resultRev},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(diff) > caps.MaxDiffBytes || !strings.Contains(diff, evidence.TruncationMarker) {
			t.Fatalf("repair diff len=%d content=%q, want role cap %d with marker", len(diff), diff, caps.MaxDiffBytes)
		}
	})

	rcfg := resolvedRoleCapsForTest(config.EvidenceRoleLifecycle, 1)

	t.Run("preclaim", func(t *testing.T) {
		root := newPreClaimIntakeHookTestRoot(t)
		store, b := newPreClaimIntakeHookTestStore(t, root)
		calls := 0
		runner := preClaimIntakeHookRunnerFunc(func(RunArgs) (*Result, error) {
			calls++
			return &Result{ExitCode: 0}, nil
		})
		got, err := NewPreClaimIntakeHook(root, store, rcfg, nil, runner)(context.Background(), b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome != PreClaimIntakeError || calls != 0 || !strings.Contains(got.Detail, "prompt cap") {
			t.Fatalf("preclaim result=%+v calls=%d, want cap rejection before dispatch", got, calls)
		}
	})

	t.Run("lint", func(t *testing.T) {
		root := newLintHookTestRoot(t)
		store, b := newLintHookTestStore(t, root)
		calls := 0
		runner := preClaimIntakeHookRunnerFunc(func(RunArgs) (*Result, error) {
			calls++
			return &Result{ExitCode: 0}, nil
		})
		_, err := NewPreDispatchLintHook(root, store, rcfg, nil, runner)(context.Background(), b.ID)
		var hookErr *LintHookError
		if !errors.As(err, &hookErr) || calls != 0 || !strings.Contains(err.Error(), "prompt cap") {
			t.Fatalf("lint err=%v calls=%d, want cap rejection before dispatch", err, calls)
		}
	})

	t.Run("triage", func(t *testing.T) {
		root := newTriageHookTestRoot(t)
		store, b := newTriageHookTestStore(t, root)
		calls := 0
		runner := preClaimIntakeHookRunnerFunc(func(RunArgs) (*Result, error) {
			calls++
			return &Result{ExitCode: 0}, nil
		})
		_, err := NewPostAttemptTriageHook(root, store, rcfg, nil, runner, nil)(context.Background(), b.ID, ExecuteBeadReport{
			BeadID:    b.ID,
			Status:    ExecuteBeadStatusExecutionFailed,
			Detail:    "test failure",
			ResultRev: "deadbeef",
		})
		if err == nil || calls != 0 || !strings.Contains(err.Error(), "prompt cap") {
			t.Fatalf("triage err=%v calls=%d, want cap rejection before dispatch", err, calls)
		}
	})

	t.Run("decomposer", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		if err := store.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		b := &bead.Bead{
			ID:          "ddx-role-cap-decomposer",
			Title:       "split lifecycle work",
			Description: strings.Repeat("large lifecycle task ", 20),
			Acceptance:  "1. preserve all requirements",
		}
		if err := store.Create(context.Background(), b); err != nil {
			t.Fatal(err)
		}
		calls := 0
		runner := preClaimIntakeHookRunnerFunc(func(RunArgs) (*Result, error) {
			calls++
			return &Result{ExitCode: 0}, nil
		})
		decomp, err := NewPreClaimDecompositionHook(store, runner, rcfg, t.TempDir())(context.Background(), b.ID)
		if err != nil || decomp == nil {
			t.Fatalf("decomposer fallback err=%v result=%+v", err, decomp)
		}
		if calls != 0 || !strings.Contains(decomp.Rationale, "deterministic fallback") || !strings.Contains(decomp.Rationale, "prompt cap") {
			t.Fatalf("decomposer calls=%d rationale=%q, want lifecycle-cap fallback before dispatch", calls, decomp.Rationale)
		}
	})
}
