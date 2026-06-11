package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkCommandHasPassthroughFlags verifies that ddx work exposes the
// harness/provider/model/min-power/max-power passthrough flags that operators
// use to constrain agent routing. These flags are forwarded opaquely; ddx work
// does not validate or branch on their string values.
func TestWorkCommandHasPassthroughFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	require.NotNil(t, workCmd)

	for _, name := range []string{"harness", "provider", "model", "min-power", "max-power", "claim-rate-window", "claim-rate-threshold", "preclaim-warn-threshold"} {
		f := workCmd.Flags().Lookup(name)
		assert.NotNil(t, f, "ddx work must have --%s passthrough flag", name)
	}
	assert.NotNil(t, workCmd.Flags().Lookup("ignore-cooldown"), "ddx work must expose --ignore-cooldown")
	assert.NotNil(t, workCmd.Flags().Lookup("reason"), "ddx work must expose --reason")
	assert.Equal(t, "10", workCmd.Flags().Lookup("claim-rate-window").DefValue, "ddx work must default claim-rate window to 10")
	assert.Equal(t, "0", workCmd.Flags().Lookup("claim-rate-threshold").DefValue, "ddx work must default claim-rate threshold to 0.0")
	assert.Equal(t, "5", workCmd.Flags().Lookup("preclaim-warn-threshold").DefValue, "ddx work must default preclaim warn threshold to 5")
}

func TestParseExecuteLoopFlags_AllFlagsPopulateSpec(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)

	setFlag := func(name, value string) {
		t.Helper()
		require.NoError(t, workCmd.Flags().Set(name, value))
	}
	setFlag("project", dir)
	setFlag("from", "HEAD~1")
	setFlag("harness", "claude")
	setFlag("model", "sonnet")
	setFlag("profile", "smart")
	setFlag("provider", "anthropic")
	setFlag("effort", "high")
	setFlag("ignore-cooldown", "true")
	setFlag("reason", "operator unblock")
	setFlag("watch", "true")
	setFlag("idle-interval", "45s")
	setFlag("json", "true")
	setFlag("local", "true")
	setFlag("no-review", "true")
	setFlag("no-review-i-know-what-im-doing", "true")
	setFlag("review-harness", "codex")
	setFlag("review-model", "gpt-5.4")
	setFlag("max-cost", "12.5")
	setFlag("max-recovery-cost", "2.5")
	setFlag("preclaim-warn-threshold", "7")
	setFlag("preclaim-timeout", "45s")
	setFlag("request-timeout", "2m")
	setFlag("rate-limit-max-wait", "90s")
	setFlag("min-power", "7")
	setFlag("max-power", "8")

	spec, dispatch, err := parseExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, dir, spec.ProjectRoot)
	assert.Equal(t, "HEAD~1", spec.FromRev)
	assert.Equal(t, "claude", spec.Harness)
	assert.Equal(t, "sonnet", spec.Model)
	assert.Equal(t, "smart", spec.Profile)
	assert.Equal(t, "anthropic", spec.Provider)
	assert.Equal(t, "high", spec.Effort)
	assert.True(t, spec.IgnoreCooldown)
	assert.Equal(t, "operator unblock", spec.CooldownOverrideReason)
	assert.Equal(t, executeloop.ModeWatch, spec.Mode)
	assert.Equal(t, 45*time.Second, spec.IdleInterval.Duration)
	assert.True(t, spec.NoReview)
	assert.Equal(t, "codex", spec.ReviewHarness)
	assert.Equal(t, "gpt-5.4", spec.ReviewModel)
	assert.True(t, spec.OpaquePassthrough)
	assert.Equal(t, 12.5, spec.MaxCostUSD)
	assert.Equal(t, 2.5, spec.MaxRecoveryCostUSD)
	assert.Equal(t, 45*time.Second, spec.PreClaimTimeout.Duration)
	assert.Equal(t, 2*time.Minute, spec.RequestTimeout.Duration)
	assert.Equal(t, 90*time.Second, spec.RateLimitMaxWait.Duration)
	assert.Equal(t, 7, spec.MinPower)
	assert.Equal(t, 8, spec.MaxPower)
	assert.Equal(t, executeloop.SpecCurrentVersion, spec.SpecVersion)
	assert.Equal(t, "true", dispatch.JSON)
	assert.True(t, dispatch.Local)
}

