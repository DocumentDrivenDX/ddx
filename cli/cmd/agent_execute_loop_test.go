package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureWorkLifecyclePassingStub(stub *executeCapturingStub) {
	stub.executeFn = func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
		body := `{"status":"success","final_text":"ok"}`
		switch {
		case strings.Contains(req.Prompt, "MODE: intake"):
			body = `{"status":"success","final_text":"{\"classification\":\"atomic\",\"confidence\":0.99,\"reasoning\":\"single-slice\"}"}`
		case strings.Contains(req.Prompt, "MODE: lint"):
			body = `{"status":"success","final_text":"{\"score\":9,\"rationale\":\"ok\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`
		case strings.Contains(req.Prompt, "MODE: triage"):
			body = `{"status":"success","final_text":"{\"classification\":\"already_satisfied\",\"recommended_action\":\"close_already_satisfied\",\"rationale\":\"ok\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`
		}
		ch := make(chan agentlib.ServiceEvent, 1)
		ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(body)}
		close(ch)
		return ch, nil
	}
}

func TestWorkUsesProjectRootForNoWorkScan(t *testing.T) {
	env := NewTestEnvironment(t)
	subdir := filepath.Join(env.Dir, "nested", "path")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	factory := NewCommandFactory(subdir)
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "work", "--json")
	require.NoError(t, err)

	var res struct {
		ProjectRoot string `json:"project_root"`
		NoReadyWork bool   `json:"no_ready_work"`
		Attempts    int    `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.Equal(t, env.Dir, res.ProjectRoot)
	assert.True(t, res.NoReadyWork)
	assert.Equal(t, 0, res.Attempts)
}

func TestInvokeExecuteBeadFromLoopParsesJSONAmidWarnings(t *testing.T) {
	workDir := t.TempDir()
	// Init git repo so HEAD can be resolved
	out, err := exec.Command("git", "init", workDir).CombinedOutput()
	require.NoError(t, err, string(out))
	// Create an initial commit so HEAD exists
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test"), 0o644))
	out, err = exec.Command("git", "-C", workDir, "add", "-A").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", workDir, "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))

	seedExecuteBead(t, workDir, &bead.Bead{
		ID:     "my-bead",
		Title:  "Test bead",
		Status: bead.StatusOpen,
	})
	// Commit the beads.jsonl so it's in the worktree snapshot
	out, err = exec.Command("git", "-C", workDir, "add", ".ddx/beads.jsonl").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", workDir, "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "add beads").CombinedOutput()
	require.NoError(t, err, string(out))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "aaaa1111",
		wtHeadRev:   "aaaa1111",
		dirty:       true,
	}
	runner := &fakeAgentRunner{result: &agent.Result{ExitCode: 0, Harness: "mock"}}

	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	res, err := agent.ExecuteBeadWithConfig(context.Background(), workDir, "my-bead", rcfg, agent.ExecuteBeadRuntime{AgentRunner: runner}, git)
	require.NoError(t, err)
	assert.Equal(t, "my-bead", res.BeadID)
	assert.Equal(t, agent.ExecuteBeadStatusNoEvidenceProduced, res.Status)
}

func TestExecuteLoopZeroConfigInferredTierSetsInitialMinPower(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	configureWorkLifecyclePassingStub(stub)
	stub.listModels = []agentlib.ModelInfo{
		{ID: "cheap-model", Power: 30, Available: true, AutoRoutable: true},
		{ID: "standard-model", Power: 70, Available: true, AutoRoutable: true},
		{ID: "smart-model", Power: 90, Available: true, AutoRoutable: true},
	}

	dir := setupWorkIntakeFixture(t)
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Update("ddx-intake-test", func(b *bead.Bead) {
		b.IssueType = "bug"
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	out, _ := executeCommand(root, "work",
		"--once",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	stub.mu.Lock()
	executionSeen := stub.executionSeen
	executionReq := stub.executionReq
	stub.mu.Unlock()

	require.True(t, executionSeen, "ddx work must run implementer for zero-config inferred tier test (output=%q)", out)
	assert.Equal(t, 70, executionReq.MinPower)
}

func TestExecuteLoopNoRoutingFlagsCheapTierMayRemainUnconstrained(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	stub := installExecuteCapturingStub(t)
	configureWorkLifecyclePassingStub(stub)
	stub.listModels = []agentlib.ModelInfo{
		{ID: "cheap-model", Power: 30, Available: true, AutoRoutable: true},
		{ID: "standard-model", Power: 70, Available: true, AutoRoutable: true},
		{ID: "smart-model", Power: 90, Available: true, AutoRoutable: true},
	}

	dir := setupWorkIntakeFixture(t)
	store := bead.NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, store.Update("ddx-intake-test", func(b *bead.Bead) {
		b.IssueType = "docs"
	}))

	root := NewCommandFactory(dir).NewRootCommand()
	out, _ := executeCommand(root, "work",
		"--once",
		"--no-review",
		"--no-review-i-know-what-im-doing",
	)

	stub.mu.Lock()
	executionSeen := stub.executionSeen
	executionReq := stub.executionReq
	stub.mu.Unlock()

	require.True(t, executionSeen, "ddx work must run implementer for zero-config cheap tier test (output=%q)", out)
	assert.Equal(t, 0, executionReq.MinPower)
}
