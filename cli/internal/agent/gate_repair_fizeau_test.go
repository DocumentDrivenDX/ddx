package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gateRepairRecordingService struct {
	*passthroughTestService
	onExecute func(agentlib.ServiceExecuteRequest)
	finalData func(call int) []byte
}

func newGateRepairRecordingService() *gateRepairRecordingService {
	return &gateRepairRecordingService{passthroughTestService: &passthroughTestService{}}
}

func (s *gateRepairRecordingService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	s.lastReq = req
	s.executeRequests = append(s.executeRequests, req)
	if s.onExecute != nil {
		s.onExecute(req)
	}
	finalData := []byte(`{"status":"success","final_text":"ok"}`)
	if s.finalData != nil {
		finalData = s.finalData(len(s.executeRequests))
	}
	ch := make(chan agentlib.ServiceEvent, 1)
	ch <- agentlib.ServiceEvent{Type: "final", Data: finalData}
	close(ch)
	return ch, nil
}

func gateRepairResolvedConfig(harness, provider, model string) config.ResolvedConfig {
	base := config.NewTestConfigForRun(config.TestRunConfigOpts{
		Permissions: "sealed-permission",
	})
	return base.Resolve(config.CLIOverrides{
		Harness:     harness,
		Provider:    provider,
		Model:       model,
		Profile:     "sealed-policy",
		Effort:      "high",
		Permissions: "sealed-permission",
		MinPower:    2,
		MaxPower:    7,
	})
}

func newProductionRepairPass(t *testing.T, root string, cfg config.ResolvedConfig, runtime AgentRunRuntime, service agentlib.FizeauService) *fizeauCandidateRepairPass {
	t.Helper()
	return &fizeauCandidateRepairPass{
		projectRoot: root,
		workspace: &AttemptWorkspace{
			Backend:     AttemptBackendWorktree,
			ProjectRoot: root,
			WorkDir:     root,
		},
		backend:   WorktreeAttemptBackend{},
		service:   service,
		config:    cfg,
		runtime:   runtime,
		gitOps:    &RealGitOps{},
		artifacts: &executeBeadArtifacts{DirAbs: t.TempDir()},
	}
}

func repairCandidate(beadID, baseRev, resultRev, root string) CandidateResult {
	return CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:            beadID,
			AttemptID:         "attempt-1",
			Status:            ExecuteBeadStatusSuccess,
			BaseRev:           baseRev,
			ResultRev:         resultRev,
			ImplementationRev: resultRev,
		},
		WorktreePath: root,
	}
}

func installRepairableResultCheck(t *testing.T, root string) {
	t.Helper()
	checkDir := ddxroot.JoinProject(root, "checks")
	require.NoError(t, os.MkdirAll(checkDir, 0o755))
	checkYAML := `name: repairable-result
when: pre_merge
command: |
  if grep -q '^fixed$' result.txt; then status=pass; else status=block; fi
  printf '{"status":"%s","message":"result.txt must contain fixed"}\n' "$status" > "${EVIDENCE_DIR}/${CHECK_NAME}.json"
`
	commitTestFile(t, root, ddxroot.JoinRelative("checks", "repairable-result.yaml"), checkYAML, "test: add repairable close gate")
}