func TestWork_IgnoreCooldownDrainsCooledQueue(t *testing.T) {
	env := NewTestEnvironment(t)
	store := bead.NewStore(env.Dir + "/.ddx")
	require.NoError(t, store.Init(context.Background()))

	future := time.Now().UTC().Add(6 * time.Hour)
	for _, tc := range []struct {
		id       string
		priority int
	}{
		{id: "cool-4", priority: 4},
		{id: "cool-1", priority: 1},
		{id: "cool-3", priority: 3},
		{id: "cool-0", priority: 0},
		{id: "cool-2", priority: 2},
	} {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:       tc.id,
			Title:    "Cooled " + tc.id,
			Priority: tc.priority,
		}))
		require.NoError(t, store.SetExecutionCooldown(tc.id, future, agent.ExecuteBeadStatusNoChanges, "retry later", "base-rev-1"))
	}

	var got []string
	factory := NewCommandFactory(env.Dir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		got = append(got, beadID)
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "rev-" + beadID,
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--ignore-cooldown",
		"--reason", "operator unblock",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "ddx work must drain cooled beads when override is explicit: %s", out)
	assert.Equal(t, []string{"cool-0", "cool-1", "cool-2", "cool-3", "cool-4"}, got)
}

// TestWork_NetworkFreeDrain_NoFetchInLoop verifies reliability principle P9:
// the drain loop's pre-claim hook never blocks on network I/O. The fixture
// repo has an origin remote whose URL points at a deleted path; the last
// observed refs/remotes/origin/main remains as a cached tip. A network-free
// drain must claim and execute a ready bead without error even though no
// `git fetch` against origin could succeed.
func TestWork_NetworkFreeDrain_NoFetchInLoop(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	gitEnv := gitpkg.CleanEnv()
	runGitEnv := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	originDir := t.TempDir()
	workDir := t.TempDir()
	runGitEnv(originDir, "init", "--bare", "-b", "main")
	runGitEnv(workDir, "clone", originDir, ".")
	runGitEnv(workDir, "config", "user.email", "test@ddx.test")
	runGitEnv(workDir, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "seed.txt"), []byte("seed\n"), 0o644))
	runGitEnv(workDir, "add", "seed.txt")
	runGitEnv(workDir, "commit", "-m", "chore: initial seed")
	runGitEnv(workDir, "push", "-u", "origin", "main")

	// Break origin: point the remote URL at a deleted path and remove the bare
	// repo. refs/remotes/origin/main survives as the last-observed tip, so a
	// network-free ancestry check still resolves, but any fetch would fail.
	deletedOrigin := filepath.Join(t.TempDir(), "gone")
	runGitEnv(workDir, "remote", "set-url", "origin", "file://"+deletedOrigin)
	require.NoError(t, os.RemoveAll(originDir))

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:        "ddx-network-free-drain",
		Title:     "network-free drain bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})

	var executed []string
	factory := NewCommandFactory(workDir)
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		executed = append(executed, beadID)
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			ResultRev: "rev-" + beadID,
		}, nil
	})

	out, err := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", workDir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "network-free drain must claim and drain without origin reachability: %s", out)
	assert.Equal(t, []string{"ddx-network-free-drain"}, executed, "the ready bead must be claimed and executed")
}

func TestParseExecuteLoopSpec_RateLimitMaxWait(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want time.Duration
	}{
		{name: "default", want: agent.RateLimitRetryDefaultBudget},
		{name: "explicit positive", set: "90s", want: 90 * time.Second},
		{name: "zero uses runtime default", set: "0", want: 0},
		{name: "negative disables retry", set: "-1s", want: -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			root := NewCommandFactory(dir).NewRootCommand()
			workCmd, _, err := root.Find([]string{"work"})
			require.NoError(t, err)
			if tt.set != "" {
				require.NoError(t, workCmd.Flags().Set("rate-limit-max-wait", tt.set))
			}

			spec, _, err := parseExecuteLoopSpec(workCmd, true)
			require.NoError(t, err)
			assert.Equal(t, tt.want, spec.RateLimitMaxWait.Duration)
		})
	}
}

// passThroughRunner accepts any prompt and returns a valid-looking JSON response;
// used to keep work moving past the stale-binary preflight under test without
// fataling on prompt sources that this test doesn't care about.
type passThroughRunner struct{}

func (passThroughRunner) Run(opts agent.RunArgs) (*agent.Result, error) {
	return &agent.Result{
		ExitCode: 0,
		Output:   `{"classification":"ready","rationale":"ok","readiness_checks":[],"score":9,"suggested_fixes":[],"waivers_applied":[]}`,
	}, nil
}

