package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingCandidateTransportBackend struct {
	inner     AttemptBackend
	imports   []ExecuteBeadResult
	publishes []ExecuteBeadResult
	releases  int
	importErr error
}

func (b *recordingCandidateTransportBackend) Name() string { return b.inner.Name() }
func (b *recordingCandidateTransportBackend) Prepare(ctx context.Context, req AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	return b.inner.Prepare(ctx, req)
}
func (b *recordingCandidateTransportBackend) Run(ctx context.Context, req AttemptBackendRunRequest) (*Result, error) {
	return b.inner.Run(ctx, req)
}
func (b *recordingCandidateTransportBackend) ImportCandidate(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	if res != nil {
		b.imports = append(b.imports, *res)
	}
	if b.importErr != nil {
		return b.importErr
	}
	return b.inner.ImportCandidate(ctx, ws, res)
}

type failingPinCandidateRefStore struct {
	calls int
	err   error
}

func (s *failingPinCandidateRefStore) PinCandidateRef(string, string, int, string) (string, error) {
	s.calls++
	return "", s.err
}

func (*failingPinCandidateRefStore) UnpinCandidateRef(string, string) error { return nil }
func (b *recordingCandidateTransportBackend) ReleaseCandidateImport(ctx context.Context, ws *AttemptWorkspace) error {
	b.releases++
	return b.inner.ReleaseCandidateImport(ctx, ws)
}
func (b *recordingCandidateTransportBackend) PublishResult(ctx context.Context, ws *AttemptWorkspace, res *ExecuteBeadResult) error {
	if res != nil {
		b.publishes = append(b.publishes, *res)
	}
	return b.inner.PublishResult(ctx, ws, res)
}
func (b *recordingCandidateTransportBackend) Cleanup(ctx context.Context, ws *AttemptWorkspace) error {
	return b.inner.Cleanup(ctx, ws)
}

func TestExecuteBeadWithConfig_RecordsCandidateCycleMetadata(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"append-line output.txt candidate cycle integration",
		"commit chore: candidate cycle integration",
	})

	runner := scriptHarnessAgentRunner{}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	require.NotEmpty(t, res.CandidateRef, "successful worker results must carry a pinned candidate ref")
	assert.Equal(t, 0, res.CycleIndex)
	require.Len(t, res.CycleTrace, 1, "the worker candidate cycle must record one initial implementation cycle")

	candidateRev := res.ImplementationRev
	if candidateRev == "" {
		candidateRev = res.ResultRev
	}
	got, err := gitRevParse(t, projectRoot, res.CandidateRef)
	require.NoError(t, err)
	assert.Equal(t, candidateRev, got, "candidate ref must remain reachable from the project root after the worktree is removed")
	assert.Equal(t, candidateRev, res.CycleTrace[0].ResultRev)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.CycleTrace[0].FinalDecision)
}

func TestExecuteBeadLinkedRepairEvidenceRejectedBeforeImportPinOrPublish(t *testing.T) {
	testExecuteBeadRepairEvidenceRejectedBeforeImportPinOrPublish(t, WorktreeAttemptBackend{}, config.CLIOverrides{Harness: "script"})
}

func TestExecuteBeadLocalCloneRepairEvidenceRejectedBeforeImportPinOrPublish(t *testing.T) {
	testExecuteBeadRepairEvidenceRejectedBeforeImportPinOrPublish(t, LocalCloneAttemptBackend{}, config.CLIOverrides{
		Harness:        "script",
		AttemptBackend: AttemptBackendLocalClone,
	})
}

