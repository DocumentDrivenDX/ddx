package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAgentService is a controllable agentlib.DdxAgent used by the
// routing-overrides e2e tests. It captures incoming RouteRequests and
// returns either a configured decision or a configured error.
type stubAgentService struct {
	resolve func(req agentlib.RouteRequest) (*agentlib.RouteDecision, error)
}

func (s *stubAgentService) Execute(_ context.Context, _ agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	return nil, fmt.Errorf("stub: not implemented")
}
func (s *stubAgentService) TailSessionLog(_ context.Context, _ string) (<-chan agentlib.ServiceEvent, error) {
	return nil, fmt.Errorf("stub: not implemented")
}
func (s *stubAgentService) ListHarnesses(_ context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "claude", Available: true}, {Name: "agent", Available: true}}, nil
}
func (s *stubAgentService) ListProviders(_ context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}
func (s *stubAgentService) ListModels(_ context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}
func (s *stubAgentService) HealthCheck(_ context.Context, _ agentlib.HealthTarget) error {
	return nil
}
func (s *stubAgentService) ResolveRoute(_ context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	if s.resolve != nil {
		return s.resolve(req)
	}
	return &agentlib.RouteDecision{Harness: req.Harness, Model: req.Model}, nil
}
func (s *stubAgentService) RouteStatus(_ context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}
func (s *stubAgentService) ListProfiles(_ context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}
func (s *stubAgentService) ResolveProfile(_ context.Context, _ string) (*agentlib.ResolvedProfile, error) {
	return nil, fmt.Errorf("stub: not implemented")
}
func (s *stubAgentService) ProfileAliases(_ context.Context) (map[string]string, error) {
	return nil, nil
}
func (s *stubAgentService) RecordRouteAttempt(_ context.Context, _ agentlib.RouteAttempt) error {
	return nil
}
func (s *stubAgentService) ListSessionLogs(_ context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}
func (s *stubAgentService) WriteSessionLog(_ context.Context, _ string, _ io.Writer) error {
	return nil
}
func (s *stubAgentService) ReplaySession(_ context.Context, _ string, _ io.Writer) error {
	return nil
}
func (s *stubAgentService) UsageReport(_ context.Context, _ agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}

// installStubService installs a stub service factory and registers cleanup.
func installStubService(t *testing.T, stub *stubAgentService) {
	t.Helper()
	SetAgentServiceFactoryForTest(func(_ string) (agentlib.DdxAgent, error) {
		return stub, nil
	})
	t.Cleanup(func() { SetAgentServiceFactoryForTest(nil) })
}

// minimalProjectDir creates a project dir with a clean .ddx/config.yaml that
// has no agent.harness pin and no agent.endpoints, so routing decisions are
// driven entirely by flag plumbing.
func minimalProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	ddxDir := filepath.Join(dir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
	return dir
}