func TestWork_WarnsStaleSourceBinaryButProceeds(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot, buildSHA, _ := seedStaleSourceCheckout(t)
	seedExecuteBead(t, projectRoot, &bead.Bead{
		ID:        "stale-work-bead",
		Title:     "stale source work bead",
		Status:    bead.StatusOpen,
		Priority:  0,
		IssueType: bead.DefaultType,
	})

	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()
	factory := NewCommandFactory(projectRoot)
	factory.Version = "0.9.0"
	factory.Commit = buildSHA
	factory.AgentRunnerOverride = passThroughRunner{}

	out, _ := executeCommand(
		factory.NewRootCommand(),
		"work",
		"--once",
		"--project", projectRoot,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	// The work command must not hard-fail with the stale-binary blocking error.
	assert.NotContains(t, out, "installed ddx binary is stale for this DDx source checkout",
		"work must not emit the hard-fail blocking message; output=%s", out)
	assert.Contains(t, out, "WARNING: installed ddx is built from",
		"stderr must include the stale-binary warning")
}

func TestWorkZeroConfigInferredTaskSelectsFizeauPolicyWithoutInitialMinPower(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        "ddx-zero-config-work-powerClass-standard",
		Title:     "Work with inferred standard routing powerClass",
		IssueType: "bug",
	}))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"work",
		"--once",
		"--project", dir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "ddx work must invoke implementation dispatch; output=%q err=%v", out, err)
	lastReq := requests[0]
	assert.Equal(t, "default", lastReq.Policy, "work should request the ordinary no-requirement Fizeau policy by metadata")
	assert.Equal(t, 0, lastReq.MinPower, "initial zero-config dispatch must not duplicate the selected policy floor as MinPower")
	assert.Empty(t, lastReq.Harness, "zero-config work must not hard-pin a harness")
	assert.Empty(t, lastReq.Provider, "zero-config work must not hard-pin a provider")
	assert.Empty(t, lastReq.Model, "zero-config work must not hard-pin a model")
}

func TestWorkZeroConfigStandardPolicyDoesNotDowngradeToCheapPolicy(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies = []agentlib.PolicyInfo{
		{Name: "cheap", MinPower: 5, MaxPower: 5},
		{Name: "default", MinPower: 7, MaxPower: 8},
		{Name: "smart", MinPower: 9, MaxPower: 10},
	}
	stub.listModels = []agentlib.ModelInfo{
		{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
		{ID: "default-offline", Power: 7, Available: false, AutoRoutable: true},
		{ID: "smart-model", Power: 9, Available: true, AutoRoutable: true},
	}
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:          "ddx-zero-config-work-standard-no-cheap",
		Title:       "Work with standard powerClass and stale medium model snapshot",
		IssueType:   "bug",
		Description: "bug work should infer standard powerClass",
	}))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"work",
		"--once",
		"--project", dir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	requests := capturedImplementationRequests(stub)
	require.NotEmpty(t, requests, "ddx work must invoke implementation dispatch; output=%q err=%v", out, err)
	lastReq := requests[0]
	assert.Equal(t, "default", lastReq.Policy, "standard-powerClass work must not downgrade to the weak policy when the model snapshot is stale")
	assert.Equal(t, 0, lastReq.MinPower, "initial zero-config dispatch must keep power as Fizeau policy metadata, not DDx hardcoded floor")
}

// TestProjectHasRoutingConfig_EndpointsAreTransportNotRoutingPin covers
// ddx-e0b95b4a. agent.endpoints declares where providers live (transport
// config) — it does NOT pin a routing decision the way agent.model does.
// Treating endpoints as a routing pin disables zero-config powerClass inference
// (autoInferPowerClass in runAgentExecuteLoopImpl), so no-flag `ddx work` sends an
// empty Policy and Fizeau resolves it via its default policy. In production
// at /home/erik/Projects/ddx this scored Opus on ordinary implementation work
// (see .ddx/attachments/ddx-c3219628/events.jsonl: actual_model=opus,
// cost_usd=15.0776 on a worker-status bug; ddx-6cde5ffd: $6.83 on a registry
// package task).
//
// The matrix verifies the new contract: only an explicit model pin in
// agent.config.yaml suppresses zero-config powerClass inference; endpoints, an
// empty agent block, or no config at all leave inference active so the
// implementation profile selector can request a Fizeau policy by metadata.
func TestProjectHasRoutingConfig_EndpointsAreTransportNotRoutingPin(t *testing.T) {
	cases := []struct {
		name     string
		writeCfg bool
		yaml     string
		want     bool
	}{
		{
			name:     "no .ddx/config.yaml",
			writeCfg: false,
			want:     false,
		},
		{
			name:     "library-only config (zero-config baseline)",
			writeCfg: true,
			yaml: `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`,
			want: false,
		},
		{
			name:     "endpoints only, no model pin (production scenario)",
			writeCfg: true,
			yaml: `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  endpoints:
    - type: lmstudio
      host: 127.0.0.1
      port: 1234
    - type: omlx
      host: 127.0.0.1
      port: 1235
`,
			want: false,
		},
		{
			name:     "model pin counts as routing config",
			writeCfg: true,
			yaml: `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  model: pinned-model
`,
			want: true,
		},
		{
			name:     "model pin coexists with endpoints (still routing config)",
			writeCfg: true,
			yaml: `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  model: pinned-model
  endpoints:
    - type: lmstudio
      host: 127.0.0.1
      port: 1234
`,
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ddxDir := filepath.Join(dir, ddxroot.DirName)
			require.NoError(t, os.MkdirAll(ddxDir, 0o755))
			if tc.writeCfg {
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(tc.yaml), 0o644))
			}
			got := projectHasRoutingConfig(dir)
			assert.Equalf(t, tc.want, got,
				"projectHasRoutingConfig(%s) = %v, want %v — endpoints alone must not gate zero-config powerClass inference",
				tc.name, got, tc.want)
		})
	}
}

