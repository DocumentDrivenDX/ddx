package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	policyescalation "github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tryHookRunnerStub struct {
	t            *testing.T
	promptSource []string
}

type blockingTryExecutor struct {
	started chan struct{}
	once    sync.Once
}

type tryGitOpsStub struct {
	worktrees []string
	removed   []string
	added     []string
	pruned    bool
	refs      map[string]string
}

func (g *tryGitOpsStub) HeadRev(string) (string, error) { return "deadbeef", nil }

func (g *tryGitOpsStub) ResolveRev(string, string) (string, error) { return "deadbeef", nil }

func (g *tryGitOpsStub) WorktreeAdd(_ string, wtPath, _ string) error {
	g.added = append(g.added, wtPath)
	return nil
}

func (g *tryGitOpsStub) WorktreeRemove(_ string, wtPath string) error {
	g.removed = append(g.removed, wtPath)
	return nil
}

func (g *tryGitOpsStub) WorktreeList(string) ([]string, error) {
	return append([]string(nil), g.worktrees...), nil
}

func (g *tryGitOpsStub) WorktreePrune(string) error {
	g.pruned = true
	return nil
}

func (g *tryGitOpsStub) IsDirty(string) (bool, error) { return false, nil }

func (g *tryGitOpsStub) SynthesizeCommit(string, string) (bool, error) { return false, nil }

func (g *tryGitOpsStub) UpdateRef(_ string, ref, sha string) error {
	if g.refs == nil {
		g.refs = map[string]string{}
	}
	g.refs[ref] = sha
	return nil
}

func (g *tryGitOpsStub) DeleteRef(_ string, ref string) error {
	if g.refs != nil {
		delete(g.refs, ref)
	}
	return nil
}

func (e *blockingTryExecutor) Execute(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
	e.once.Do(func() { close(e.started) })
	<-ctx.Done()
	return agent.ExecuteBeadReport{
		BeadID:    beadID,
		Status:    agent.ExecuteBeadStatusExecutionFailed,
		Detail:    ctx.Err().Error(),
		BaseRev:   "feedface",
		ResultRev: "feedface",
	}, ctx.Err()
}

func runInterruptedTryCommand(t *testing.T) (*bead.Store, string, error) {
	t.Helper()

	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "try-interrupt-001",
		Title: "Interrupted try bead",
	}))

	factory := NewCommandFactory(env.Dir)
	runner := &blockingTryExecutor{started: make(chan struct{})}
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = runner
	root := factory.NewRootCommand()

	ctx, cancel := context.WithCancel(context.Background())
	root.SetContext(ctx)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"try", "try-interrupt-001", "--no-review", "--no-review-i-know-what-im-doing"})

	done := make(chan error, 1)
	go func() {
		done <- root.Execute()
	}()

	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("try executor did not start")
	}
	cancel()

	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("try command did not return after cancel")
	}

	return store, buf.String(), err
}

func (r *tryHookRunnerStub) Run(opts agent.RunArgs) (*agent.Result, error) {
	r.promptSource = append(r.promptSource, opts.PromptSource)
	switch opts.PromptSource {
	case "bead-lifecycle-intake":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"classification":"ready","rationale":"single-slice","readiness_checks":[]}`,
		}, nil
	case "bead-lifecycle-lint":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"score":9,"rationale":"ok","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	case "bead-lifecycle-triage":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"classification":"transport","recommended_action":"release_claim_retry","rationale":"transient","suggested_amendments":[],"suggested_followup_beads":[]}`,
		}, nil
	default:
		r.t.Fatalf("unexpected prompt source: %q", opts.PromptSource)
		return nil, nil
	}
}

// TestTry_BeadNotFound verifies that "ddx try unknown-id" exits non-zero
// and writes "bead not found: <id>" to stderr.
func TestTry_BeadNotFound(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	out, err := executeCommand(root, "try", "unknown-bead-xyz")
	assert.Error(t, err, "ddx try with unknown bead must exit non-zero")
	assert.Contains(t, out, "bead not found: unknown-bead-xyz",
		"stderr must contain the documented 'bead not found' message")
}

