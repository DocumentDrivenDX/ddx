package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// beadReviewerFunc is a test-local functional adapter implementing BeadReviewer.
type beadReviewerFunc func(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error)

func (f beadReviewerFunc) ReviewBead(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error) {
	return f(ctx, beadID, resultRev, impl)
}

type beadReviewGroupFunc func(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error)

func (f beadReviewGroupFunc) ReviewBead(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error) {
	group, err := f(ctx, beadID, resultRev, impl)
	if err != nil || group == nil {
		return nil, err
	}
	for _, slot := range group.Slots {
		if slot.Result != nil {
			return slot.Result, nil
		}
	}
	return nil, fmt.Errorf("review group did not return a slot result")
}

func (f beadReviewGroupFunc) ReviewGroup(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
	return f(ctx, beadID, resultRev, impl)
}

// runnerRunWithConfig is the test-only equivalent of the retired
// Runner.RunWithConfig. It assembles RunArgs from a sealed ResolvedConfig +
// AgentRunRuntime and delegates to Run. Production callers use
// RunWithConfigViaService.
func runnerRunWithConfig(r *Runner, ctx context.Context, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	sessionLogDir := runtime.SessionLogDirOverride
	if sessionLogDir == "" {
		sessionLogDir = rcfg.SessionLogDir()
	}
	opts := RunArgs{
		Context:       ctx,
		Harness:       rcfg.Harness(),
		Prompt:        runtime.Prompt,
		PromptFile:    runtime.PromptFile,
		PromptSource:  runtime.PromptSource,
		Correlation:   runtime.Correlation,
		Model:         rcfg.Model(),
		Provider:      rcfg.Provider(),
		Effort:        rcfg.Effort(),
		Timeout:       rcfg.Timeout(),
		WallClock:     rcfg.WallClock(),
		WorkDir:       runtime.WorkDir,
		Permissions:   rcfg.Permissions(),
		SessionLogDir: sessionLogDir,
		Env:           runtime.Env,
	}
	return r.Run(opts)
}

// runnerCapabilities is the test-only equivalent of the retired
// Runner.Capabilities. Production callers use CapabilitiesViaService.
func runnerCapabilities(r *Runner, name string) (*HarnessCapabilities, error) {
	harness, harnessName, err := r.resolveHarness(RunArgs{Harness: name})
	if err != nil {
		return nil, err
	}
	caps := &HarnessCapabilities{
		Harness:             harnessName,
		Available:           true,
		Binary:              harness.Binary,
		ReasoningLevels:     r.resolveReasoningLevels(harnessName, harness),
		Surface:             harness.Surface,
		CostClass:           harness.CostClass,
		IsLocal:             harness.IsLocal,
		ExactPinSupport:     harness.ExactPinSupport,
		SupportsEffort:      harness.EffortFlag != "",
		SupportsPermissions: len(harness.PermissionArgs) > 0,
	}
	if path, err := r.LookPath(harness.Binary); err == nil {
		caps.Path = path
	}
	model := r.resolveModel(RunArgs{}, harnessName)
	if model == "" {
		model = harness.DefaultModel
	}
	if model != "" {
		caps.Model = model
		caps.Models = []string{model}
	}
	return caps, nil
}

// runnerValidateForExecuteLoop is the test-only equivalent of the retired
// Runner.ValidateForExecuteLoop. Production callers use
// ValidateForExecuteLoopViaService.
func runnerValidateForExecuteLoop(r *Runner, harnessName, model, _provider string) error {
	if harnessName == "" {
		return nil
	}
	_, _, err := r.resolveHarness(RunArgs{Harness: harnessName})
	if err != nil {
		return err
	}
	return nil
}

// runnerTestProviderConnectivity is the test-only equivalent of the retired
// Runner.TestProviderConnectivity. Production callers use
// TestProviderConnectivityViaService.
func runnerTestProviderConnectivity(r *Runner, harnessName string, timeout time.Duration) ProviderStatus {
	status := ProviderStatus{Reachable: false}
	if harnessName == "virtual" || harnessName == "agent" {
		status.Reachable = true
		status.CreditsOK = true
		return status
	}
	harness, ok := r.registry.Get(harnessName)
	if !ok {
		status.Error = "unknown harness"
		return status
	}
	if _, err := r.LookPath(harness.Binary); err != nil {
		status.Error = "binary not found"
		return status
	}
	opts := RunArgs{Harness: harnessName, Prompt: "echo ok", Timeout: timeout}
	start := time.Now()
	result, err := r.Run(opts)
	duration := time.Since(start)
	if err != nil {
		status.Error = fmt.Sprintf("probe failed: %v (%.0fs)", err, duration.Seconds())
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "quota") ||
			strings.Contains(errStr, "credit") || strings.Contains(errStr, "insufficient") {
			status.CreditsOK = false
		}
		return status
	}
	if result.ExitCode != 0 || result.Error != "" {
		errStr := strings.ToLower(result.Error)
		status.Error = fmt.Sprintf("probe failed: %s (%.0fs)", result.Error, duration.Seconds())
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "quota") ||
			strings.Contains(errStr, "credit") || strings.Contains(errStr, "insufficient") {
			status.CreditsOK = false
		}
		return status
	}
	status.Reachable = true
	status.CreditsOK = true
	return status
}