func TestWorkZeroConfigSemanticRetryUsesNextViablePowerFloor(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()
	implementerCalls := 0
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Role != "implementer" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
			close(ch)
			return ch, nil
		}
		implementerCalls++
		if implementerCalls == 1 {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"build failed","routing_actual":{"power":5}}`)}
		} else {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
		}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        "ddx-zero-config-work-powerClass-retry",
		Title:     "Work retries semantic failures at the next viable MinPower",
		IssueType: "bug",
	}))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"work",
		"--once",
		"--project", dir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	requests := capturedImplementationRequests(stub)
	require.Len(t, requests, 2, "ddx work should retry an escalatable implementation failure; output=%q err=%v", out, err)
	assert.Equal(t, "default", requests[0].Policy, "first attempt should use the inferred no-requirement default policy")
	assert.Equal(t, 0, requests[0].MinPower, "first attempt must not send an initial MinPower floor")
	assert.Equal(t, "default", requests[1].Policy, "retry should stay within the selected policy band")
	assert.Equal(t, 7, requests[1].MinPower, "semantic retry should use the next viable model-power floor")
}

func TestWorkZeroConfigProviderConnectivityRetryAddsExactMinPowerFloor(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()
	implementerCalls := 0
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Role != "implementer" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
			close(ch)
			return ch, nil
		}
		implementerCalls++
		if implementerCalls == 1 {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"provider request failed: dial tcp 100.70.199.113:1235: connect: connection refused","routing_actual":{"provider":"vidar","model":"qwen-local","power":5}}`)}
		} else {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"ok"}`)}
		}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        "ddx-zero-config-work-provider-connectivity-retry",
		Title:     "Work routes around failed provider connectivity",
		IssueType: "bug",
	}))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"work",
		"--once",
		"--project", dir,
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	requests := capturedImplementationRequests(stub)
	require.Len(t, requests, 2, "ddx work should retry provider connectivity with a higher floor; output=%q err=%v", out, err)
	assert.Equal(t, "default", requests[0].Policy, "first attempt should use the inferred no-requirement default policy")
	assert.Equal(t, 0, requests[0].MinPower, "first attempt must not send an initial MinPower floor")
	assert.Equal(t, requests[0].Policy, requests[1].Policy, "retry should preserve the selected policy intent")
	assert.Equal(t, 6, requests[1].MinPower, "provider connectivity retry should ask Fizeau for a route above the failed power")
}

// TestWorkProviderConnectivityCommitsEvidence is the ddx-ca94d157 regression
// test. A ddx work attempt that terminally fails with provider_connectivity —
// the routed local endpoint is unreachable before any agent output — published
// its .ddx/executions/<attempt>/ evidence bundle (manifest.json, prompt.md,
// result.json) into the project root but the CLI execute/work finalizer never
// committed it, leaving the worktree dirty after the tracker commit had already
// landed. The finalizer must now leave the worktree clean (mirroring the server
// worker path) while still recording route-failure evidence on the bead.
//
// The route is pinned (--provider/--model, harness left empty so dispatch
// still flows through the stubbed Fizeau service) so the escalation ladder is
// disabled: exactly one attempt runs, it fails connectivity, and ModeOnce
// returns — the terminal failure path the bead describes.
func TestWorkProviderConnectivityCommitsEvidence(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	stub := installExecuteCapturingStub(t)
	stub.listPolicies, stub.listModels = canonicalFizeauPolicyFixture()
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		ch := make(chan agentlib.ServiceEvent, 1)
		if req.Role != "implementer" {
			ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"success","final_text":"{\"classification\":\"ready\",\"rationale\":\"ok\",\"readiness_checks\":[],\"score\":9,\"suggested_fixes\":[],\"waivers_applied\":[],\"recommended_action\":\"release_claim_retry\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`)}
			close(ch)
			return ch, nil
		}
		// The implementer dispatch fails at the routed provider endpoint before
		// any agent output — the terminal provider_connectivity path (the exact
		// niflheim failure: dial tcp ... i/o timeout).
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(`{"status":"error","exit_code":1,"error":"openai: Post \"http://grendel:1235/v1/chat/completions\": dial tcp 100.97.179.68:1235: i/o timeout","routing_actual":{"provider":"grendel-omlx","model":"qwen-local","power":5}}`)}
		close(ch)
		return ch, nil
	}

	dir := minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run())
	// Use the production .gitignore so the policy-excluded embedded session logs
	// (.ddx/executions/*/embedded/) are ignored just as they are in a real
	// `ddx init` project — the finalizer is only responsible for the tracked
	// evidence files (manifest/prompt/result), not the runtime-only logs.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte(strings.Join(initGitignoreRules, "\n")+"\n"), 0o644))
	require.NoError(t, exec.Command("git", "-C", dir, "add", ".").Run())
	require.NoError(t, exec.Command("git", "-C", dir, "commit", "-m", "init").Run())
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	beadID := "ddx-provider-connectivity-evidence"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        beadID,
		Title:     "Work commits evidence for terminal provider connectivity failures",
		IssueType: "bug",
	}))

	factory := NewCommandFactory(dir)
	root := factory.NewRootCommand()
	out, err := executeCommand(
		root,
		"work",
		"--once",
		"--project", dir,
		"--provider", "grendel-omlx",
		"--model", "qwen-local",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)
	require.NoError(t, err, "output=%q", out)

	// AC #2: the worktree is clean after the terminal failure path — the
	// published evidence bundle is committed, not left untracked. The only
	// tolerated leftover is .ddx/metrics/locks.jsonl: per-machine lock-latency
	// diagnostics rewritten on every `ddx work` run, runtime-only state outside
	// this bead's evidence contract.
	statusOut, statusErr := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	require.NoError(t, statusErr, string(statusOut))
	var dirty []string
	for _, line := range strings.Split(strings.TrimSpace(string(statusOut)), "\n") {
		if strings.TrimSpace(line) == "" || strings.Contains(line, ".ddx/metrics/") {
			continue
		}
		dirty = append(dirty, line)
	}
	require.Empty(t, dirty,
		"worktree must be clean after a provider_connectivity attempt; output=%q\nstatus:\n%s", out, statusOut)

	// AC #1: any execution-evidence bundle that survived to the project root is
	// committed (tracked), never left untracked. A clean worktree (above) plus
	// a present-and-tracked or absent bundle satisfies the contract.
	logOut, logErr := exec.Command("git", "-C", dir, "log", "--all", "--name-only", "--pretty=format:").CombinedOutput()
	require.NoError(t, logErr, string(logOut))
	execDir := filepath.Join(dir, ddxroot.DirName, "executions")
	if entries, statErr := os.ReadDir(execDir); statErr == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			for _, name := range []string{"manifest.json", "prompt.md", "result.json"} {
				if _, ferr := os.Stat(filepath.Join(execDir, e.Name(), name)); ferr != nil {
					continue
				}
				tracked := filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", e.Name(), name))
				require.Contains(t, string(logOut), tracked,
					"evidence file %s present on disk must be committed", tracked)
			}
		}
	}

	// AC #3: route-failure evidence is recorded on the bead with provider,
	// model, reason, and error detail.
	events, evErr := store.Events(beadID)
	require.NoError(t, evErr)
	var routeFailure *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "route-failure" {
			routeFailure = &events[i]
		}
	}
	require.NotNil(t, routeFailure, "expected a kind=route-failure event recorded on the bead")
	require.Contains(t, routeFailure.Body, "grendel-omlx", "route-failure event must record the failed provider")
	require.Contains(t, routeFailure.Body, "provider_connectivity", "route-failure event must record the outcome reason")
	require.Contains(t, routeFailure.Body, "i/o timeout", "route-failure event must record the transport error detail")
}