// TestTry_BeadClosed verifies that "ddx try <closed-bead-id>" exits non-zero
// and writes "bead is not claimable (status=closed)" to stderr.
func TestTry_BeadClosed(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "closed-bead-001",
		Title:  "A closed bead",
		Status: bead.StatusOpen,
	}))
	require.NoError(t, store.SetLifecycleStatus("closed-bead-001", bead.StatusClosed, bead.LifecycleTransitionOptions{ManualClose: true}))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "try", "closed-bead-001")
	assert.Error(t, err, "ddx try on a closed bead must exit non-zero")
	assert.Contains(t, out, "bead is not claimable (status=closed)",
		"stderr must name the closed status")
}

// TestTry_BeadCancelledNotClaimable verifies the same shape for cancelled beads.
func TestTry_BeadCancelledNotClaimable(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "cancelled-bead-001",
		Title:  "A cancelled bead",
		Status: bead.StatusOpen,
	}))
	require.NoError(t, store.SetLifecycleStatus("cancelled-bead-001", bead.StatusCancelled, bead.LifecycleTransitionOptions{}))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "try", "cancelled-bead-001")
	assert.Error(t, err, "ddx try on a cancelled bead must exit non-zero")
	assert.Contains(t, out, "bead is not claimable (status=cancelled)",
		"stderr must name the cancelled status")
}

// TestTry_BeadUnmetDeps verifies that "ddx try <bead-with-open-dep>" exits
// non-zero and writes "bead has unmet dependencies: <list>" to stderr.
func TestTry_BeadUnmetDeps(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())

	// Create the dependency first (store validates dep references exist).
	require.NoError(t, store.Create(&bead.Bead{
		ID:     "dep-bead-001",
		Title:  "Dependency bead (still open)",
		Status: bead.StatusOpen,
	}))
	require.NoError(t, store.Create(&bead.Bead{
		ID:           "target-bead-001",
		Title:        "Target bead with dep",
		Dependencies: []bead.Dependency{{IssueID: "target-bead-001", DependsOnID: "dep-bead-001", Type: "blocks"}},
	}))

	root := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(root, "try", "target-bead-001")
	assert.Error(t, err, "ddx try with unmet deps must exit non-zero")
	assert.Contains(t, out, "bead has unmet dependencies",
		"stderr must describe the unmet dep condition")
	assert.Contains(t, out, "dep-bead-001",
		"stderr must name the blocking dep ID")
}

func TestTry_BlocksStaleSourceBinaryBeforeClaim(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot, buildSHA, headSHA := seedStaleSourceCheckout(t)
	seedExecuteBead(t, projectRoot, &bead.Bead{
		ID:        "stale-try-bead",
		Title:     "stale source try bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})

	runner := &staleSourceRunnerProbe{t: t}
	executorCalled := false
	factory := NewCommandFactory(projectRoot)
	factory.Version = "0.9.0"
	factory.Commit = buildSHA
	factory.AgentRunnerOverride = runner
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		executorCalled = true
		t.Fatalf("executor must not run when stale-source preflight blocks: %s", beadID)
		return agent.ExecuteBeadReport{}, nil
	})

	out, err := executeCommand(factory.NewRootCommand(), "try", "stale-try-bead", "--project", projectRoot)
	require.Error(t, err)
	assert.Contains(t, out, "ddx try: installed ddx binary is stale for this DDx source checkout.")
	assert.Contains(t, out, "project root: "+projectRoot)
	assert.Contains(t, out, "binary commit: "+buildSHA)
	assert.Contains(t, out, "source HEAD: "+headSHA)
	assert.Contains(t, out, "recovery: cd "+projectRoot+" && make install")
	assert.False(t, executorCalled, "try must fail before execution dispatch")
	assert.Equal(t, 0, runner.calls, "try must fail before readiness hooks run")

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	got, getErr := store.Get("stale-try-bead")
	require.NoError(t, getErr)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	events, eventsErr := store.Events("stale-try-bead")
	require.NoError(t, eventsErr)
	assert.Empty(t, events, "try must fail before claim or attempt evidence is written")

	_, statErr := os.Stat(filepath.Join(projectRoot, agent.DefaultLogDir))
	assert.True(t, os.IsNotExist(statErr), "try must fail before attempt log setup")
}

