package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
		DockerRun:   "/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt",
		DockerHome:  "/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt/home",
	}
	args := dockerRunArgs(&config.ExecutionsDockerConfig{
		Memory:     "8g",
		MemorySwap: "8g",
		CPUs:       "4",
		PidsLimit:  1024,
		TmpfsSize:  "2g",
		Network:    "none",
	}, ws, "/usr/bin/ddx", "runner:latest", []dockerToolMount{{Name: "codex", Path: "/usr/bin/codex"}})

	require.Contains(t, args, "--rm")
	require.Contains(t, args, "--init")
	require.Contains(t, args, "--memory")
	require.Contains(t, args, "8g")
	require.Contains(t, args, "--memory-swap")
	require.Contains(t, args, "--cpus")
	require.Contains(t, args, "--pids-limit")
	require.Contains(t, args, "--network")
	require.Contains(t, args, "/tmp:rw,nosuid,nodev,size=2g,mode=1777")
	require.Contains(t, args, "type=bind,src=/usr/bin/ddx,dst=/usr/local/bin/ddx,readonly")
	require.Contains(t, args, "PATH=/usr/local/go/bin:/opt/go/bin:/usr/local/bin:/usr/bin:/bin")
	require.Contains(t, args, "HOME=/ddx-runtime/home")
	require.Contains(t, args, "GOCACHE=/work/.gocache")
	require.Contains(t, args, "GOTMPDIR=/ddx-runtime/go-tmp")
	require.NotContains(t, args, "GOMODCACHE=/ddx-runtime/go/pkg/mod")
	require.NotContains(t, args, "GOCACHE=/ddx-runtime/go-build-cache")
	require.Contains(t, args, "type=bind,src=/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt,dst=/ddx-runtime")
	require.Contains(t, args, "type=bind,src=/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt/work-gocache,dst=/work/.gocache")
	require.Contains(t, args, "type=bind,src=/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt/work-tmp,dst=/work/.tmp")
	require.Contains(t, args, "type=bind,src=/usr/bin/codex,dst=/usr/local/bin/codex,readonly")
	require.Equal(t, "runner:latest", args[len(args)-1])
}

func TestLocalCloneAttemptBackendExcludesTransientMountDirs(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	ws, err := (LocalCloneAttemptBackend{}).Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-int-0001",
		AttemptID:   "20260518T100000-deadbeef",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = (LocalCloneAttemptBackend{}).Cleanup(context.Background(), ws)
	})

	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".gocache"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".tmp"), 0o755))

	excludePath, err := runGitIntegOutput(ws.WorkDir, "rev-parse", "--git-path", "info/exclude")
	require.NoError(t, err)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(ws.WorkDir, excludePath)
	}
	excludeRaw, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	require.Contains(t, string(excludeRaw), "/.gocache/")
	require.Contains(t, string(excludeRaw), "/.tmp/")

	out, err := runGitIntegOutput(ws.WorkDir, "check-ignore", "-v", ".gocache", ".tmp")
	require.NoError(t, err, out)
	require.Contains(t, out, "/.gocache/")
	require.Contains(t, out, "/.tmp/")

	status, err := runGitIntegOutput(ws.WorkDir, "status", "--short")
	require.NoError(t, err, status)
	require.NotContains(t, status, ".gocache")
	require.NotContains(t, status, ".tmp")
}

func TestPrepareDockerAttemptHomeCopiesMinimalAuth(t *testing.T) {
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	require.NoError(t, os.MkdirAll(filepath.Join(hostHome, ".codex"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(hostHome, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".codex", "auth.json"), []byte(`{"token":"test"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".codex", "config.toml"), []byte("model = 'test'\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".codex", "logs_2.sqlite"), []byte("large runtime state"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".claude", ".credentials.json"), []byte(`{"credential":"test"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".claude", "history.jsonl"), []byte("runtime history"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".claude.json"), []byte(`{"projects":{}}`), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(hostHome, ".local", "state", "fizeau"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".local", "state", "fizeau", "claude-quota.json"), []byte(`{"five_hour_remaining":96}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".local", "state", "fizeau", "codex-quota.json"), []byte(`{"remaining":50}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(hostHome, ".local", "state", "fizeau", "gemini-quota.json"), []byte(`{"remaining":50}`), 0o600))

	attemptHome := filepath.Join(t.TempDir(), "attempt-home")
	require.NoError(t, prepareDockerAttemptHome(attemptHome))

	require.FileExists(t, filepath.Join(attemptHome, ".codex", "auth.json"))
	require.FileExists(t, filepath.Join(attemptHome, ".codex", "config.toml"))
	require.FileExists(t, filepath.Join(attemptHome, ".claude", ".credentials.json"))
	require.FileExists(t, filepath.Join(attemptHome, ".claude.json"))
	require.FileExists(t, filepath.Join(attemptHome, ".local", "state", "fizeau", "claude-quota.json"))
	require.FileExists(t, filepath.Join(attemptHome, ".local", "state", "fizeau", "codex-quota.json"))
	require.FileExists(t, filepath.Join(attemptHome, ".local", "state", "fizeau", "gemini-quota.json"))
	require.NoFileExists(t, filepath.Join(attemptHome, ".codex", "logs_2.sqlite"))
	require.NoFileExists(t, filepath.Join(attemptHome, ".claude", "history.jsonl"))
}

func TestPrepareDockerAttemptRuntimeCreatesCacheRoots(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "attempt-runtime")
	require.NoError(t, prepareDockerAttemptRuntime(runDir))

	for _, dir := range []string{
		"cache",
		filepath.Join("go", "pkg", "mod"),
		"go-build-cache",
		"go-tmp",
		"tmp",
		"work-gocache",
		"work-tmp",
	} {
		require.DirExists(t, filepath.Join(runDir, dir))
	}
}

func TestDockerProjectDockerfileAutodetectsProjectLayer(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(ddxroot.InTree(projectRoot), 0o755))
	dockerfile := ddxroot.InTree(projectRoot, "attempt-runner.Dockerfile")
	require.NoError(t, os.WriteFile(dockerfile, []byte("FROM scratch\n"), 0o644))

	got, ok, err := dockerProjectDockerfile(projectRoot, nil)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dockerfile, got)
}