func TestRunAgentExecuteLoopImpl_PassesRateLimitMaxWait(t *testing.T) {
	var out bytes.Buffer
	budget := 42 * time.Second

	runtime := executeLoopAttemptRuntime(
		executeloop.ExecuteLoopSpec{
			FromRev:          "HEAD~1",
			RateLimitMaxWait: executeloop.Duration{Duration: budget},
		},
		&out,
		nil,
		nil,
		nil,
	)

	assert.Equal(t, "HEAD~1", runtime.FromRev)
	assert.Same(t, &out, runtime.Output)
	assert.Equal(t, budget, runtime.RateLimitMaxWait)
}

func TestParseExecuteLoopFlags_OnceFlag(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("once", "true"))

	spec, _, err := parseExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, executeloop.ModeOnce, spec.Mode)
	assert.Zero(t, spec.IdleInterval.Duration)
}

func TestParseExecuteLoopFlags_OnceAndWatchAreMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("once", "true"))
	require.NoError(t, workCmd.Flags().Set("watch", "true"))

	_, _, err = parseExecuteLoopSpec(workCmd, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--once and --watch are mutually exclusive")
}

func TestParseExecuteLoopFlags_IdleIntervalRequiresWatch(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("idle-interval", "15s"))

	_, _, err = parseExecuteLoopSpec(workCmd, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--idle-interval only applies in watch mode")
}

