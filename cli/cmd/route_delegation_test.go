package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routeDelegationExecute(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent, 1)
	if req.Role != "" && req.Role != "implementer" {
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
	} else {
		// Route-delegation tests intentionally produce no implementation commit.
		// Make that a typed no-change result instead of relying on equal revisions
		// being interpreted as no_changes by the command wrapper.
		if attemptID := req.Metadata["attempt_id"]; attemptID != "" {
			bundleDir := filepath.Join(req.WorkDir, ddxroot.DirName, "executions", attemptID)
			if err := os.MkdirAll(bundleDir, 0o755); err != nil {
				return nil, err
			}
			rationale := "status: open\nreason: route-delegation fixture intentionally makes no implementation commit\nsuggested_action: retry with smart agent\n"
			if err := os.WriteFile(filepath.Join(bundleDir, "no_changes_rationale.txt"), []byte(rationale), 0o644); err != nil {
				return nil, err
			}
		}
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok","routing_actual":{"harness":"fizeau-chosen-harness","provider":"fizeau-chosen-provider","model":"fizeau-chosen-model","power":8}}`)}
	}
	close(ch)
	return ch, nil
}

func newRouteDelegationProject(t *testing.T, beadID string) string {
	t.Helper()
	dir := minimalProjectDir(t)
	configBytes, err := os.ReadFile(filepath.Join(ddxroot.JoinProject(dir), "config.yaml"))
	require.NoError(t, err)
	stateRoot := filepath.Join(dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(stateRoot, 0o755))
	configPath := filepath.Join(stateRoot, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, configBytes, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# route delegation\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	if beadID != "" {
		store := bead.NewStore(ddxroot.JoinProject(dir))
		require.NoError(t, store.Init(context.Background()))
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:          beadID,
			Title:       "execution: prove route delegation",
			IssueType:   "task",
			Description: "PROBLEM\nDDx must not choose Fizeau routes.\n\nROOT CAUSE\ncli/cmd/work.go dispatches execution.\n\nPROPOSED FIX\nCapture the opaque Execute request.\n\nNON-SCOPE\nDo not modify Fizeau routing.\n",
			Acceptance:  "1. TestLifecycleDispatchNeverQueriesFizeauCatalog passes\n2. TestExplicitOperatorRoutePassthroughPreservedByteForByte passes\n3. cd cli && go test ./cmd/... passes\n4. lefthook run pre-commit passes\n",
			Labels:      []string{"phase:build", "area:routing", "kind:test"},
		}))
	}
	return dir
}

func TestLifecycleDispatchNeverQueriesFizeauCatalog(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	dir := newRouteDelegationProject(t, "ddx-work-route-delegation")

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"work", "--once", "--project", dir,
		"--harness", "codex", "--profile", "smart",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "output=%q", out)

	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "work must reach Fizeau Execute; output=%q", out)
	assert.Empty(t, capturedRouteRequests(stub), "work must never call ResolveRoute before Execute")
	assert.Equal(t, "codex", requests[0].Harness, "explicit operator harness is opaque passthrough")
	assert.Equal(t, "smart", requests[0].Policy, "explicit operator profile is opaque passthrough")
	assert.Zero(t, requests[0].MinPower, "non-empty public policy must suppress difficulty inference")
	assert.Empty(t, requests[0].Model, "DDx must not fill in a model")
	assert.Empty(t, requests[0].Provider, "DDx must not fill in a provider")
	assert.Zero(t, modelQueriesBeforeExecute(stub), "initial work dispatch must not query the model catalog")
}

func TestWorkOperatorRouteConstraintsDoNotSuppressDifficultyInference(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	dir := newRouteDelegationProject(t, "ddx-work-sticky-route-constraints")

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"work", "--once", "--project", dir,
		"--harness", " opaque-harness ",
		"--provider", " opaque-provider ",
		"--model", " opaque-model ",
		"--max-power", "10",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "output=%q", out)

	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "work must reach Fizeau Execute; output=%q", out)
	req := requests[0]
	assert.Equal(t, 7, req.MinPower, "sticky route constraints must not suppress default difficulty inference")
	assert.Equal(t, 10, req.MaxPower)
	assert.Equal(t, " opaque-harness ", req.Harness)
	assert.Equal(t, " opaque-provider ", req.Provider)
	assert.Equal(t, " opaque-model ", req.Model)
	assert.Empty(t, req.Policy)
	assert.Empty(t, capturedRouteRequests(stub))
	assert.Zero(t, modelQueriesBeforeExecute(stub))
}

