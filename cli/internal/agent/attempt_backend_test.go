package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/require"
)

// TestSharedTrackerLockPathIsBackendIndependent asserts that
// lockmetrics.SharedTrackerLockPath resolves to the same lock directory for
// worktree and local-clone attempt backends on one project root — including
// when resolved from each backend's WorkDir (ddx-39e78654).
func TestSharedTrackerLockPathIsBackendIndependent(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))

	ctx := context.Background()
	wt, err := (WorktreeAttemptBackend{}).Prepare(ctx, AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-lock-wt",
		AttemptID:   "20260726T000001-wt",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, wt)
	t.Cleanup(func() { _ = (WorktreeAttemptBackend{}).Cleanup(ctx, wt) })

	cl, err := (LocalCloneAttemptBackend{}).Prepare(ctx, AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-lock-cl",
		AttemptID:   "20260726T000001-cl",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, cl)
	t.Cleanup(func() { _ = (LocalCloneAttemptBackend{}).Cleanup(ctx, cl) })

	want := lockmetrics.SharedTrackerLockPath(projectRoot)
	require.Equal(t, want, lockmetrics.SharedTrackerLockPath(wt.ProjectRoot),
		"worktree ProjectRoot lock path")
	require.Equal(t, want, lockmetrics.SharedTrackerLockPath(cl.ProjectRoot),
		"local-clone ProjectRoot lock path")
	require.Equal(t, want, lockmetrics.SharedTrackerLockPath(wt.WorkDir),
		"worktree WorkDir must share project lock domain")
	require.Equal(t, want, lockmetrics.SharedTrackerLockPath(cl.WorkDir),
		"local-clone WorkDir must share project lock domain")
}

func TestResolveAttemptBackendDefaultsToLocalClone(t *testing.T) {
	rcfg := (&config.Config{Version: "1.0", Agent: &config.AgentConfig{}}).Resolve(config.CLIOverrides{})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendLocalClone, backend.Name())
}

func TestResolveAttemptBackendExplicitWorktree(t *testing.T) {
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendWorktree,
	})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendWorktree, backend.Name())
}

func TestDefaultAttemptBackendSandboxCanCommitWithoutPrimaryGitMetadata(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	rcfg := (&config.Config{Version: "1.0", Agent: &config.AgentConfig{}}).Resolve(config.CLIOverrides{})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendLocalClone, backend.Name())

	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-int-0001",
		AttemptID:   "20260720-sandboxed-default",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Cleanup(context.Background(), ws) })

	gitDir := runGitInteg(t, ws.WorkDir, "rev-parse", "--absolute-git-dir")
	require.Equal(t, filepath.Join(ws.WorkDir, ".git"), gitDir)
	require.NotContains(t, gitDir, filepath.Join(projectRoot, ".git", "worktrees"))

	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, "sandboxed.txt"), []byte("ok\n"), 0o644))
	runGitInteg(t, ws.WorkDir, "add", "sandboxed.txt")
	runGitInteg(t, ws.WorkDir, "commit", "-m", "test: sandboxed default clone commit")
}