func TestGateFailureUsesFreshFizeauExecuteAtSamePower(t *testing.T) {
	root, _ := newScriptHarnessRepo(t, 1)
	installRepairableResultCheck(t, root)

	cfg := gateRepairResolvedConfig("", "", "")
	service := newGateRepairRecordingService()
	service.finalData = func(call int) []byte {
		if call == 1 {
			return []byte(`{"status":"success","final_text":"implemented","cost_usd":0.25,"cost_source":"reported"}`)
		}
		return []byte(`{"status":"success","final_text":"repaired","cost_usd":0.5,"cost_source":"reported"}`)
	}
	service.onExecute = func(req agentlib.ServiceExecuteRequest) {
		if strings.Contains(req.Prompt, "Repair bead") {
			commitTestFile(t, req.WorkDir, "result.txt", "fixed\n", "fix: repair candidate")
			return
		}
		commitTestFile(t, req.WorkDir, "result.txt", "broken\n", "feat: initial implementation")
	}

	reviewCalls := 0
	res, err := ExecuteBeadWithConfig(context.Background(), root, "ddx-int-0001", cfg, ExecuteBeadRuntime{
		Service: service,
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			body, readErr := os.ReadFile(filepath.Join(candidate.WorktreePath, "result.txt"))
			require.NoError(t, readErr)
			assert.Equal(t, "fixed\n", string(body))
			return CandidateReviewResult{Verdict: "APPROVE"}, nil
		}),
		RepairMaxCycles: 1,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, 1, reviewCalls)
	require.Len(t, service.executeRequests, 2, "repair must be a second fresh Fizeau Execute")
	assert.Equal(t, 2, service.executeRequests[0].MinPower)
	assert.Equal(t, service.executeRequests[0].MinPower, service.executeRequests[1].MinPower)
	assert.Equal(t, service.executeRequests[0].Permissions, service.executeRequests[1].Permissions)
	assert.Equal(t, "sealed-permission", service.executeRequests[0].Permissions)
	assert.InDelta(t, 0.75, res.CostUSD, 1e-9, "durable result must include primary plus repair cost")
}

func TestGateFailureLocalCloneUsesFreshFizeauRepairAndImportsResult(t *testing.T) {
	root, _ := newScriptHarnessRepo(t, 1)
	installRepairableResultCheck(t, root)
	mainBefore := runGitInteg(t, root, "rev-parse", "HEAD")
	backend := &recordingCandidateTransportBackend{inner: LocalCloneAttemptBackend{}}
	service := newGateRepairRecordingService()
	service.onExecute = func(req agentlib.ServiceExecuteRequest) {
		if strings.Contains(req.Prompt, "Repair bead") {
			commitTestFile(t, req.WorkDir, "result.txt", "fixed\n", "fix: local clone repair")
			return
		}
		commitTestFile(t, req.WorkDir, "result.txt", "broken\n", "feat: local clone implementation")
	}
	reviewCalls := 0

	res, err := ExecuteBeadWithConfig(context.Background(), root, "ddx-int-0001", gateRepairResolvedConfig("", "", ""), ExecuteBeadRuntime{
		Service:        service,
		AttemptBackend: backend,
		Reviewer: candidateReviewerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateReviewResult, error) {
			reviewCalls++
			body, readErr := os.ReadFile(filepath.Join(candidate.WorktreePath, "result.txt"))
			require.NoError(t, readErr)
			assert.Equal(t, "fixed\n", string(body))
			return CandidateReviewResult{Verdict: "APPROVE"}, nil
		}),
		RepairMaxCycles: 1,
	}, &RealGitOps{})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, 1, reviewCalls)
	require.Len(t, service.executeRequests, 2, "local clone repair must be a second Fizeau Execute")
	require.Len(t, backend.imports, 2, "initial and repaired clone revisions must each be imported")
	assert.NotEqual(t, backend.imports[0].ResultRev, backend.imports[1].ResultRev)
	assert.Equal(t, res.ResultRev, backend.imports[1].ResultRev)
	assert.Equal(t, 2, backend.releases)
	require.Len(t, backend.publishes, 1, "only the repaired result may be published")
	assert.Equal(t, res.ResultRev, backend.publishes[0].ResultRev)
	pinnedRev, pinErr := gitRevParse(t, root, res.CandidateRef)
	require.NoError(t, pinErr)
	assert.Equal(t, res.ResultRev, pinnedRev)
	assert.Equal(t, mainBefore, runGitInteg(t, root, "rev-parse", "refs/heads/main"))
}