// TestTry_HappyPath_ClaimsAndExecutes verifies the core AC: given a ready bead
// and a stub executor that returns success, the command claims the bead,
// invokes the executor, and exits zero.
func TestTry_HappyPath_ClaimsAndExecutes(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "happy-bead-001",
		Title: "Happy path bead",
	}))

	var executorCalled bool
	stubExecutor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		executorCalled = true
		assert.Equal(t, "happy-bead-001", beadID, "executor must receive the targeted bead ID")
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "deadbeef01234567", // non-empty so ClosureGate passes
		}, nil
	})

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = stubExecutor
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "try", "happy-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try with a successful stub executor must exit zero: %s", out)
	assert.True(t, executorCalled, "executor must have been called")

	// Verify the bead was claimed (status should be closed after success).
	b, storeErr := store.Get("happy-bead-001")
	require.NoError(t, storeErr)
	assert.Equal(t, bead.StatusClosed, b.Status, "bead must be closed after successful execution")
}

func TestTry_ForceClaimBypassesCooldown(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "cooldown-bead-001",
		Title: "Cooldown bead",
	}))
	require.NoError(t, store.SetExecutionCooldown(
		"cooldown-bead-001",
		time.Now().UTC().Add(6*time.Hour),
		agent.ExecuteBeadStatusNoChanges,
		"retry later",
		"base-rev-1",
	))

	var calls int
	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		calls++
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "forced-success-rev",
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try", "cooldown-bead-001",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "ddx try without --force-claim must respect retry cooldown")
	assert.Contains(t, out, "retry cooldown")
	assert.Equal(t, 0, calls, "executor must not run without --force-claim")

	out, err = executeCommand(
		factory.NewRootCommand(),
		"try", "cooldown-bead-001",
		"--force-claim",
		"--reason", "operator unblock",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "ddx try --force-claim must execute cooled bead: %s", out)
	assert.Equal(t, 1, calls, "executor must run once when cooldown is explicitly bypassed")

	got, getErr := store.Get("cooldown-bead-001")
	require.NoError(t, getErr)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestTry_ForceClaimRequiresReason(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "cooldown-bead-need-reason",
		Title: "Cooldown bead",
	}))

	out, err := executeCommand(
		NewCommandFactory(env.Dir).NewRootCommand(),
		"try", "cooldown-bead-need-reason",
		"--force-claim",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.Error(t, err)
	assert.Contains(t, out+err.Error(), "--force-claim requires --reason")
}

func TestTry_ForceClaimEmitsAuditEvent(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "cooldown-bead-audit",
		Title: "Cooldown bead audit",
	}))
	require.NoError(t, store.SetExecutionCooldown(
		"cooldown-bead-audit",
		time.Now().UTC().Add(6*time.Hour),
		agent.ExecuteBeadStatusNoChanges,
		"retry later",
		"base-rev-1",
	))

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "audit-success-rev",
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try", "cooldown-bead-audit",
		"--force-claim",
		"--reason", "operator unblock",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, out)

	events, eventsErr := store.Events("cooldown-bead-audit")
	require.NoError(t, eventsErr)

	var forceClaimEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "force-claim" {
			forceClaimEvent = &events[i]
			break
		}
	}
	require.NotNil(t, forceClaimEvent, "expected force-claim event")
	assert.Equal(t, resolveClaimAssignee(), forceClaimEvent.Actor)
	assert.False(t, forceClaimEvent.CreatedAt.IsZero())

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(forceClaimEvent.Body), &body))
	assert.Equal(t, "operator unblock", body["reason"])
	assert.NotEmpty(t, body["forced_at"])
}