func TestInferredMinPowerConflictWithMaxPowerErrors(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	beadID := "ddx-work-inferred-power-conflict"
	dir := newRouteDelegationProject(t, beadID)
	store := bead.NewStore(ddxroot.JoinProject(dir))
	require.NoError(t, store.Update(context.Background(), beadID, func(target *bead.Bead) {
		if target.Extra == nil {
			target.Extra = map[string]any{}
		}
		target.Extra[escalation.BeadEstimatedDifficultyKey] = string(escalation.DifficultyHard)
	}))

	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"work", "--once", "--project", dir,
		"--max-power", "8",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "work reports per-bead failures in its structured output; output=%q", out)
	assert.Contains(t, out, "inferred MinPower 9 conflicts with requested MaxPower 8")
	assert.Empty(t, capturedImplementationRequests(stub), "conflicting bounds must fail before Fizeau dispatch")
}

func TestRunWithoutExplicitRouteLeavesSelectionToFizeau(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	dir := newRouteDelegationProject(t, "")
	out, err := executeCommand(NewCommandFactory(dir).NewRootCommand(),
		"run", "--min-power", "7", "--text", "hello", "--json", "--timeout", "5s")
	require.NoError(t, err, "output=%q", out)
	require.Len(t, stub.requests, 1)
	assertRouteNeutralRequest(t, stub.requests[0])
	assert.Contains(t, out, "fizeau-chosen-harness", "returned route is retained as evidence")
	assert.Empty(t, capturedRouteRequests(stub))
	assert.Zero(t, modelQueriesBeforeExecute(stub))
}

func TestTryWithoutExplicitRouteLeavesSelectionToFizeau(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	dir := newRouteDelegationProject(t, "ddx-try-route-delegation")
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	out, err := executeCommand(factory.NewRootCommand(),
		"try", "ddx-try-route-delegation", "--min-power", "7",
		"--no-review", "--no-review-i-know-what-im-doing")
	require.Error(t, err, "try currently reports no-changes as a non-zero command result; output=%q", out)
	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "try must reach Fizeau Execute; output=%q", out)
	assertRouteNeutralRequest(t, requests[0])
	assert.Empty(t, capturedRouteRequests(stub))
	assert.Zero(t, modelQueriesBeforeExecute(stub))
	assertReturnedRouteEvidence(t, dir, "ddx-try-route-delegation")
}

func TestWorkWithoutExplicitRouteLeavesSelectionToFizeau(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeFn = routeDelegationExecute
	dir := newRouteDelegationProject(t, "ddx-work-neutral-delegation")
	out, err := executeCommand(NewCommandFactory(dir).NewRootCommand(),
		"work", "--once", "--project", dir, "--min-power", "7",
		"--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "output=%q", out)
	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "work must reach Fizeau Execute; output=%q", out)
	assertRouteNeutralRequest(t, requests[0])
	assert.Empty(t, capturedRouteRequests(stub))
	assert.Zero(t, modelQueriesBeforeExecute(stub))
	assertReturnedRouteEvidence(t, dir, "ddx-work-neutral-delegation")
}

// TestExplicitOperatorRoutePassthroughPreservedByteForByte is the
// production-path tripwire for DDx/Fizeau routing ownership. A semantic
// failure may make the next request stronger, but DDx must preserve every
// operator constraint and must not inspect or pre-resolve Fizeau's route
// catalog between attempts.
func TestExplicitOperatorRoutePassthroughPreservedByteForByte(t *testing.T) {
	stub := exerciseEscalationRaisesOnlyMinPower(t)

	runDir := newRouteDelegationProject(t, "")
	runOut, runErr := executeCommand(
		NewCommandFactory(runDir).NewRootCommand(),
		"run", "--text", "review passthrough",
		"--harness", " opaque-run-harness ",
		"--provider", " opaque-run-provider ",
		"--model", " opaque-run-model ",
		"--profile", " opaque-run-profile ",
		"--effort", "high",
		"--permissions", "supervised",
		"--min-power", "7", "--max-power", "10",
	)
	require.NoError(t, runErr, "output=%q", runOut)
	stub.mu.Lock()
	runReq := stub.lastReq
	stub.mu.Unlock()
	assert.Equal(t, " opaque-run-harness ", runReq.Harness)
	assert.Equal(t, " opaque-run-provider ", runReq.Provider)
	assert.Equal(t, " opaque-run-model ", runReq.Model)
	assert.Equal(t, " opaque-run-profile ", runReq.Policy)
	assert.Equal(t, agentlib.Reasoning("high"), runReq.Reasoning)
	assert.Equal(t, "supervised", runReq.Permissions)
	assert.Equal(t, 7, runReq.MinPower)
	assert.Equal(t, 10, runReq.MaxPower)
}