func TestGateRepairCarriesTaskDiffAndFailureOutput(t *testing.T) {
	var repairPrompt string
	checks := 0
	coord := &AttemptCycleCoordinator{
		Pass: staticCandidateResultPass{candidate: CandidateResult{
			Report: ExecuteBeadReport{BeadID: "ddx-prompt", AttemptID: "attempt", BaseRev: "base", ResultRev: "candidate", Status: ExecuteBeadStatusSuccess},
		}},
		Checks: candidateCheckRunnerFunc(func(context.Context, string, CandidateResult) (CandidateCheckResult, error) {
			checks++
			return CandidateCheckResult{Passed: checks > 1, Detail: "go test: TestRepair failed"}, nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, prompt string) (CandidateResult, error) {
			repairPrompt = prompt
			candidate.Report.ResultRev = "repaired"
			candidate.Report.Status = ExecuteBeadStatusSuccess
			return candidate, nil
		}),
		NoReview:     true,
		RefStore:     &inMemoryCandidateRefStore{},
		OriginalTask: "AC1: repair the deterministic gate",
		CandidateDiff: func(CandidateResult) (string, error) {
			return "diff --git a/gate.go b/gate.go", nil
		},
	}

	_, err := coord.Run(context.Background(), "ddx-prompt")
	require.NoError(t, err)
	assert.Contains(t, repairPrompt, "AC1: repair the deterministic gate")
	assert.Contains(t, repairPrompt, "diff --git a/gate.go b/gate.go")
	assert.Contains(t, repairPrompt, "go test: TestRepair failed")
}

func TestGateRepairRunsChecksBeforeReviewer(t *testing.T) {
	var order []string
	checks := 0
	coord := &AttemptCycleCoordinator{
		Pass: staticCandidateResultPass{candidate: CandidateResult{
			Report: ExecuteBeadReport{BeadID: "ddx-order", AttemptID: "attempt", BaseRev: "base", ResultRev: "candidate", Status: ExecuteBeadStatusSuccess},
		}},
		Checks: candidateCheckRunnerFunc(func(context.Context, string, CandidateResult) (CandidateCheckResult, error) {
			order = append(order, "checks")
			checks++
			return CandidateCheckResult{Passed: checks > 1, Detail: "first gate failed"}, nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, _ string) (CandidateResult, error) {
			order = append(order, "repair")
			candidate.Report.ResultRev = "repaired"
			candidate.Report.Status = ExecuteBeadStatusSuccess
			return candidate, nil
		}),
		Reviewer: candidateReviewerFunc(func(context.Context, string, CandidateResult) (CandidateReviewResult, error) {
			order = append(order, "review")
			return CandidateReviewResult{Verdict: "APPROVE"}, nil
		}),
		RefStore: &inMemoryCandidateRefStore{},
	}

	_, err := coord.Run(context.Background(), "ddx-order")
	require.NoError(t, err)
	assert.Equal(t, []string{"checks", "repair", "checks", "review"}, order)
}

func TestGateRepairPreservesPassthroughAndDoesNotSetConcreteRoute(t *testing.T) {
	for _, tc := range []struct {
		name              string
		harness, provider string
		model             string
	}{
		{name: "route neutral remains route neutral"},
		{name: "opaque operator passthrough remains byte identical", harness: "opaque-harness", provider: "opaque-provider", model: "opaque-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, baseRev := initTestGitRepo(t)
			implRev := commitTestFile(t, root, "route.txt", "broken\n", "feat: route-neutral implementation")
			cfg := gateRepairResolvedConfig(tc.harness, tc.provider, tc.model)
			runtime := AgentRunRuntime{
				WorkDir:          root,
				Role:             "implementer",
				CorrelationID:    "ddx-route:attempt",
				MinPowerOverride: 4,
			}
			service := newGateRepairRecordingService()
			service.onExecute = func(req agentlib.ServiceExecuteRequest) {
				commitTestFile(t, req.WorkDir, "route.txt", "fixed\n", "fix: route-neutral repair")
			}

			_, err := newProductionRepairPass(t, root, cfg, runtime, service).Repair(
				context.Background(),
				repairCandidate("ddx-route", baseRev, implRev, root),
				"repair the route-neutral candidate",
			)
			require.NoError(t, err)
			require.Len(t, service.executeRequests, 1)
			req := service.executeRequests[0]
			assert.Equal(t, tc.harness, req.Harness)
			assert.Equal(t, tc.provider, req.Provider)
			assert.Equal(t, tc.model, req.Model)
			assert.Equal(t, "sealed-policy", req.Policy)
			assert.Equal(t, 4, req.MinPower)
			assert.Equal(t, 7, req.MaxPower)
			assert.Equal(t, "sealed-permission", req.Permissions)
		})
	}
}