func TestForceClaim_PreservesCooldownFieldOnFailure(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "cooldown-bead-preserve",
		Title: "Cooldown preserve bead",
	}))

	retryAfter := time.Now().UTC().Add(6 * time.Hour).Format(time.RFC3339)
	require.NoError(t, store.Update(context.Background(), "cooldown-bead-preserve", func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra[bead.ExtraRetryAfter] = retryAfter
		b.Extra[bead.ExtraCooldownBaseRev] = "base-rev-1"
		b.Extra[bead.ExtraLastStatus] = agent.ExecuteBeadStatusNoChanges
		b.Extra[bead.ExtraLastDetail] = "retry later"
	}))

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID: beadID,
			Status: agent.ExecuteBeadStatusExecutionFailed,
			Detail: "still failing",
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try", "cooldown-bead-preserve",
		"--force-claim",
		"--reason", "operator unblock",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.Error(t, err, "forced failure must still exit non-zero: %s", out)

	got, getErr := store.Get("cooldown-bead-preserve")
	require.NoError(t, getErr)
	assert.Equal(t, retryAfter, got.Extra[bead.ExtraRetryAfter])
	assert.Equal(t, "base-rev-1", got.Extra[bead.ExtraCooldownBaseRev])
	assert.Equal(t, agent.ExecuteBeadStatusNoChanges, got.Extra[bead.ExtraLastStatus])
	assert.Equal(t, "retry later", got.Extra[bead.ExtraLastDetail])
}

// TestTryRecordsExecutionRoutingIntent verifies that ddx try records the
// bead-hint routing intent evidence and prints the concise routing-intent
// line when the durable power hint metadata is present.
func TestTryRecordsExecutionRoutingIntent(t *testing.T) {
	env := NewTestEnvironment(t)
	skillPath := env.Dir + "/.agents/skills/ddx/bead-lifecycle"
	require.NoError(t, os.MkdirAll(skillPath, 0o755))
	require.NoError(t, os.WriteFile(skillPath+"/SKILL.md", []byte("lint"), 0o644))

	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "hint-bead-001",
		Title: "Hinted bead",
		Extra: map[string]any{policyescalation.BeadEstimatedDifficultyKey: string(policyescalation.DifficultyHard)},
	}))

	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:      beadID,
			Status:      agent.ExecuteBeadStatusSuccess,
			SessionID:   "sess-hint",
			ResultRev:   "deadbeef01234567",
			Harness:     "claude",
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-6",
			ActualPower: 91,
		}, nil
	})
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "try", "hint-bead-001", "--harness=claude", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try with a hard bead hint must succeed: %s", out)
	assert.Contains(t, out, "routing intent: difficulty=hard powerClass=smart source=bead_hint")

	events, err := store.Events("hint-bead-001")
	require.NoError(t, err)

	var intentEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-routing-intent" {
			intentEvent = &events[i]
			break
		}
	}
	require.NotNil(t, intentEvent, "expected execution-routing-intent evidence on the bead")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(intentEvent.Body), &body))
	assert.Equal(t, "bead_hint", body["routing_intent_source"])
	assert.Equal(t, "hard", body["estimated_difficulty"])
	assert.Equal(t, "smart", body["requested_power_class"])
	assert.Equal(t, "claude", body["actual_harness"])
	assert.Equal(t, "anthropic", body["actual_provider"])
	assert.Equal(t, "claude-sonnet-4-6", body["actual_model"])
	assert.Equal(t, float64(91), body["actual_power"])
}