func TestEscalationRaisesOnlyMinPowerWithoutCatalogLookup(t *testing.T) {
	exerciseEscalationRaisesOnlyMinPower(t)
}

func exerciseEscalationRaisesOnlyMinPower(t *testing.T) *executeCapturingStub {
	t.Helper()
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	implementationAttempt := 0
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Role != "implementer" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
			close(ch)
			return ch, nil
		}

		implementationAttempt++
		if implementationAttempt == 1 {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"build failed","routing_actual":{"power":7}}`)}
		} else {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok","routing_actual":{"power":8}}`)}
		}
		close(ch)
		return ch, nil
	}

	dir := newRouteDelegationProject(t, "ddx-escalation-route-delegation")
	out, err := executeCommand(
		NewCommandFactory(dir).NewRootCommand(),
		"work", "--once", "--project", dir,
		"--harness", " opaque-harness ",
		"--provider", " opaque-provider ",
		"--model", " opaque-model ",
		"--profile", " opaque-profile ",
		"--min-power", "7", "--max-power", "10",
		"--no-review", "--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "output=%q", out)

	requests := capturedImplementationRequests(stub)
	require.Len(t, requests, 2, "semantic failure must retry through the production work path; output=%q err=%v", out, err)
	assert.Equal(t, []int{7, 8}, []int{requests[0].MinPower, requests[1].MinPower})
	for _, req := range requests {
		assert.Equal(t, 10, req.MaxPower)
		assert.Equal(t, " opaque-harness ", req.Harness)
		assert.Equal(t, " opaque-provider ", req.Provider)
		assert.Equal(t, " opaque-model ", req.Model)
		assert.Equal(t, " opaque-profile ", req.Policy)
	}
	assert.Zero(t, capturedCatalogPreflightQueries(stub),
		"retry control must not call ResolveRoute, inventory/health APIs, RouteStatus, or RecordRouteAttempt")
	return stub
}

func TestTryPropagatesRouteResolutionTimeout(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.executeContextFn = func(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent)
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, nil
	}
	dir := newRouteDelegationProject(t, "ddx-try-route-timeout")
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	started := time.Now()
	out, err := executeCommand(factory.NewRootCommand(),
		"try", "ddx-try-route-timeout",
		"--route-resolution-timeout", "25ms",
		"--no-review", "--no-review-i-know-what-im-doing")
	if err != nil {
		var exitErr *ExitError
		require.ErrorAs(t, err, &exitErr, "unexpected try error: %v output=%q", err, out)
	}
	// The elapsed budget mostly covers command setup around the 25ms route
	// stage. Shared-host command startup and cleanup can push that path into
	// the high single-digit seconds even when the route timeout is still being
	// honored, so keep a hard ceiling that is still far below
	// DefaultRouteResolutionTimeout (60s) — an ignored deadline still fails.
	routeDeadlineBudget := 15 * time.Second
	if raceEnabled {
		routeDeadlineBudget = 20 * time.Second
	}
	assert.Less(t, time.Since(started), routeDeadlineBudget, "try must apply the requested route-stage deadline")

	events, eventsErr := bead.NewStore(ddxroot.JoinProject(dir)).Events("ddx-try-route-timeout")
	require.NoError(t, eventsErr)
	var found bool
	for _, event := range events {
		if event.Kind == "operator_attention" && event.Summary == agent.FailureModeRouteResolutionTimeout {
			found = true
			break
		}
	}
	assert.True(t, found, "try route timeout must release the lease and emit operator attention")
}

func assertReturnedRouteEvidence(t *testing.T, projectRoot, beadID string) {
	t.Helper()
	events, err := bead.NewStore(ddxroot.JoinProject(projectRoot)).Events(beadID)
	require.NoError(t, err)
	for _, event := range events {
		if strings.Contains(event.Body, "fizeau-chosen-harness") &&
			strings.Contains(event.Body, "fizeau-chosen-provider") &&
			strings.Contains(event.Body, "fizeau-chosen-model") {
			return
		}
	}
	t.Fatalf("Fizeau's returned route was not retained as evidence for %s", beadID)
}

func assertRouteNeutralRequest(t *testing.T, req agentlib.ServiceExecuteRequest) {
	t.Helper()
	assert.Equal(t, 7, req.MinPower)
	assert.Empty(t, req.Harness)
	assert.Empty(t, req.Provider)
	assert.Empty(t, req.Model)
	assert.Empty(t, req.Policy)
	assert.Empty(t, req.Reasoning)
	assert.Zero(t, req.ProviderTimeout)
}
