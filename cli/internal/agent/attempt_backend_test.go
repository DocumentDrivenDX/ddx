package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveAttemptBackend_DefaultsToWorktree(t *testing.T) {
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendWorktree, backend.Name())
}

func TestResolveAttemptBackend_DockerCloneFromOverride(t *testing.T) {
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendDockerClone,
	})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendDockerClone, backend.Name())
}

func TestResolveAttemptBackend_RejectsUnknownBackend(t *testing.T) {
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: "bogus",
	})
	_, err := ResolveAttemptBackend(rcfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown attempt backend")
}

func TestExecuteBeadWithConfig_LocalCloneBackendImportsResult(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	directivePath := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, directivePath, []string{
		"append-line output.txt from local clone backend",
		"commit chore: local clone backend output",
	})

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: directivePath})
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner:    NewRunner(Config{}),
		AttemptBackend: LocalCloneAttemptBackend{},
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	require.NotEmpty(t, res.ResultRev)
	require.NotEqual(t, res.BaseRev, res.ResultRev)

	out, catErr := runGitIntegOutput(projectRoot, "cat-file", "-e", res.ResultRev+"^{commit}")
	require.NoError(t, catErr, out)

	landing, landErr := LandBeadResult(projectRoot, res, &RealOrchestratorGitOps{}, BeadLandingOptions{
		LandingAdvancer: func(r *ExecuteBeadResult) (*LandResult, error) {
			return Land(projectRoot, BuildLandRequestFromResult(projectRoot, r), RealLandingGitOps{})
		},
	})
	require.NoError(t, landErr)
	require.Equal(t, "merged", landing.Outcome)

	showOut, showErr := runGitIntegOutput(projectRoot, "show", "HEAD:output.txt")
	require.NoError(t, showErr, showOut)
	require.Contains(t, showOut, "from local clone backend")
}

func TestDockerRunArgs_AppliesResourceLimitsAndMounts(t *testing.T) {
	ws := &AttemptWorkspace{
		ProjectRoot: "/repo/project",
		WorkDir:     "/tmp/ddx-exec-wt/.execute-bead-clone-ddx-1-attempt",
		BeadID:      "ddx-1",
		AttemptID:   "20260518T100000-deadbeef",
	}
	args := dockerRunArgs(&config.ExecutionsDockerConfig{
		Memory:     "8g",
		MemorySwap: "8g",
		CPUs:       "4",
		PidsLimit:  1024,
		TmpfsSize:  "2g",
		Network:    "none",
	}, ws, "/usr/bin/ddx", "runner:latest")

	require.Contains(t, args, "--memory")
	require.Contains(t, args, "8g")
	require.Contains(t, args, "--memory-swap")
	require.Contains(t, args, "--cpus")
	require.Contains(t, args, "--pids-limit")
	require.Contains(t, args, "--network")
	require.Contains(t, args, "/tmp:rw,nosuid,nodev,size=2g,mode=1777")
	require.Contains(t, args, "type=bind,src=/usr/bin/ddx,dst=/usr/local/bin/ddx,readonly")
	require.Equal(t, "runner:latest", args[len(args)-1])
}

func TestShouldRetryCloneWithoutHardlinks(t *testing.T) {
	require.True(t, shouldRetryCloneWithoutHardlinks("", []byte("fatal: Invalid cross-device link")))
	require.True(t, shouldRetryCloneWithoutHardlinks("hardlink", []byte("operation not permitted")))
	require.False(t, shouldRetryCloneWithoutHardlinks("copy", []byte("fatal: Invalid cross-device link")))
	require.False(t, shouldRetryCloneWithoutHardlinks("", []byte("fatal: repository not found")))
}