func TestDockerProjectDockerfileRejectsEscapes(t *testing.T) {
	projectRoot := t.TempDir()
	_, _, err := dockerProjectDockerfile(projectRoot, &config.ExecutionsDockerConfig{
		ProjectDockerfile: "../Dockerfile",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes project root")
}

func TestDockerProjectImageSkipsProjectDockerfile(t *testing.T) {
	image, err := resolveDockerAttemptImage(context.Background(), &config.ExecutionsDockerConfig{
		ProjectImage: "project-runner:dev",
	}, t.TempDir(), "base-runner:dev")
	require.NoError(t, err)
	require.Equal(t, "project-runner:dev", image)
}

func TestDockerRunArgs_BindsSharedGoCacheWhenSet(t *testing.T) {
	ws := &AttemptWorkspace{
		ProjectRoot:         "/repo/project",
		WorkDir:             "/tmp/ddx-exec-wt/.execute-bead-clone-ddx-1-attempt",
		BeadID:              "ddx-1",
		AttemptID:           "20260519T100000-deadbeef",
		DockerRun:           "/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt",
		DockerHome:          "/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt/home",
		DockerSharedGoCache: "/tmp/ddx-exec-wt/.ddx-shared-cache-abc123/gocache",
	}
	args := dockerRunArgs(nil, ws, "/usr/bin/ddx", "runner:latest", nil)
	require.Contains(t, args, "type=bind,src=/tmp/ddx-exec-wt/.ddx-shared-cache-abc123/gocache,dst=/work/.gocache")
	require.NotContains(t, args, "type=bind,src=/tmp/ddx-exec-wt/.execute-bead-runtime-ddx-1-attempt/work-gocache,dst=/work/.gocache")
}

func TestSkipRebuildIfImagePresent_DefaultsTrue(t *testing.T) {
	require.True(t, skipRebuildIfImagePresent(nil))
	require.True(t, skipRebuildIfImagePresent(&config.ExecutionsDockerConfig{}))
	off := false
	require.False(t, skipRebuildIfImagePresent(&config.ExecutionsDockerConfig{SkipImageRebuildIfPresent: &off}))
	on := true
	require.True(t, skipRebuildIfImagePresent(&config.ExecutionsDockerConfig{SkipImageRebuildIfPresent: &on}))
}

func TestDockerSharedGoCacheDisabled(t *testing.T) {
	require.False(t, dockerSharedGoCacheDisabled(nil))
	require.False(t, dockerSharedGoCacheDisabled(&config.ExecutionsDockerConfig{}))
	require.True(t, dockerSharedGoCacheDisabled(&config.ExecutionsDockerConfig{DisableSharedGoCache: true}))
}

func TestShouldRetryCloneWithoutHardlinks(t *testing.T) {
	require.True(t, shouldRetryCloneWithoutHardlinks("", []byte("fatal: Invalid cross-device link")))
	require.True(t, shouldRetryCloneWithoutHardlinks("hardlink", []byte("operation not permitted")))
	require.False(t, shouldRetryCloneWithoutHardlinks("copy", []byte("fatal: Invalid cross-device link")))
	require.False(t, shouldRetryCloneWithoutHardlinks("", []byte("fatal: repository not found")))
}

// failingPrepareAttemptBackend fails isolated-worktree creation with a fixed
// error, simulating `git worktree add` running out of disk.
type failingPrepareAttemptBackend struct{ prepareErr error }

func (failingPrepareAttemptBackend) Name() string { return "failing-prepare" }

func (b failingPrepareAttemptBackend) Prepare(context.Context, AttemptBackendPrepareRequest) (*AttemptWorkspace, error) {
	return nil, b.prepareErr
}

func (failingPrepareAttemptBackend) Run(context.Context, AttemptBackendRunRequest) (*Result, error) {
	return nil, fmt.Errorf("Run must not be called after Prepare fails")
}

func (failingPrepareAttemptBackend) PublishResult(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}

func (failingPrepareAttemptBackend) Cleanup(context.Context, *AttemptWorkspace) error { return nil }

// TestResourceExhaustedWorktreeCreationReleasesClaim verifies that a disk
// exhaustion failure during isolated-worktree creation (after the bead is
// claimed, after a successful pre-execution resource preflight) surfaces as a
// resource_exhausted ExecuteBeadResult rather than a raw error. A raw error
// left the bead claimed-but-open and execution-ineligible until a manual
// --unclaim (ddx-f677a50b); the resource_exhausted status routes through the
// execute-loop's existing unclaim path. The loop's unclaim-and-leave-open
// behavior for this status is covered by
// TestExecuteBeadWorkerResourceExhaustedStopsLoop /
// TestExecuteBeadWorkerResourceExhaustedUnclaimsAndNoCooldown.
func TestResourceExhaustedWorktreeCreationReleasesClaim(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{})
	rcfg := cfg.Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: NewRunner(Config{}),
		AttemptBackend: failingPrepareAttemptBackend{
			prepareErr: fmt.Errorf("creating isolated worktree: git worktree add: fatal: could not create work tree dir: No space left on device"),
		},
	}, &RealGitOps{})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, ExecuteBeadStatusResourceExhausted, res.Status)
	require.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
	require.Contains(t, res.Error, "No space left on device")
}