func TestReusableAttemptWorkspaceScrubsCrossBeadState(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	ws, err := (LocalCloneAttemptBackend{}).Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-a",
		AttemptID:   "20260728T000001-a",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = (LocalCloneAttemptBackend{}).Cleanup(context.Background(), ws) })

	// Bead A residue: a staged tracked change plus an unstaged tracked change.
	seedPath := filepath.Join(ws.WorkDir, "seed.txt")
	require.NoError(t, os.WriteFile(seedPath, []byte("bead-a staged line\n"), 0o644))
	runGitInteg(t, ws.WorkDir, "add", "seed.txt")
	require.NoError(t, os.WriteFile(seedPath, []byte("bead-a modified line\n"), 0o644))

	// Bead A residue: untracked source, evidence, credential material, and
	// cleanup metadata that must not survive into bead B's run.
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, "bead-a-untracked.txt"), []byte("untracked\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".ddx", "executions", "bead-a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".ddx", "executions", "bead-a", "result.json"), []byte("{\"bead\":\"a\"}\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".codex"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".claude"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(ws.WorkDir, ".local", "state", "fizeau"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".codex", "auth.json"), []byte(`{"token":"a"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".codex", "config.toml"), []byte("model = 'a'\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".claude", ".credentials.json"), []byte(`{"credential":"a"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".claude", "settings.json"), []byte(`{"theme":"dark"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".claude.json"), []byte(`{"projects":{"a":true}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "claude-quota.json"), []byte(`{"remaining":1}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "codex-quota.json"), []byte(`{"remaining":2}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "gemini-quota.json"), []byte(`{"remaining":3}`), 0o600))
	require.NoError(t, WriteExecutionCleanupMetadata(ws.WorkDir, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-a",
		AttemptID:    "20260728T000001-a",
		WorktreePath: ws.WorkDir,
		Registered:   true,
	}))

	preStatus, err := runGitIntegOutput(ws.WorkDir, "status", "--porcelain", "--untracked-files=all")
	require.NoError(t, err)
	require.NotEmpty(t, preStatus)
	require.Contains(t, preStatus, "seed.txt")
	require.Contains(t, preStatus, "bead-a-untracked.txt")

	require.NoError(t, scrubReusableAttemptWorkspace(context.Background(), ws.WorkDir, baseRev, nil))

	headRev := runGitInteg(t, ws.WorkDir, "rev-parse", "HEAD")
	require.Equal(t, baseRev, headRev)

	postStatus, err := runGitIntegOutput(ws.WorkDir, "status", "--porcelain", "--untracked-files=all")
	require.NoError(t, err)
	require.Empty(t, postStatus)

	gotSeed, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	require.Equal(t, "seed\n", string(gotSeed))

	for _, path := range []string{
		filepath.Join(ws.WorkDir, "bead-a-untracked.txt"),
		filepath.Join(ws.WorkDir, ".ddx", "executions"),
		filepath.Join(ws.WorkDir, ExecutionCleanupMetadataFileName),
		filepath.Join(ws.WorkDir, ".codex", "auth.json"),
		filepath.Join(ws.WorkDir, ".codex", "config.toml"),
		filepath.Join(ws.WorkDir, ".claude", ".credentials.json"),
		filepath.Join(ws.WorkDir, ".claude", "settings.json"),
		filepath.Join(ws.WorkDir, ".claude.json"),
		filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "claude-quota.json"),
		filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "codex-quota.json"),
		filepath.Join(ws.WorkDir, ".local", "state", "fizeau", "gemini-quota.json"),
	} {
		_, statErr := os.Stat(path)
		require.True(t, os.IsNotExist(statErr), "expected %s to be removed", path)
	}
}

func TestReusableAttemptWorkspaceHardResetsToRequestedBaseRevision(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	ws, err := (LocalCloneAttemptBackend{}).Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-a",
		AttemptID:   "20260728T000002-a",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = (LocalCloneAttemptBackend{}).Cleanup(context.Background(), ws) })

	seedPath := filepath.Join(ws.WorkDir, "seed.txt")
	require.NoError(t, os.WriteFile(seedPath, []byte("bead-a staged line\n"), 0o644))
	runGitInteg(t, ws.WorkDir, "add", "seed.txt")
	require.NoError(t, os.WriteFile(seedPath, []byte("bead-a modified line\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws.WorkDir, "bead-a-untracked.txt"), []byte("untracked\n"), 0o644))

	require.NoError(t, scrubReusableAttemptWorkspace(context.Background(), ws.WorkDir, baseRev, nil))

	headRev := runGitInteg(t, ws.WorkDir, "rev-parse", "HEAD")
	require.Equal(t, baseRev, headRev)

	cachedDiff, err := runGitIntegOutput(ws.WorkDir, "diff", "--cached", "--name-only")
	require.NoError(t, err)
	require.Empty(t, cachedDiff)

	worktreeDiff, err := runGitIntegOutput(ws.WorkDir, "diff", "--name-only")
	require.NoError(t, err)
	require.Empty(t, worktreeDiff)

	postStatus, err := runGitIntegOutput(ws.WorkDir, "status", "--porcelain", "--untracked-files=all")
	require.NoError(t, err)
	require.Empty(t, postStatus)
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

type workspaceReuseTelemetryBody struct {
	SlotHitCount  int   `json:"slot_hit_count"`
	SlotMissCount int   `json:"slot_miss_count"`
	TimeSavedMS   int64 `json:"time_saved_ms"`
	BytesSaved    int64 `json:"bytes_saved"`
}

type workspaceReuseSavings struct {
	TimeSavedMS int64
	BytesSaved  int64
}

func appendWorkspaceReuseTelemetryEvent(appender BeadEventAppender, beadID string, slot *AttemptWorkspaceSlot, savings workspaceReuseSavings) {
	if appender == nil || beadID == "" || slot == nil {
		return
	}
	body := workspaceReuseTelemetryBody{
		TimeSavedMS: savings.TimeSavedMS,
		BytesSaved:  savings.BytesSaved,
	}
	if slot.Pooled {
		body.SlotHitCount = 1
	} else {
		body.SlotMissCount = 1
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := fmt.Sprintf("slot_hit_count=%d slot_miss_count=%d time_saved_ms=%d bytes_saved=%d",
		body.SlotHitCount, body.SlotMissCount, body.TimeSavedMS, body.BytesSaved)
	_ = appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "attempt-workspace-reuse",
		Summary: summary,
		Body:    string(data),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	})
}

func TestAttemptWorkspaceReuseTelemetryRecordsHitsMissesAndSavings(t *testing.T) {
	root := t.TempDir()
	key := AttemptWorkspaceSlotKey{
		ProjectRoot:   "/proj/reuse-telemetry",
		Backend:       AttemptBackendLocalClone,
		WorkerSlot:    "w0",
		TrustBoundary: "default",
	}

	maxSlots := 1
	enabled := true
	pool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{
		Enabled:  &enabled,
		MaxSlots: &maxSlots,
	}).withRoot(root)

	reusedFirst, err := pool.Allocate(key)
	require.NoError(t, err)
	require.NotNil(t, reusedFirst)
	require.True(t, reusedFirst.Pooled)

	// Return the slot so the next allocation represents sequential reuse.
	require.NoError(t, pool.Release(reusedFirst))
	reusedSecond, err := pool.Allocate(key)
	require.NoError(t, err)
	require.NotNil(t, reusedSecond)
	require.True(t, reusedSecond.Pooled)
	t.Cleanup(func() { _ = pool.Release(reusedSecond) })

	disabled := false
	coldPool := NewAttemptWorkspaceSlotPool(&config.ReusableWorkspaceConfig{
		Enabled:  &disabled,
		MaxSlots: &maxSlots,
	}).withRoot(root)
	coldStart, err := coldPool.Allocate(key)
	require.NoError(t, err)
	require.NotNil(t, coldStart)
	require.False(t, coldStart.Pooled)
	t.Cleanup(func() { _ = coldPool.Release(coldStart) })

	app := &stubBeadEventAppender{}
	appendWorkspaceReuseTelemetryEvent(app, "ddx-reuse", reusedSecond, workspaceReuseSavings{
		TimeSavedMS: 18_000,
		BytesSaved:  256 << 20,
	})
	appendWorkspaceReuseTelemetryEvent(app, "ddx-cold", coldStart, workspaceReuseSavings{})

	require.Len(t, app.events, 2)

	decode := func(evt bead.BeadEvent) workspaceReuseTelemetryBody {
		t.Helper()
		var body workspaceReuseTelemetryBody
		require.NoError(t, json.Unmarshal([]byte(evt.Body), &body))
		return body
	}

	reusedBody := decode(app.events[0].Event)
	coldBody := decode(app.events[1].Event)

	require.Equal(t, 1, reusedBody.SlotHitCount)
	require.Zero(t, reusedBody.SlotMissCount)
	require.Greater(t, reusedBody.TimeSavedMS, int64(0))
	require.Greater(t, reusedBody.BytesSaved, int64(0))

	require.Zero(t, coldBody.SlotHitCount)
	require.Equal(t, 1, coldBody.SlotMissCount)
	require.Zero(t, coldBody.TimeSavedMS)
	require.Zero(t, coldBody.BytesSaved)

	reusedMap := map[string]any{}
	coldMap := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(app.events[0].Event.Body), &reusedMap))
	require.NoError(t, json.Unmarshal([]byte(app.events[1].Event.Body), &coldMap))
	require.Equal(t, len(reusedMap), len(coldMap), "combined telemetry shape must match across reused and cold-start attempts")
	for key := range reusedMap {
		_, ok := coldMap[key]
		require.Truef(t, ok, "cold-start event missing key %q", key)
	}
	for key := range coldMap {
		_, ok := reusedMap[key]
		require.Truef(t, ok, "reused event missing key %q", key)
	}
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
	rcfg := cfg.Resolve(config.CLIOverrides{Harness: "script", Model: directivePath})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner:    scriptHarnessAgentRunner{},
		AttemptBackend: LocalCloneAttemptBackend{},
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	require.NotEmpty(t, res.ResultRev)
	require.NotEqual(t, res.BaseRev, res.ResultRev)

	out, catErr := runGitIntegOutput(projectRoot, "cat-file", "-e", res.ResultRev+"^{commit}")
	require.NoError(t, catErr, out)

	landing, landErr := LandBeadResult(projectRoot, res, &RealGitOps{}, BeadLandingOptions{
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

func TestDockerNestedDDXRunArgsPreserveOpaqueEnvelope(t *testing.T) {
	t.Run("surrounding whitespace is preserved and power override wins", func(t *testing.T) {
		cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
			Harness:     " harness-with-spaces ",
			Provider:    " provider-with-spaces ",
			Model:       " model-with-spaces ",
			Profile:     " policy-with-spaces ",
			Effort:      " effort-with-spaces ",
			Permissions: " config-permission ",
			MinPower:    2,
			MaxPower:    7,
		})
		args := dockerNestedDDXRunArgs(AttemptBackendRunRequest{
			Config: cfg,
			Runtime: AgentRunRuntime{
				PermissionsOverride: " runtime-permission ",
				MinPowerOverride:    4,
			},
		}, "/work/.ddx/executions/attempt/repair.md")

		require.Equal(t, " harness-with-spaces ", nestedDDXArgValue(t, args, "--harness"))
		require.Equal(t, " provider-with-spaces ", nestedDDXArgValue(t, args, "--provider"))
		require.Equal(t, " model-with-spaces ", nestedDDXArgValue(t, args, "--model"))
		require.Equal(t, " policy-with-spaces ", nestedDDXArgValue(t, args, "--profile"))
		require.Equal(t, " effort-with-spaces ", nestedDDXArgValue(t, args, "--effort"))
		require.Equal(t, " runtime-permission ", nestedDDXArgValue(t, args, "--permissions"))
		require.Equal(t, "4", nestedDDXArgValue(t, args, "--min-power"))
		require.Equal(t, "7", nestedDDXArgValue(t, args, "--max-power"))

		configPermissionArgs := dockerNestedDDXRunArgs(AttemptBackendRunRequest{Config: cfg}, "/work/prompt.md")
		require.Equal(t, " config-permission ", nestedDDXArgValue(t, configPermissionArgs, "--permissions"))
	})

	t.Run("whitespace-only opaque values survive and exact empty permission is omitted", func(t *testing.T) {
		cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
			Harness:  " \t ",
			Provider: "\n",
			Model:    "  ",
			Profile:  "\t",
			Effort:   " \r ",
		})
		args := dockerNestedDDXRunArgs(AttemptBackendRunRequest{Config: cfg}, "/work/prompt.md")

		require.Equal(t, " \t ", nestedDDXArgValue(t, args, "--harness"))
		require.Equal(t, "\n", nestedDDXArgValue(t, args, "--provider"))
		require.Equal(t, "  ", nestedDDXArgValue(t, args, "--model"))
		require.Equal(t, "\t", nestedDDXArgValue(t, args, "--profile"))
		require.Equal(t, " \r ", nestedDDXArgValue(t, args, "--effort"))
		for _, flag := range []string{"--permissions", "--min-power", "--max-power"} {
			require.NotContains(t, args, flag)
		}
	})
}

func nestedDDXArgValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for index, arg := range args {
		if arg == flag {
			require.Less(t, index+1, len(args), "flag %s missing value in %v", flag, args)
			return args[index+1]
		}
	}
	t.Fatalf("flag %s not found in %s", flag, strings.Join(args, " "))
	return ""
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
	testutils.MakeInitializedDDxRoot(t, projectRoot)
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
func (failingPrepareAttemptBackend) ImportCandidate(context.Context, *AttemptWorkspace, *ExecuteBeadResult) error {
	return nil
}
func (failingPrepareAttemptBackend) ReleaseCandidateImport(context.Context, *AttemptWorkspace) error {
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
		AgentRunner: scriptHarnessAgentRunner{},
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

func TestResolveAttemptBackend_InTreeFromOverride(t *testing.T) {
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{
		AttemptBackend: AttemptBackendInTree,
	})
	backend, err := ResolveAttemptBackend(rcfg)
	require.NoError(t, err)
	require.Equal(t, AttemptBackendInTree, backend.Name())
}

func TestInTreeAttemptBackendSelectsProjectRoot(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	backend := InTreeAttemptBackend{}
	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-test",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, ws)
	require.Equal(t, projectRoot, ws.WorkDir)
	require.Equal(t, AttemptBackendInTree, ws.Backend)
	t.Cleanup(func() {
		_ = backend.Cleanup(context.Background(), ws)
	})
}

func TestInTreeAttemptBackendDirtyTreeGuard(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0002"

	dirtyFile := filepath.Join(projectRoot, "dirty.txt")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("dirty content"), 0o644))

	backend := InTreeAttemptBackend{}
	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-test",
		BaseRev:     baseRev,
	})
	require.Error(t, err)
	require.Nil(t, ws)
	require.Contains(t, err.Error(), "clean working tree")
	require.Contains(t, err.Error(), "dirty.txt")
}

func TestInTreeAttemptBackendExcludesDDxDir(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0003"

	ddxDir := ddxroot.InTree(projectRoot)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	ddxFile := filepath.Join(ddxDir, "test.txt")
	require.NoError(t, os.WriteFile(ddxFile, []byte("ddx state"), 0o644))

	backend := InTreeAttemptBackend{}
	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-test",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, ws)
	t.Cleanup(func() {
		_ = backend.Cleanup(context.Background(), ws)
	})
}

func TestInTreeAttemptBackendExcludesExecutionArtifacts(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0004"
	attemptID := "20260527T105633-test"

	execDir := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)
	require.NoError(t, os.MkdirAll(execDir, 0o755))
	execFile := filepath.Join(execDir, "result.json")
	require.NoError(t, os.WriteFile(execFile, []byte(`{"status":"success"}`), 0o644))

	backend := InTreeAttemptBackend{}
	ws, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   attemptID,
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, ws)
	t.Cleanup(func() {
		_ = backend.Cleanup(context.Background(), ws)
	})
}

func TestInTreeAttemptBackendExclusiveLock(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0005"

	backend := InTreeAttemptBackend{}

	ws1, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-first",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, ws1)
	t.Cleanup(func() {
		_ = backend.Cleanup(context.Background(), ws1)
	})

	ws2, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-second",
		BaseRev:     baseRev,
	})
	require.Error(t, err)
	require.Nil(t, ws2)
	require.Contains(t, err.Error(), "already running")
}

// TestAttemptBackend_LocalCloneCleanupUnchanged verifies that local-clone and
// worktree backend cleanup remains routed through the existing non-docker path
// (os.RemoveAll for local-clone, gitOps.WorktreeRemove for worktree). The
// docker-clone Cleanup additions must not alter these paths.
func TestAttemptBackend_LocalCloneCleanupUnchanged(t *testing.T) {
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "sentinel.txt"), []byte("test"), 0o644))

	ws := &AttemptWorkspace{
		Backend:   AttemptBackendLocalClone,
		WorkDir:   workDir,
		BeadID:    "ddx-test",
		AttemptID: "test-attempt",
	}
	err := (LocalCloneAttemptBackend{}).Cleanup(context.Background(), ws)
	require.NoError(t, err)
	require.NoDirExists(t, workDir, "local-clone Cleanup must remove the work dir via os.RemoveAll")
}

func TestInTreeAttemptBackendReleasesLockOnCleanup(t *testing.T) {
	projectRoot, baseRev := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0006"

	backend := InTreeAttemptBackend{}

	ws1, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-first",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	_ = backend.Cleanup(context.Background(), ws1)

	ws2, err := backend.Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   "20260527T105633-second",
		BaseRev:     baseRev,
	})
	require.NoError(t, err)
	require.NotNil(t, ws2)
	t.Cleanup(func() {
		_ = backend.Cleanup(context.Background(), ws2)
	})
}