func TestTryZeroConfigInferredTaskSelectsFizeauPolicyWithoutInitialMinPower(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:        "ddx-zero-config-try-powerClass-standard",
		Title:     "Try with inferred standard routing powerClass",
		IssueType: "bug",
	}))

	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"try",
		"ddx-zero-config-try-powerClass-standard",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()
	require.True(t, executeCalled, "ddx try must invoke the implementation dispatch; output=%q err=%v", out, err)
	assert.Equal(t, "default", lastReq.Policy, "dispatch should request the ordinary no-requirement Fizeau policy by metadata")
	assert.Equal(t, 0, lastReq.MinPower, "initial zero-config dispatch must not duplicate the selected policy floor as MinPower")
	assert.Empty(t, lastReq.Harness, "zero-config routing must not hard-pin a harness")
	assert.Empty(t, lastReq.Provider, "zero-config routing must not hard-pin a provider")
	assert.Empty(t, lastReq.Model, "zero-config routing must not hard-pin a model")
}

func TestTryZeroConfigCheapHintSkipsRequirementProfile(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-zero-config-try-powerClass-cheap",
		Title: "Try with explicit cheap routing powerClass",
		Extra: map[string]any{policyescalation.BeadEstimatedDifficultyKey: string(policyescalation.DifficultyEasy)},
	}))

	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"try",
		"ddx-zero-config-try-powerClass-cheap",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()
	require.True(t, executeCalled, "ddx try must invoke the implementation dispatch; output=%q err=%v", out, err)
	assert.Equal(t, "cheap", lastReq.Policy, "cheap zero-config routing must not pick requirement-bearing air-gapped policy")
	assert.Equal(t, 0, lastReq.MinPower, "initial zero-config dispatch must not duplicate the selected policy floor as MinPower")
}

func TestTryIgnoresNumericBeadPowerHint(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "ddx-zero-config-ignore-numeric-hint",
		Title: "Try ignores numeric bead retry hint",
		Extra: map[string]any{
			agent.TriagePowerHintKey: 90,
		},
	}))

	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"try",
		"ddx-zero-config-ignore-numeric-hint",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	stub.mu.Lock()
	executeCalled := stub.executeCalled
	lastReq := stub.lastReq
	stub.mu.Unlock()
	require.True(t, executeCalled, "ddx try must invoke implementation dispatch; output=%q err=%v", out, err)
	assert.Equal(t, "default", lastReq.Policy, "numeric retry metadata must not override default zero-config policy selection")
	assert.Equal(t, 0, lastReq.MinPower, "numeric retry metadata must not become the requested MinPower")
}

// TestTryInterrupt_InFlightAttemptUnclaimsTarget verifies that cancelling
// `ddx try <id>` during an in-flight attempt releases the claim and leaves the
// bead open so it can be reclaimed.
func TestTryInterrupt_InFlightAttemptUnclaimsTarget(t *testing.T) {
	store, out, err := runInterruptedTryCommand(t)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(out+err.Error()), "context canceled", "try should surface the interrupted outcome")
	assert.Contains(t, out, "context canceled", "interrupted try should surface the interrupted outcome")

	got, storeErr := store.Get("try-interrupt-001")
	require.NoError(t, storeErr)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	fresh, found, hbErr := store.ClaimHeartbeatFresh("try-interrupt-001")
	require.NoError(t, hbErr)
	assert.False(t, found, "interrupted try must remove the claim heartbeat")
	assert.False(t, fresh)
	require.NoError(t, store.Claim("try-interrupt-001", "worker-b"))
}

// TestTryInterrupt_DoesNotReportOperatorCancel verifies that plain context
// cancellation is not classified as operator_cancel unless the cancel marker
// is explicitly present.
func TestTryInterrupt_DoesNotReportOperatorCancel(t *testing.T) {
	store, out, err := runInterruptedTryCommand(t)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(out+err.Error()), "context canceled", "try should surface the interrupted outcome")
	assert.NotContains(t, out, agent.OperatorCancelReason, "plain cancellation must not report operator_cancel")

	got, storeErr := store.Get("try-interrupt-001")
	require.NoError(t, storeErr)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