// TestLegacyAgentNamespaceRemoved verifies that the old nested command name is
// no longer part of the public CLI surface.
func TestLegacyAgentNamespaceRemoved(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(root, "agent", "work")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestWork_PreservesJSONResultSchema_SameAsExecuteLoop(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  2,
		Successes: 1,
		Failures:  1,
		Results: []agent.ExecuteBeadReport{
			{BeadID: "ddx-1", Status: agent.ExecuteBeadStatusSuccess},
			{BeadID: "ddx-2", Status: agent.ExecuteBeadStatusExecutionFailed},
		},
	}, true)
	require.NoError(t, err)

	var payload struct {
		ProjectRoot string `json:"project_root"`
		*agent.ExecuteBeadLoopResult
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	assert.Equal(t, "/tmp/project", payload.ProjectRoot)
	require.NotNil(t, payload.ExecuteBeadLoopResult)
	assert.Equal(t, 2, payload.Attempts)
	assert.Len(t, payload.Results, 2)
}

// TestWorkHelpDocumentsReviewGuardrails verifies that the work help text
// documents the break-glass review override and the review:skip rationale
// requirement so operators can discover both guardrails from the CLI.
func TestWorkHelpDocumentsReviewGuardrails(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	out, err := executeCommand(root, "work", "--help")
	require.NoError(t, err)

	assert.Contains(t, out, "--no-review-i-know-what-im-doing")
	assert.Contains(t, out, "review:skip-reason:*")
	assert.Contains(t, out, "break-glass")
}

// TestWorkPassthroughNotValidated is the primary AC test: ddx work must NOT
// call ValidateForExecuteLoopViaService. When an unknown harness is provided
// and no ready beads exist, work must succeed with no_ready_work rather than
// failing on harness validation. This proves harness/provider/model are opaque
// passthrough — ddx work does not validate or branch on their string values.
func TestWorkPassthroughNotValidated(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	// "bogus_harness_xyz_notreal" does not exist in any harness registry.
	// ValidateForExecuteLoopViaService would fail with "unknown harness: ..."
	// if called. ddx work must skip that call and proceed to the queue scan,
	// finding no ready beads and returning successfully.
	out, err := executeCommand(root, "work",
		"--json", "--once",
		"--harness=bogus_harness_xyz_notreal",
	)
	require.NoError(t, err,
		"ddx work must not fail on unknown harness when there are no ready beads; "+
			"harness is opaque passthrough, not validated by ddx work")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork,
		"ddx work with no ready beads must report no_ready_work=true")
}