// runQuorumWithConfig is the test-only equivalent of the retired
// RunQuorumWithConfig. Production callers use RunQuorumWithConfigViaService.
func runQuorumWithConfig(run RunFunc, _rcfg config.ResolvedConfig, runtime QuorumRuntime) ([]*Result, error) {
	return RunQuorumWith(run, runtime)
}

func TestExecutionTrace_AdapterPreservesCycleFields(t *testing.T) {
	original := ExecuteBeadReport{
		BeadID:    "ddx-trace",
		AttemptID: "attempt-trace-002",
		WorkerID:  "worker-1",
		BaseRev:   "base-rev",
		ResultRev: "result-rev",
		CycleTrace: []ExecutionCycleTrace{
			{
				CycleIndex: 0,
				AttemptID:  "attempt-trace-002",
				ResultRev:  "candidate-rev",
				ImplementerRoute: ExecutionCycleRouteFacts{
					Harness:         "codex",
					Provider:        "openai",
					Model:           "gpt-5",
					ActualPower:     70,
					RouteReason:     "profile_hint",
					ResolvedBaseURL: "https://api.openai.com",
				},
				ReviewGroupID:   "rg-1",
				ReviewerIndices: []int{0, 1},
				ReviewVerdicts:  []string{"BLOCK", "BLOCK"},
				ReviewResult:    ExecutionCycleReviewResult{Verdict: "REQUEST_CHANGES", Rationale: "fix gap", Classification: ReviewFindingClassFixableGap, PerAC: []ReviewAC{{Number: 1, Item: "Add test", Grade: "REQUEST_CHANGES", Evidence: "missing coverage"}}, Findings: []Finding{{Severity: "warn", Summary: "missing coverage", Location: "file.go:1"}}},
				FinalDecision:   ExecuteBeadStatusReviewFixableGap,
			},
		},
	}

	legacy := toTryReport(original)
	require.Len(t, legacy.CycleTrace, 1)
	assert.Equal(t, original.CycleTrace[0].CycleIndex, legacy.CycleTrace[0].CycleIndex)
	assert.Equal(t, original.CycleTrace[0].AttemptID, legacy.CycleTrace[0].AttemptID)
	assert.Equal(t, original.CycleTrace[0].ResultRev, legacy.CycleTrace[0].ResultRev)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.Harness, legacy.CycleTrace[0].ImplementerRoute.Harness)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.Provider, legacy.CycleTrace[0].ImplementerRoute.Provider)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.Model, legacy.CycleTrace[0].ImplementerRoute.Model)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.ActualPower, legacy.CycleTrace[0].ImplementerRoute.ActualPower)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.RouteReason, legacy.CycleTrace[0].ImplementerRoute.RouteReason)
	assert.Equal(t, original.CycleTrace[0].ImplementerRoute.ResolvedBaseURL, legacy.CycleTrace[0].ImplementerRoute.ResolvedBaseURL)
	assert.Equal(t, original.CycleTrace[0].ReviewGroupID, legacy.CycleTrace[0].ReviewGroupID)
	assert.Equal(t, original.CycleTrace[0].ReviewerIndices, legacy.CycleTrace[0].ReviewerIndices)
	assert.Equal(t, original.CycleTrace[0].ReviewVerdicts, legacy.CycleTrace[0].ReviewVerdicts)
	assert.Equal(t, original.CycleTrace[0].ReviewResult.Verdict, legacy.CycleTrace[0].ReviewResult.Verdict)
	assert.Equal(t, original.CycleTrace[0].ReviewResult.Rationale, legacy.CycleTrace[0].ReviewResult.Rationale)
	assert.Equal(t, original.CycleTrace[0].ReviewResult.Classification, legacy.CycleTrace[0].ReviewResult.Classification)
	assert.Equal(t, original.CycleTrace[0].ReviewResult.PerAC[0].Evidence, legacy.CycleTrace[0].ReviewResult.PerAC[0].Evidence)
	assert.Equal(t, original.CycleTrace[0].ReviewResult.Findings[0].Summary, legacy.CycleTrace[0].ReviewResult.Findings[0].Summary)
	assert.Equal(t, original.CycleTrace[0].FinalDecision, legacy.CycleTrace[0].FinalDecision)

	roundTripped := fromTryReport(legacy)
	assert.Equal(t, original, roundTripped)
}