// TestTry_FlagsPlumbThrough verifies that routing flags (--harness, --model)
// reach the executor. We capture the resolved config in the stub executor.
func TestTry_FlagsPlumbThrough(t *testing.T) {
	// Isolate from ~/.config/fizeau/config.yaml so the pre-dispatch lint hook
	// does not dispatch to real providers (HTTP or exec-based) during the test.
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "flag-bead-001",
		Title: "Flag plumbing bead",
	}))

	var capturedBeadID string
	stubExecutor := agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		capturedBeadID = beadID
		return agent.ExecuteBeadReport{
			BeadID: beadID,
			Status: agent.ExecuteBeadStatusSuccess,
		}, nil
	})

	factory := NewCommandFactory(env.Dir)
	factory.tryExecutorOverride = stubExecutor
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()

	// The --harness and --model flags are passthrough — they reach the runtime
	// config (CLIOverrides). Since the stub executor bypasses the real agent,
	// we assert the flags are accepted without error (no flag-parsing failure)
	// and the executor is invoked with the correct bead ID.
	out, err := executeCommand(root, "try", "flag-bead-001",
		"--harness=codex", "--model=gpt-5.4-mini", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err, "ddx try with routing flags must not fail on flag parsing: %s", out)
	assert.Equal(t, "flag-bead-001", capturedBeadID,
		"executor must receive the correct bead ID regardless of routing flags")
	// Verify the flags appear in help (AC #7 surface check).
	tryCmd, _, findErr := root.Find([]string{"try"})
	require.NoError(t, findErr)
	assert.NotNil(t, tryCmd.Flags().Lookup("harness"), "try must expose --harness flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("model"), "try must expose --model flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("profile"), "try must expose --profile flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("provider"), "try must expose --provider flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("force-claim"), "try must expose --force-claim flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("reason"), "try must expose --reason flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("no-review"), "try must expose --no-review flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("no-review-i-know-what-im-doing"), "try must expose the break-glass acknowledgement flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("review-harness"), "try must expose --review-harness flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("review-model"), "try must expose --review-model flag")
}

// TestTry_HooksWired verifies that ddx try wires the pre-dispatch lint hook
// into ExecuteBeadLoopRuntime, and that the triage hook is skipped (not invoked)
// when the implementer produces no commits (BaseRev == ResultRev).
func TestTry_HooksWired(t *testing.T) {
	env := NewTestEnvironment(t)
	skillPath := env.Dir + "/.agents/skills/ddx/bead-lifecycle"
	require.NoError(t, os.MkdirAll(skillPath, 0o755))
	require.NoError(t, os.WriteFile(skillPath+"/SKILL.md", []byte("lint + triage"), 0o644))
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, os.WriteFile(env.Dir+"/.ddx/config.yaml", []byte(`version: "1.0"
bead-quality:
  mode: warn-only
  lint:
    block_threshold_score: 1
`), 0o644))
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "hook-bead-001",
		Title: "Hook wiring bead",
	}))

	runner := &tryHookRunnerStub{t: t}
	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = runner
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusNoChanges,
			Detail:    "nothing changed",
			BaseRev:   "feedface",
			ResultRev: "feedface",
			SessionID: "sess-hook",
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"try",
		"hook-bead-001",
	)
	require.Error(t, err)
	assert.Contains(t, out, "bead:", "try command should still render the terminal report")
	// triage is skipped for empty diffs (BaseRev == ResultRev); only lint fires
	assert.Equal(t, []string{"bead-lifecycle-lint"}, runner.promptSource)

	events, err := store.Events("hook-bead-001")
	require.NoError(t, err)
	var lintSeen, skippedSeen bool
	for _, ev := range events {
		switch ev.Kind {
		case "bead-quality.lint":
			lintSeen = true
		case agent.ReviewerSkippedEmptyDiffEventKind:
			skippedSeen = true
		}
	}
	assert.True(t, lintSeen, "try must emit the lint event when the hook is wired")
	assert.True(t, skippedSeen, "try must emit the reviewer-skipped-empty-diff event for empty-diff no_changes")
}

