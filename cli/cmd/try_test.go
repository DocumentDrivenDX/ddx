package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tryHookRunnerStub struct {
	t            *testing.T
	promptSource []string
}

func (r *tryHookRunnerStub) Run(opts agent.RunArgs) (*agent.Result, error) {
	r.promptSource = append(r.promptSource, opts.PromptSource)
	switch opts.PromptSource {
	case "bead-lifecycle-lint":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"score":9,"rationale":"ok","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	case "bead-lifecycle-triage":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"classification":"transport","recommended_action":"retry","rationale":"transient","suggested_amendments":"none","suggested_followup_beads":[]}`,
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
	// Set status directly via Update — the admin escape hatch that bypasses ClosureGate.
	require.NoError(t, store.Update("closed-bead-001", func(b *bead.Bead) {
		b.Status = bead.StatusClosed
	}))

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
	// Transition to cancelled via Update (direct status override for test).
	require.NoError(t, store.Update("cancelled-bead-001", func(b *bead.Bead) {
		b.Status = bead.StatusCancelled
	}))

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

// TestTry_FlagsPlumbThrough verifies that routing flags (--harness, --model)
// reach the executor. We capture the resolved config in the stub executor.
func TestTry_FlagsPlumbThrough(t *testing.T) {
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
	assert.NotNil(t, tryCmd.Flags().Lookup("no-review"), "try must expose --no-review flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("no-review-i-know-what-im-doing"), "try must expose the break-glass acknowledgement flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("review-harness"), "try must expose --review-harness flag")
	assert.NotNil(t, tryCmd.Flags().Lookup("review-model"), "try must expose --review-model flag")
}

// TestTry_HooksWired verifies that ddx try wires the pre-dispatch lint and
// post-attempt triage hooks into ExecuteBeadLoopRuntime, and that both hooks
// route through the shared runner override seam.
func TestTry_HooksWired(t *testing.T) {
	env := NewTestEnvironment(t)
	skillPath := env.Dir + "/.agents/skills/ddx/bead-lifecycle"
	require.NoError(t, os.MkdirAll(skillPath, 0o755))
	require.NoError(t, os.WriteFile(skillPath+"/SKILL.md", []byte("lint + triage"), 0o644))
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init())
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
		"--harness=codex",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.Error(t, err)
	assert.Contains(t, out, "bead:", "try command should still render the terminal report")
	assert.Equal(t, []string{"bead-lifecycle-lint", "bead-lifecycle-triage"}, runner.promptSource)

	events, err := store.Events("hook-bead-001")
	require.NoError(t, err)
	var lintSeen, triageSeen bool
	for _, ev := range events {
		switch ev.Kind {
		case "bead-quality.lint":
			lintSeen = true
		case "bead-quality.triage":
			triageSeen = true
		}
	}
	assert.True(t, lintSeen, "try must emit the lint event when the hook is wired")
	assert.True(t, triageSeen, "try must emit the triage event when the hook is wired")
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