func testExecuteBeadRepairEvidenceRejectedBeforeImportPinOrPublish(t *testing.T, inner AttemptBackend, overrides config.CLIOverrides) {
	t.Helper()
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	mainBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{
		"create-file initial.txt initial-candidate",
		"commit feat: initial candidate",
	})
	backend := &recordingCandidateTransportBackend{inner: inner}
	overrides.Model = directivePath
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(overrides)

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner:    scriptHarnessAgentRunner{},
		AttemptBackend: backend,
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			evidenceRel := filepath.Join(ddxroot.DirName, "executions", candidate.Report.AttemptID, "repair-report.md")
			require.NoError(t, os.WriteFile(filepath.Join(candidate.WorktreePath, evidenceRel), []byte("invalid repaired evidence\n"), 0o644))
			out, addErr := exec.Command("git", "-C", candidate.WorktreePath, "add", "-f", "--", evidenceRel).CombinedOutput()
			require.NoError(t, addErr, "git add repaired evidence: %s", out)
			out, commitErr := exec.Command("git", "-C", candidate.WorktreePath, "commit", "-m", "repair: force-add execution evidence").CombinedOutput()
			require.NoError(t, commitErr, "git commit repaired evidence: %s", out)
			repairedRev, revErr := exec.Command("git", "-C", candidate.WorktreePath, "rev-parse", "HEAD").Output()
			require.NoError(t, revErr)
			repaired := candidate
			repaired.Report.ResultRev = strings.TrimSpace(string(repairedRev))
			repaired.CycleIndex = candidate.CycleIndex + 1
			return repaired, nil
		}),
	}, &RealGitOps{})

	require.ErrorContains(t, err, "candidate history commit")
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
	assert.Equal(t, FailureModeAttemptIntegrity, res.FailureMode)
	assert.Equal(t, res.ResultRev, res.ImplementationRev, "repaired revision must replace provisional implementation provenance")
	require.Len(t, backend.imports, 1, "only the validated initial candidate may be imported")
	assert.NotEqual(t, res.ResultRev, backend.imports[0].ResultRev, "invalid repaired revision must never be imported")
	assert.Equal(t, 1, backend.releases)
	assert.Empty(t, backend.publishes, "invalid repaired result must never reach final PublishResult")
	assert.Empty(t, res.CandidateRef)
	assert.Empty(t, runGitInteg(t, projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/iterations/"))
	assert.Empty(t, runGitInteg(t, projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/attempt-backend/"))
	assert.Equal(t, mainBefore, runGitInteg(t, projectRoot, "rev-parse", "refs/heads/main"), "invalid repair must not land")
	assert.Empty(t, runGitInteg(t, projectRoot, "log", "main", "--format=%H", "--", ".ddx/executions"))
	reportPath := filepath.Join(projectRoot, filepath.FromSlash(res.ExecutionDir), "repair-report.md")
	reportBytes, readErr := os.ReadFile(reportPath)
	require.NoError(t, readErr)
	assert.Equal(t, "invalid repaired evidence\n", string(reportBytes))
}

func TestExecuteBeadLocalCloneValidRepairImportsPinsProjectsAndPublishesOnce(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	mainBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{
		"create-file initial-clone.txt initial-candidate",
		"commit feat: initial clone candidate",
	})
	backend := &recordingCandidateTransportBackend{inner: LocalCloneAttemptBackend{}}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{
		Harness:        "script",
		AttemptBackend: AttemptBackendLocalClone,
	})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner:    scriptHarnessAgentRunner{},
		AttemptBackend: backend,
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			if candidate.CycleIndex > 0 {
				return CandidateReviewResult{Verdict: "APPROVE", Rationale: "repair is valid"}, nil
			}
			return repairCycleFixableReview(), nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			repairPath := filepath.Join(candidate.WorktreePath, "valid-repair.txt")
			require.NoError(t, os.WriteFile(repairPath, []byte("valid repair\n"), 0o644))
			out, addErr := exec.Command("git", "-C", candidate.WorktreePath, "add", "--", "valid-repair.txt").CombinedOutput()
			require.NoError(t, addErr, "git add repair: %s", out)
			out, commitErr := exec.Command("git", "-C", candidate.WorktreePath, "commit", "-m", "repair: valid append-only repair").CombinedOutput()
			require.NoError(t, commitErr, "git commit repair: %s", out)
			repairedRev, revErr := exec.Command("git", "-C", candidate.WorktreePath, "rev-parse", "HEAD").Output()
			require.NoError(t, revErr)
			repaired := candidate
			repaired.Report.ResultRev = strings.TrimSpace(string(repairedRev))
			repaired.CycleIndex = candidate.CycleIndex + 1
			return repaired, nil
		}),
	}, &RealGitOps{})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, res.ResultRev, res.ImplementationRev)
	assert.Equal(t, 1, res.CycleIndex)
	require.Len(t, backend.imports, 2, "initial and repaired clone revisions must each be imported before pinning")
	assert.NotEqual(t, backend.imports[0].ResultRev, backend.imports[1].ResultRev)
	assert.Equal(t, res.ResultRev, backend.imports[1].ResultRev)
	assert.Equal(t, res.ImplementationRev, backend.imports[1].ImplementationRev)
	assert.Equal(t, 2, backend.releases)
	require.Len(t, backend.publishes, 1, "only the final projected result may be published")
	assert.Equal(t, res.ResultRev, backend.publishes[0].ResultRev)
	assert.Equal(t, res.ImplementationRev, backend.publishes[0].ImplementationRev)
	pinnedRev, pinErr := gitRevParse(t, projectRoot, res.CandidateRef)
	require.NoError(t, pinErr)
	assert.Equal(t, res.ResultRev, pinnedRev)
	assert.Empty(t, runGitInteg(t, projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/attempt-backend/candidate-"), "candidate transport refs must be short-lived")
	resultRef := attemptBackendResultRef("result", "ddx-int-0001", res.AttemptID)
	resultRefRev, refErr := gitRevParse(t, projectRoot, resultRef)
	require.NoError(t, refErr)
	assert.Equal(t, res.ResultRev, resultRefRev)
	assert.Equal(t, mainBefore, runGitInteg(t, projectRoot, "rev-parse", "refs/heads/main"), "candidate execution must not land directly")
}

func TestExecuteBeadCandidateImportAndPinFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name        string
		importErr   error
		pinStore    *failingPinCandidateRefStore
		wantPinCall int
	}{
		{name: "import failure", importErr: errors.New("injected candidate import failure")},
		{name: "pin failure", pinStore: &failingPinCandidateRefStore{err: errors.New("injected candidate pin failure")}, wantPinCall: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot, _ := newScriptHarnessRepo(t, 1)
			mainBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
			directivePath := filepath.Join(t.TempDir(), "directive.txt")
			writeDirectiveFile(t, directivePath, []string{
				"create-file transport.txt candidate",
				"commit feat: transport candidate",
			})
			backend := &recordingCandidateTransportBackend{inner: WorktreeAttemptBackend{}, importErr: tt.importErr}
			reviewCalls := 0
			rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath}).Resolve(config.CLIOverrides{Harness: "script"})

			res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
				AgentRunner:       scriptHarnessAgentRunner{},
				AttemptBackend:    backend,
				CandidateRefStore: tt.pinStore,
				Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
					reviewCalls++
					return CandidateReviewResult{Verdict: "APPROVE"}, nil
				}),
			}, &RealGitOps{})

			require.Error(t, err)
			require.NotNil(t, res)
			assert.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
			assert.Equal(t, ExecuteBeadStatusExecutionFailed, res.Status)
			assert.Equal(t, 1, res.ExitCode)
			assert.Equal(t, FailureModeLandRetry, res.FailureMode)
			assert.Zero(t, reviewCalls, "transport failure must stop before review")
			assert.Empty(t, backend.publishes, "transport failure must stop final publication")
			assert.Equal(t, 1, backend.releases, "transient import cleanup must run")
			if tt.pinStore != nil {
				assert.Equal(t, tt.wantPinCall, tt.pinStore.calls)
			}
			assert.Empty(t, res.CandidateRef)
			assert.Empty(t, runGitInteg(t, projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/iterations/"))
			assert.Equal(t, mainBefore, runGitInteg(t, projectRoot, "rev-parse", "refs/heads/main"))
		})
	}
}
