package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

type workResourceExhaustedChecker struct {
	calls       int32
	projectRoot string
}

func (c *workResourceExhaustedChecker) Check(ctx context.Context) (agent.ExecutionResourceCheckResult, error) {
	_ = ctx
	n := atomic.AddInt32(&c.calls, 1)
	result := agent.ExecutionResourceCheckResult{
		ProjectRoot: c.projectRoot,
		TempRoot:    filepath.Join(c.projectRoot, ".ddx", "tmp"),
		EvidenceRoots: []string{
			filepath.Join(c.projectRoot, ".ddx", "executions"),
		},
	}
	if n == 2 {
		result.CleanupSummary = agent.ExecutionCleanupSummary{
			ProjectRoot:                 c.projectRoot,
			TempRoot:                    result.TempRoot,
			ScannedTempDirs:             1,
			RemovedUnregisteredTempDirs: 1,
			BytesReclaimed:              1024,
		}
		return result, &agent.ResourceExhaustedError{
			Detail: "temp root is full",
			Result: result,
		}
	}
	return result, nil
}

type panicAgentRunner struct {
	t *testing.T
}

func (r *panicAgentRunner) Run(opts agent.RunArgs) (*agent.Result, error) {
	switch opts.PromptSource {
	case "bead-lifecycle-intake":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"classification":"atomic","confidence":0.99,"reasoning":"single-slice"}`,
		}, nil
	case "bead-lifecycle-lint":
		return &agent.Result{
			ExitCode: 0,
			Output:   `{"score":9,"rationale":"ok","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	}
	r.t.Fatalf("agent runner must not run after resource exhaustion: %+v", opts)
	return nil, nil
}

func TestWorkResourceExhausted_PrintsOperatorMessage(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	agentCfgDir := filepath.Join(homeDir, ".config", "agent")
	require.NoError(t, os.MkdirAll(agentCfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentCfgDir, "config.yaml"), []byte(`providers:
  testprov:
    type: lmstudio
    base_url: http://127.0.0.1:1
    api_key: test
    model: test-model
default_provider: testprov
`), 0o644))
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# ddx\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".ddx", "config.yaml"), []byte(`version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.com/lib
    branch: main
`), 0o644))

	out, err := exec.Command("git", "init", projectRoot).CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "add", "-A").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))

	seedExecuteBead(t, projectRoot, &bead.Bead{
		ID:       "resource-bead-1",
		Title:    "First ready bead",
		Status:   bead.StatusOpen,
		Priority: 0,
	})
	seedExecuteBead(t, projectRoot, &bead.Bead{
		ID:       "resource-bead-2",
		Title:    "Second ready bead",
		Status:   bead.StatusOpen,
		Priority: 0,
	})

	checker := &workResourceExhaustedChecker{projectRoot: projectRoot}
	factory := NewCommandFactory(projectRoot)
	factory.resourceCheckerOverride = checker
	factory.AgentRunnerOverride = &panicAgentRunner{t: t}

	root := factory.NewRootCommand()
	outStr, err := executeCommand(root, "work", "--poll-interval=0", "--project", projectRoot)
	require.NoError(t, err)
	require.Contains(t, outStr, agent.ResourceExhaustedStopMessage)
	require.Equal(t, int32(2), atomic.LoadInt32(&checker.calls))
}