func TestWorkDefaultDrainModeExitsOnEmptyQueue(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()
	// Use a generous safety-net timeout so the race detector's overhead
	// under parallel test runs does not fire this cancel before the command
	// can respond. The actual performance assertion is the elapsed < 5s
	// check below, which catches the bug (ddx work hanging on empty queue).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root.SetContext(ctx)

	start := time.Now()
	out, err := executeCommand(root, "work", "--json")
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Less(t, elapsed, 5*time.Second,
		"ddx work must not wait for the long-running worker poll interval by default")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
		Attempts    int  `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.Equal(t, 0, res.Attempts)
}

func TestWorkWatchModeFlagsArePresent(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	watch := workCmd.Flags().Lookup("watch")
	require.NotNil(t, watch)
	assert.Contains(t, watch.Usage, "Keep watching")
	idle := workCmd.Flags().Lookup("idle-interval")
	require.NotNil(t, idle)
	assert.Equal(t, "30s", idle.DefValue)
	assert.Contains(t, idle.Usage, "watch mode")

	// Verify claim-success-rate monitoring flags are present and correctly defaulted
	windowFlag := workCmd.Flags().Lookup("claim-rate-window")
	require.NotNil(t, windowFlag, "ddx work must expose --claim-rate-window")
	assert.Equal(t, "10", windowFlag.DefValue, "claim-rate-window must default to 10")

	thresholdFlag := workCmd.Flags().Lookup("claim-rate-threshold")
	require.NotNil(t, thresholdFlag, "ddx work must expose --claim-rate-threshold")
	assert.Equal(t, "0", thresholdFlag.DefValue, "claim-rate-threshold must default to 0.0 (warn on 100% failure)")
}

func TestWorkPollIntervalRemoved(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	assert.Nil(t, workCmd.Flags().Lookup("poll-interval"))

	_, err = executeCommand(root, "work", "--poll-interval=30s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --poll-interval")
}

func TestWorkHelpDocumentsWatchMode(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	out, err := executeCommand(root, "work", "--help")
	require.NoError(t, err)

	assert.Contains(t, out, "ddx work")
	assert.Contains(t, out, "ddx work --once")
	assert.Contains(t, out, "ddx work --watch")
	assert.Contains(t, out, "ddx work --watch --idle-interval 15s")
	assert.Contains(t, out, "--watch")
	assert.Contains(t, out, "--idle-interval")
	assert.NotContains(t, out, "--poll-interval")
}

func TestWorkDefaultOutput_DoesNotPrintBareRouteSummaryAfterScopedRoutes(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  1,
		Successes: 1,
		Failures:  0,
		Results: []agent.ExecuteBeadReport{{
			Harness:                     "agent",
			Provider:                    "openai",
			Model:                       "gpt-5.2",
			ActualPower:                 78,
			PredictedPower:              82,
			PredictedSpeedTPS:           35.5,
			PredictedCostUSDPer1kTokens: 0.012345,
			PredictedCostSource:         "catalog",
		}},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.NotContains(t, got, "\nroute:")
	assert.NotContains(t, got, "route: harness=agent")
	assert.Contains(t, got, "attempts: 1")
}

func TestWorkDefaultOutput_PrintsCompactTerminalSummary(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:      2,
		Successes:     2,
		Failures:      0,
		StopCondition: "Drained",
		ExitReason:    "drained",
		Results: []agent.ExecuteBeadReport{
			{BeadID: "ddx-changed", Status: agent.ExecuteBeadStatusSuccess},
			{BeadID: "ddx-satisfied", Status: agent.ExecuteBeadStatusAlreadySatisfied},
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "worker exited: drained current execution-ready queue")
	assert.Contains(t, got, "attempts: 2  |  closed: 2  |  changed: 1  |  already-satisfied: 1  |  preserved: 0  |  failures: 0")
	assert.NotContains(t, got, "completed:")
}

func TestWorkDefaultOutput_PreservedNeedsReviewIsNotFailed(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:      1,
		Successes:     0,
		Failures:      0,
		StopCondition: "Drained",
		ExitReason:    "drained",
		Results: []agent.ExecuteBeadReport{
			{
				BeadID:      "ddx-preserved",
				Status:      agent.ExecuteBeadStatusPreservedNeedsReview,
				Detail:      "large-deletion gate: gateway.rs deleted 250 lines (threshold 200)",
				PreserveRef: "refs/ddx/iterations/ddx-preserved/attempt-1",
			},
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "attempts: 1  |  closed: 0  |  changed: 0  |  already-satisfied: 0  |  preserved: 1  |  failures: 0")
	assert.Contains(t, got, "\npreserved:\n")
	assert.Contains(t, got, "ddx-preserved: large-deletion gate: gateway.rs deleted 250 lines (threshold 200) (refs/ddx/iterations/ddx-preserved/attempt-1)")
	assert.NotContains(t, got, "\nfailed:\n")
}

func TestWorkDirtyTreeCheckpointReturnsOperatorAttention(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		StopCondition: "OperatorAttention",
		ExitReason:    "operator_attention",
		OperatorAttention: &agent.OperatorAttentionStop{
			Reason:      "checkpoint_dirty",
			BeadID:      "ddx-0001",
			ProjectRoot: "/tmp/project",
			DirtyPaths:  []string{"cli/cmd/work.go", "cli/internal/agent/execute_bead_loop.go"},
			Message:     "commit or clean the listed implementation files before restarting ddx work: cli/cmd/work.go, cli/internal/agent/execute_bead_loop.go",
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "worker exited: operator attention required")
	assert.Contains(t, got, "released bead: ddx-0001")
	assert.Contains(t, got, "commit or clean the listed implementation files before restarting ddx work")
	assert.Contains(t, got, "project root: /tmp/project")
	assert.Contains(t, got, "dirty paths: cli/cmd/work.go, cli/internal/agent/execute_bead_loop.go")
}

func TestWorkDefaultOutput_PrintsQueueStateAndHumanBlockers(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:    1,
		Successes:   1,
		NoReadyWork: true,
		ExitReason:  "drained",
		Results: []agent.ExecuteBeadReport{{
			BeadID: "ddx-done",
			Status: agent.ExecuteBeadStatusSuccess,
		}},
		QueueSnapshot: &agent.QueueSnapshot{
			ExecutionReadyCount:            0,
			BlockedCount:                   30,
			ProposedOperatorAttentionCount: 3,
			HumanReviewBlockerCount:        3,
			HumanReviewBlockedTotal:        30,
			RetryCooldownCount:             2,
			NextRetryAfter:                 "2026-05-09T12:00:00Z",
			ExecutionIneligibleCount:       1,
			SupersededCount:                1,
			SkippedEpicsCount:              1,
			EpicClosureCandidatesCount:     1,
			HumanReviewBlockers: []agent.HumanReviewBlockerSnapshot{
				{ID: "ddx-human-1", Title: "Needs human 1", DownstreamBlockedCount: 10},
				{ID: "ddx-human-2", Title: "Needs human 2", DownstreamBlockedCount: 10},
				{ID: "ddx-human-3", Title: "Needs human 3", DownstreamBlockedCount: 10},
			},
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "remaining queue: execution-ready=0 blocked=30 operator-attention=3 needs-human/investigation=3 cooldown/deferred=2 next-retry=2026-05-09T12:00:00Z execution-ineligible=1 superseded=1 epics=1 epic-closure-candidates=1")
	assert.Contains(t, got, "30 beads blocked behind 3 needs-human blockers:")
	assert.Contains(t, got, "1. ddx-human-1 Needs human 1 (10 downstream)")
	assert.Contains(t, got, "2. ddx-human-2 Needs human 2 (10 downstream)")
	assert.Contains(t, got, "3. ddx-human-3 Needs human 3 (10 downstream)")
}

func TestWorkStopSummary_ProposedVsDependencyWaiting(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		NoReadyWork: true,
		NoReadyWorkDetail: agent.NoReadyWorkBreakdown{
			ProposedOperatorAttention: []string{"ddx-proposed"},
			DependencyWaiting:         []string{"ddx-waiting"},
			ExternalBlocked:           []string{"ddx-external"},
			RetryCooldown:             []string{"ddx-cooldown"},
			NotEligible:               []string{"ddx-ineligible"},
			Superseded:                []string{"ddx-superseded"},
			Epics:                     []string{"ddx-epic"},
			NextRetryAfter:            "2026-05-09T12:00:00Z",
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "operator attention: 1 proposed bead(s) stop autonomous work and may block downstream dependents: ddx-proposed")
	assert.Contains(t, got, "waiting on dependencies: 1 open bead(s): ddx-waiting")
	assert.Contains(t, got, "external blocked: 1 bead(s) with explicit blocked status: ddx-external")
	assert.Contains(t, got, "retry cooldown: 1 bead(s) (next retry-after: 2026-05-09T12:00:00Z): ddx-cooldown")
	assert.Contains(t, got, "not execution eligible: 1 bead(s): ddx-ineligible")
	assert.Contains(t, got, "superseded: 1 bead(s): ddx-superseded")
	assert.Contains(t, got, "epic containers: 1 ready epic(s) with open children")
	assert.NotContains(t, got, "needs investigation")
	assert.NotContains(t, got, "blocked/proposed")
}

func TestWorkStopSummary_CompletedThenDrainedIncludesNoReadyDetail(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:    1,
		Successes:   1,
		NoReadyWork: true,
		NoReadyWorkDetail: agent.NoReadyWorkBreakdown{
			ProposedOperatorAttention: []string{"ddx-proposed"},
			DependencyWaiting:         []string{"ddx-waiting"},
		},
		Results: []agent.ExecuteBeadReport{{
			BeadID: "ddx-done",
			Status: agent.ExecuteBeadStatusSuccess,
			Detail: "merged cleanly",
		}},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "attempts: 1  |  closed: 1  |  changed: 1  |  already-satisfied: 0  |  preserved: 0  |  failures: 0")
	assert.Contains(t, got, "No execution-ready beads.")
	assert.Contains(t, got, "operator attention: 1 proposed bead(s) stop autonomous work")
	assert.Contains(t, got, "waiting on dependencies: 1 open bead(s): ddx-waiting")
}

func TestWorkStopSummary_DrainedEmptyQueue(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		NoReadyWork: true,
	}, false)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "queue drained: no open work remains in lifecycle queues.")
}

func TestWorkJSONOutput_IncludesRouteEconomicsWithoutHumanLines(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  1,
		Successes: 1,
		Failures:  0,
		Results: []agent.ExecuteBeadReport{{
			Harness:                     "agent",
			Provider:                    "openai",
			Model:                       "gpt-5.2",
			ActualPower:                 78,
			PredictedPower:              82,
			PredictedSpeedTPS:           35.5,
			PredictedCostUSDPer1kTokens: 0.012345,
			PredictedCostSource:         "catalog",
		}},
	}, true)
	require.NoError(t, err)

	jsonStart := strings.Index(out.String(), "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	assert.NotContains(t, out.String()[:jsonStart], "route: ")

	var res struct {
		Attempts int `json:"attempts"`
		Results  []struct {
			Harness                     string  `json:"harness"`
			Provider                    string  `json:"provider"`
			Model                       string  `json:"model"`
			ActualPower                 int     `json:"actual_power"`
			PredictedPower              int     `json:"predicted_power"`
			PredictedSpeedTPS           float64 `json:"predicted_speed_tps"`
			PredictedCostUSDPer1kTokens float64 `json:"predicted_cost_usd_per_1k_tokens"`
			PredictedCostSource         string  `json:"predicted_cost_source"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()[jsonStart:]), &res))
	require.Equal(t, 1, res.Attempts)
	require.Len(t, res.Results, 1)
	assert.Equal(t, "agent", res.Results[0].Harness)
	assert.Equal(t, "openai", res.Results[0].Provider)
	assert.Equal(t, "gpt-5.2", res.Results[0].Model)
	assert.Equal(t, 78, res.Results[0].ActualPower)
	assert.Equal(t, 82, res.Results[0].PredictedPower)
	assert.Equal(t, 35.5, res.Results[0].PredictedSpeedTPS)
	assert.Equal(t, 0.012345, res.Results[0].PredictedCostUSDPer1kTokens)
	assert.Equal(t, "catalog", res.Results[0].PredictedCostSource)
}