// TestTry_NoReviewRequiresAckFlag verifies the break-glass guardrail: the
// command must reject --no-review unless the explicit acknowledgement flag is
// also present.
func TestTry_NoReviewRequiresAckFlag(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "guardrail-bead-001",
		Title: "Guardrail bead",
	}))

	factory := NewCommandFactory(env.Dir)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		t.Fatalf("executor should not be called when --no-review ack is missing")
		return agent.ExecuteBeadReport{}, nil
	})
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "try", "guardrail-bead-001", "--no-review")
	require.Error(t, err)
	assert.Contains(t, out, "--no-review requires --no-review-i-know-what-im-doing")
}

// TestTry_CommandRegistered verifies AC #7: "ddx try" is wired into the root
// command and "ddx try --help" works.
func TestTry_CommandRegistered(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	tryCmd, _, err := root.Find([]string{"try"})
	require.NoError(t, err, "ddx try must be registered in root command")
	require.NotNil(t, tryCmd, "ddx try must be non-nil")

	out, err := executeCommand(root, "try", "--help")
	// cobra returns ErrHelp for --help; that is not a real error here
	if err != nil && !strings.Contains(err.Error(), "pflag: help requested") {
		require.NoError(t, err, "ddx try --help must not return an error")
	}
	assert.Contains(t, out, "bead-id", "help must mention bead-id")
	assert.Contains(t, out, "--no-review-i-know-what-im-doing", "help must document the break-glass acknowledgement flag")
	assert.Contains(t, out, "review:skip-reason:*", "help must document the sibling rationale requirement")
}

func TestTry_RecoversOrphanedWorktreesBeforeSpawn(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID:    "recover-bead-001",
		Title: "Recover orphaned worktrees",
	}))

	orphan := filepath.Join(env.Dir, ddxroot.DirName, agent.ExecuteBeadWtPrefix+"recover-bead-001-old")
	git := &tryGitOpsStub{
		worktrees: []string{orphan, filepath.Join(env.Dir, "unrelated")},
	}

	factory := NewCommandFactory(env.Dir)
	factory.executeBeadGitOverride = git
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "deadbeef01234567",
		}, nil
	})

	root := factory.NewRootCommand()
	_, err := executeCommand(root, "try", "recover-bead-001", "--no-review", "--no-review-i-know-what-im-doing")
	require.NoError(t, err)
	assert.Contains(t, git.removed, orphan, "try must prune orphaned execute-bead worktrees before spawning a new one")
	assert.True(t, git.pruned, "try must prune stale worktrees after recovery")
}

func TestTry_AppliesGateContextToLanding(t *testing.T) {
	env := NewTestEnvironment(t)
	manifestRel := filepath.Join("bundle", "manifest.json")
	manifestAbs := filepath.Join(env.Dir, manifestRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestAbs), 0o755))
	require.NoError(t, os.WriteFile(manifestAbs, []byte(`{"governing":[{"id":"gate-001"}]}`), 0o644))

	res := &agent.ExecuteBeadResult{
		BeadID:       "gate-bead-001",
		AttemptID:    "attempt-1",
		ResultRev:    "deadbeef01234567",
		ManifestFile: manifestRel,
	}
	git := &tryGitOpsStub{}

	wt, ids, cleanup, err := agent.BuildLandingGateContext(env.Dir, res, git)
	require.NoError(t, err)
	require.NotEmpty(t, wt)
	require.Equal(t, []string{"gate-001"}, ids)
	require.Len(t, git.added, 1)

	cleanup()
	require.Len(t, git.removed, 1)
	assert.Equal(t, wt, git.removed[0])
}