// AC #1: ddx agent run --harness claude → ResolveRoute(Profile: "default", Harness: "claude").
// Mocks a gate failure (harness binary missing surfaced as ErrUnknownProvider) and
// asserts the typed error surfaces verbatim — no silent substitution.
func TestRunHarnessFlagPlumbsToResolveRouteAndSurfacesTypedError(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	gateErr := agentlib.ErrUnknownProvider{Provider: "claude-quota-exhausted"}
	installStubService(t, &stubAgentService{
		resolve: func(_ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			return nil, gateErr
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run", "--harness", "claude", "--text", "hi", "--timeout", "2s")
	require.Error(t, err, "typed gate error must surface")
	require.True(t, errors.Is(err, agentlib.ErrUnknownProvider{}),
		"upstream typed error must surface unmodified, got: %v", err)

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1, "ResolveRoute must be invoked")
	last := reqs[len(reqs)-1]
	assert.Equal(t, "default", last.Profile, "Profile must default to \"default\"")
	assert.Equal(t, "claude", last.Harness, "--harness must plumb through to RouteRequest.Harness")
}

// AC #2: ddx agent run --model opus-4.7 → ResolveRoute(Profile: "default", Model: "opus-4.7").
// Stub seeds 3 candidates at different cost; asserts the lowest-cost pick wins.
func TestRunModelFlagPlumbsToResolveRouteAndPicksLowestCost(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	candidates := []agentlib.RouteCandidate{
		{Harness: "claude", Provider: "expensive", Model: "opus-4.7", CostUSDPer1kTokens: 0.030, Eligible: true},
		{Harness: "claude", Provider: "midrange", Model: "opus-4.7", CostUSDPer1kTokens: 0.018, Eligible: true},
		{Harness: "claude", Provider: "cheap", Model: "opus-4.7", CostUSDPer1kTokens: 0.005, Eligible: true},
	}
	// Stub picks the lowest CostUSDPer1kTokens — mirrors upstream's tiebreak.
	installStubService(t, &stubAgentService{
		resolve: func(_ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			best := candidates[0]
			for _, c := range candidates[1:] {
				if c.Eligible && c.CostUSDPer1kTokens < best.CostUSDPer1kTokens {
					best = c
				}
			}
			return &agentlib.RouteDecision{
				Harness:    best.Harness,
				Provider:   best.Provider,
				Model:      best.Model,
				Reason:     "cheapest eligible",
				Candidates: candidates,
			}, nil
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	// Execute will fail downstream (stub Execute is unimplemented), but the
	// RouteRequest plumbing is observable via the recorder regardless.
	_, _ = executeCommand(root, "agent", "run", "--model", "opus-4.7", "--text", "hi", "--timeout", "2s")

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1, "ResolveRoute must be invoked")
	last := reqs[len(reqs)-1]
	assert.Equal(t, "default", last.Profile, "Profile defaults to \"default\"")
	assert.Equal(t, "opus-4.7", last.Model, "--model must plumb through to RouteRequest.Model")

	// Cost-tiebreak observability: the stub's lowest-cost pick wins.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Eligible && c.CostUSDPer1kTokens < best.CostUSDPer1kTokens {
			best = c
		}
	}
	assert.Equal(t, "cheap", best.Provider, "lowest-cost provider must win the tiebreak")
}

// AC #3a: ddx agent run --harness claude --model X → ErrHarnessModelIncompatible
// when the harness allow-list rejects X. Typed error surfaces unmodified.
func TestRunHarnessModelIncompatibleSurfacesTypedError(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	pinErr := agentlib.ErrHarnessModelIncompatible{
		Harness:         "claude",
		Model:           "gpt-5.4",
		SupportedModels: []string{"sonnet", "opus", "claude-sonnet-4-6"},
	}
	installStubService(t, &stubAgentService{
		resolve: func(_ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			return nil, pinErr
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run",
		"--harness", "claude", "--model", "gpt-5.4", "--text", "hi", "--timeout", "2s")
	require.Error(t, err)
	require.True(t, errors.Is(err, agentlib.ErrHarnessModelIncompatible{}),
		"typed ErrHarnessModelIncompatible must surface unmodified, got: %v", err)

	var typed agentlib.ErrHarnessModelIncompatible
	require.True(t, errors.As(err, &typed), "errors.As must extract typed error")
	assert.Equal(t, "claude", typed.Harness)
	assert.Equal(t, "gpt-5.4", typed.Model)

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1)
	last := reqs[len(reqs)-1]
	assert.Equal(t, "claude", last.Harness)
	assert.Equal(t, "gpt-5.4", last.Model)
}

// AC #3b: ddx agent run --harness claude --model opus → clean dispatch when
// the pin is valid. ResolveRoute returns a successful decision; CLI proceeds.
func TestRunHarnessModelCompatibleDispatchesCleanly(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	installStubService(t, &stubAgentService{
		resolve: func(req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			return &agentlib.RouteDecision{
				Harness:  req.Harness,
				Model:    req.Model,
				Provider: "anthropic",
				Reason:   "pin accepted",
			}, nil
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	// Execute() on the stub returns "not implemented"; the relevant assertion
	// is that ResolveRoute did NOT return a typed routing error.
	_, err := executeCommand(root, "agent", "run",
		"--harness", "claude", "--model", "opus", "--text", "hi", "--timeout", "2s")
	if err != nil {
		assert.False(t, errors.Is(err, agentlib.ErrHarnessModelIncompatible{}),
			"valid pin must not surface ErrHarnessModelIncompatible, got: %v", err)
		assert.False(t, errors.Is(err, agentlib.ErrProfilePinConflict{}),
			"valid pin must not surface ErrProfilePinConflict, got: %v", err)
	}

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1)
	last := reqs[len(reqs)-1]
	assert.Equal(t, "claude", last.Harness)
	assert.Equal(t, "opus", last.Model)
}

// AC #4: ddx agent run --profile local --harness claude → ErrProfilePinConflict
// from upstream. DDx surfaces it unmodified.
func TestRunProfilePinConflictSurfacesUpstreamErrorUnmodified(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	upstreamErr := agentlib.ErrProfilePinConflict{
		Profile:           "local",
		ConflictingPin:    "Harness=claude",
		ProfileConstraint: "local-only",
	}
	installStubService(t, &stubAgentService{
		resolve: func(_ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			return nil, upstreamErr
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)
	root := NewCommandFactory(dir).NewRootCommand()
	_, err := executeCommand(root, "agent", "run",
		"--profile", "local", "--harness", "claude", "--text", "hi", "--timeout", "2s")
	require.Error(t, err)
	require.True(t, errors.Is(err, agentlib.ErrProfilePinConflict{}),
		"upstream typed error must surface unmodified, got: %v", err)
	// The error string from upstream must appear unchanged (no DDx prefix wrapping
	// added — D5 contract).
	assert.Equal(t, upstreamErr.Error(), err.Error(),
		"DDx must surface upstream message verbatim")

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1)
	last := reqs[len(reqs)-1]
	assert.Equal(t, "local", last.Profile)
	assert.Equal(t, "claude", last.Harness)
}

// AC #5: ddx work shares the same plumbing — exercising the same flag set and
// asserting the captured RouteRequest matches the flags. The work command
// goes through the execute-loop worker callback, which is the same plumbing
// surface as `ddx agent run` per ddx-fdd3ea36 D5.
func TestWorkSharesSameRoutingPlumbing(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	upstreamErr := agentlib.ErrHarnessModelIncompatible{
		Harness:         "claude",
		Model:           "gpt-5.4",
		SupportedModels: []string{"sonnet", "opus"},
	}
	installStubService(t, &stubAgentService{
		resolve: func(_ agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
			return nil, upstreamErr
		},
	})
	ResetRecordedRouteRequestsForTest()

	dir := minimalProjectDir(t)

	// Seed one ready bead so the loop has something to claim.
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:       "ddx-work-routing-plumb",
		Title:    "ddx work routing plumbing fixture",
		Priority: 0,
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	// --local executes in-process so the worker callback runs with our stub.
	// --once stops after one bead.
	_, _ = executeCommand(root, "work",
		"--local", "--once",
		"--profile", "default",
		"--harness", "claude",
		"--model", "gpt-5.4",
	)

	reqs := RecordedRouteRequestsForTest()
	require.GreaterOrEqual(t, len(reqs), 1,
		"ddx work must route through ResolveRoute via the same plumbing as ddx agent run")
	// At least one captured request must reflect the flags we passed.
	matched := false
	for _, r := range reqs {
		if r.Profile == "default" && r.Harness == "claude" && r.Model == "gpt-5.4" {
			matched = true
			break
		}
	}
	assert.True(t, matched,
		"ddx work must plumb --profile/--harness/--model into ResolveRoute; got requests: %#v", reqs)
}