func TestGateRepairRejectsTwoCommits(t *testing.T) {
	root, baseRev := initTestGitRepo(t)
	implRev := commitTestFile(t, root, "history.txt", "broken\n", "feat: initial implementation")
	service := newGateRepairRecordingService()
	service.onExecute = func(req agentlib.ServiceExecuteRequest) {
		commitTestFile(t, req.WorkDir, "repair-one.txt", "one\n", "fix: first repair commit")
		commitTestFile(t, req.WorkDir, "repair-two.txt", "two\n", "fix: second repair commit")
	}

	_, err := newProductionRepairPass(t, root, gateRepairResolvedConfig("", "", ""), AgentRunRuntime{WorkDir: root}, service).Repair(
		context.Background(),
		repairCandidate("ddx-two-commits", baseRev, implRev, root),
		"repair with exactly one commit",
	)
	require.ErrorContains(t, err, "must append exactly one commit; got 2")
}

func TestGateRepairRejectsRewrittenHistory(t *testing.T) {
	root, baseRev := initTestGitRepo(t)
	implRev := commitTestFile(t, root, "history.txt", "broken\n", "feat: initial implementation")
	service := newGateRepairRecordingService()
	service.onExecute = func(req agentlib.ServiceExecuteRequest) {
		out, resetErr := exec.Command("git", "-C", req.WorkDir, "reset", "--hard", baseRev).CombinedOutput()
		require.NoError(t, resetErr, "git reset: %s", out)
		commitTestFile(t, req.WorkDir, "rewritten.txt", "replacement\n", "fix: replacement history")
	}

	_, err := newProductionRepairPass(t, root, gateRepairResolvedConfig("", "", ""), AgentRunRuntime{WorkDir: root}, service).Repair(
		context.Background(),
		repairCandidate("ddx-rewritten", baseRev, implRev, root),
		"repair without rewriting history",
	)
	require.ErrorContains(t, err, "rewrote candidate history")
}

func TestGateRepairDoesNotInvokeProviderCLI(t *testing.T) {
	root, baseRev := initTestGitRepo(t)
	implRev := commitTestFile(t, root, "tripwire.txt", "broken\n", "feat: tripwire implementation")
	tripwireDir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "provider-cli-invoked")
	for _, providerCLI := range []string{"claude", "codex", "gemini"} {
		script := fmt.Sprintf("#!/bin/sh\ntouch %q\nexit 97\n", sentinel)
		require.NoError(t, os.WriteFile(filepath.Join(tripwireDir, providerCLI), []byte(script), 0o755))
	}
	t.Setenv("PATH", tripwireDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	service := newGateRepairRecordingService()
	service.onExecute = func(req agentlib.ServiceExecuteRequest) {
		commitTestFile(t, req.WorkDir, "tripwire.txt", "fixed\n", "fix: service-only repair")
	}
	cfg := gateRepairResolvedConfig("", "", "")
	_, err := newProductionRepairPass(t, root, cfg, AgentRunRuntime{WorkDir: root, MinPowerOverride: 4}, service).Repair(
		context.Background(),
		repairCandidate("ddx-tripwire", baseRev, implRev, root),
		"repair without invoking a provider CLI",
	)
	require.NoError(t, err)
	assert.True(t, service.executeCalled, "repair must cross FizeauService.Execute")
	_, statErr := os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr), "DDx invoked a provider CLI directly")

	tripwireName := "clau" + "de"
	tripwirePath, lookErr := exec.LookPath(tripwireName)
	require.NoError(t, lookErr, "provider tripwire must resolve through PATH")
	positiveControlErr := exec.Command(tripwirePath).Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, positiveControlErr, &exitErr, "provider tripwire must be executable through PATH")
	assert.Equal(t, 97, exitErr.ExitCode(), "tripwire positive control must reach the poison provider CLI")
	require.FileExists(t, sentinel, "provider CLI PATH observer was not armed")
}
