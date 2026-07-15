package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleWithoutExplicitRouteLeavesExecutionUnpinned(t *testing.T) {
	projectRoot := t.TempDir()
	rcfg := (&config.NewConfig{
		Version: "1.0",
		Agent:   &config.AgentConfig{},
	}).Resolve(config.CLIOverrides{})
	assert.Empty(t, rcfg.Model(), "resolved execution state must leave an unspecified model empty")

	svc := &passthroughTestService{}
	runtime := AgentRunRuntime{
		Prompt: "classify lifecycle readiness",
		Role:   "classifier",
	}
	applyLifecycleHookRouting(rcfg, &runtime)
	_, err := dispatchLifecycleRun(context.Background(), projectRoot, svc, nil, rcfg, runtime)
	require.NoError(t, err)
	require.True(t, svc.executeCalled)
	assert.Empty(t, svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Reasoning)
	assert.Zero(t, svc.lastReq.ProviderTimeout)
	assert.False(t, svc.listHarnessesCalled)
	assert.False(t, svc.listProvidersCalled)
	assert.False(t, svc.listModelsCalled)
	assert.False(t, svc.listPoliciesCalled)
}

// TestNewLifecycleScratchDirSeedsFromHEAD verifies the lifecycle readiness
// scratch worktree is seeded with the project's HEAD source instead of an empty
// repo. An empty scratch made the readiness-check classifier block every bead
// with "target file not found in working directory" (ddx-efadca32).
func TestNewLifecycleScratchDirSeedsFromHEAD(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)

	dir, err := newLifecycleScratchDir(projectRoot)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// The file committed at HEAD must be present in the scratch worktree —
	// the bug left only an empty .git here.
	seed := filepath.Join(dir, "seed.txt")
	require.FileExists(t, seed)
	content, err := os.ReadFile(seed)
	require.NoError(t, err)
	assert.Equal(t, "seed\n", string(content))
}