func TestTry_PreservesJSONResultSchema_SameAsExecuteBead(t *testing.T) {
	payload := agent.ExecuteBeadResult{
		BeadID:      "ddx-1",
		AttemptID:   "attempt-1",
		BaseRev:     "base",
		ResultRev:   "result",
		Outcome:     "merged",
		Status:      agent.ExecuteBeadStatusSuccess,
		PreserveRef: "refs/ddx/iterations/ddx-1/attempt-1",
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "ddx-1", got["bead_id"])
	assert.Equal(t, "attempt-1", got["attempt_id"])
	assert.Equal(t, "result", got["result_rev"])
	assert.Equal(t, "merged", got["outcome"])
	assert.Equal(t, "success", got["status"])
	assert.Equal(t, "refs/ddx/iterations/ddx-1/attempt-1", got["preserve_ref"])
}

func TestTry_ExitCodeContract_0_1_2(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		want    int
		wantErr bool
	}{
		{name: "success", status: agent.ExecuteBeadStatusSuccess, want: 0},
		{name: "already_satisfied", status: agent.ExecuteBeadStatusAlreadySatisfied, want: 0},
		{name: "preserved", status: agent.ExecuteBeadStatusPreservedNeedsReview, want: 1, wantErr: true},
		{name: "no_changes", status: agent.ExecuteBeadStatusNoChanges, want: 1, wantErr: true},
		{name: "no_evidence", status: agent.ExecuteBeadStatusNoEvidenceProduced, want: 1, wantErr: true},
		{name: "failed", status: agent.ExecuteBeadStatusExecutionFailed, want: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tryExitCodeForStatus(tt.status)
			if tt.wantErr {
				require.Error(t, err)
				exitErr, ok := err.(*ExitError)
				require.True(t, ok)
				assert.Equal(t, tt.want, exitErr.Code)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTryResult_PrintsLandingCoordinates(t *testing.T) {
	var buf bytes.Buffer
	writeTryResult(&buf, agent.ExecuteBeadReport{
		BeadID:            "ddx-landing-001",
		Status:            agent.ExecuteBeadStatusSuccess,
		Detail:            "merged",
		ResultRev:         "0d70176c11112222333344445555666677778888",
		ImplementationRev: "6f6f6f6f11112222333344445555666677778888",
		LandedRev:         "0d70176c11112222333344445555666677778888",
		TargetBranch:      "ddx/a54e0299-burndown-232516",
		ProjectRoot:       "/tmp/fizeau-a54e0299-rescue.235101",
	})

	out := buf.String()
	assert.Contains(t, out, "implementation_rev: 6f6f6f6f11112222333344445555666677778888")
	assert.Contains(t, out, "landed_rev: 0d70176c11112222333344445555666677778888")
	assert.Contains(t, out, "landed_branch: ddx/a54e0299-burndown-232516")
	assert.Contains(t, out, "project_root: /tmp/fizeau-a54e0299-rescue.235101")
}

func TestTrySideWorktreeLanding_PrintsNoRewriteRescueCommand(t *testing.T) {
	var buf bytes.Buffer
	writeTryResult(&buf, agent.ExecuteBeadReport{
		BeadID:       "ddx-landing-002",
		Status:       agent.ExecuteBeadStatusSuccess,
		ResultRev:    "0d70176c11112222333344445555666677778888",
		TargetBranch: "ddx/a54e0299-burndown-232516",
		ProjectRoot:  "/tmp/fizeau-a54e0299-rescue.235101",
	})

	out := buf.String()
	assert.Contains(t, out, "landing_scope: branch-local")
	assert.Contains(t, out, "rescue_command: git merge --no-ff ddx/a54e0299-burndown-232516")
	assert.Contains(t, out, "dirty_worktree_rescue: git worktree add ../ddx-rescue-ddx-a54e0299-burndown-232516 <target-branch> && cd ../ddx-rescue-ddx-a54e0299-burndown-232516 && git merge --no-ff ddx/a54e0299-burndown-232516")
}
