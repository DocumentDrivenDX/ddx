package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerDesiredStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 2
	desired.DefaultSpec.Harness = "fiz"
	desired.DefaultSpec.Model = "qwen/qwen3.6"
	desired.DefaultSpec.Provider = "openrouter"
	desired.DefaultSpec.Profile = "default"
	desired.DefaultSpec.LabelFilter = "phase:reliability"
	desired.DefaultSpec.Mode = executeloop.ModeWatch
	desired.DefaultSpec.IdleInterval = executeloop.Duration{Duration: 45 * time.Second}
	desired.Restart.Enabled = true
	desired.Restart.MaxRestartsPerHour = 4
	desired.Restart.Backoff = executeloop.Duration{Duration: 25 * time.Second}
	desired.Restart.BackoffMax = executeloop.Duration{Duration: 5 * time.Minute}

	require.NoError(t, supervisor.SaveDesiredState(&desired))

	loaded, err := supervisor.LoadDesiredState()
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())

	assert.Equal(t, root, loaded.ProjectRoot)
	assert.Equal(t, 2, loaded.DesiredCount)
	assert.Equal(t, "fiz", loaded.DefaultSpec.Harness)
	assert.Equal(t, "qwen/qwen3.6", loaded.DefaultSpec.Model)
	assert.Equal(t, "openrouter", loaded.DefaultSpec.Provider)
	assert.Equal(t, "default", loaded.DefaultSpec.Profile)
	assert.Equal(t, "phase:reliability", loaded.DefaultSpec.LabelFilter)
	assert.Equal(t, executeloop.ModeWatch, loaded.DefaultSpec.Mode)
	assert.Equal(t, 45*time.Second, loaded.DefaultSpec.IdleInterval.Duration)
	assert.Equal(t, executeloop.SpecCurrentVersion, loaded.DefaultSpec.SpecVersion)
	assert.True(t, loaded.Restart.Enabled)
	assert.Equal(t, 4, loaded.Restart.MaxRestartsPerHour)
	assert.Equal(t, 25*time.Second, loaded.Restart.Backoff.Duration)
	assert.Equal(t, 5*time.Minute, loaded.Restart.BackoffMax.Duration)
	assert.True(t, loaded.UpdatedAt.Equal(desired.UpdatedAt))
}

func TestWorkerDesiredStateV1ProfileMigrationPreservesOnlyUnambiguousPins(t *testing.T) {
	tests := []struct {
		name        string
		legacy      string
		wantProfile string
	}{
		{
			name:        "ambiguous synthesized default becomes unpinned",
			legacy:      "default",
			wantProfile: "",
		},
		{
			name:        "unambiguous explicit profile is preserved",
			legacy:      "operator-custom-profile",
			wantProfile: "operator-custom-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			setupBeadStore(t, root)
			manager := NewWorkerManager(root)
			defer manager.StopWatchdog()
			supervisor := NewWorkerSupervisor(manager)

			legacy := DefaultWorkerDesiredState(root)
			legacy.Version = legacyWorkerDesiredStateVersion
			legacy.DefaultSpec.Profile = tt.legacy
			require.NoError(t, os.MkdirAll(manager.rootDir, 0o755))
			data, err := json.MarshalIndent(legacy, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(supervisor.desiredStatePath(), append(data, '\n'), 0o644))

			loaded, err := supervisor.LoadDesiredState()
			require.NoError(t, err)
			assert.Equal(t, workerDesiredStateVersion, loaded.Version)
			assert.Equal(t, tt.wantProfile, loaded.DefaultSpec.Profile)
		})
	}
}

func TestWorkerDesiredStateRejectsRemovedReviewRoutingFields(t *testing.T) {
	for _, field := range []string{"review_harness", "review_model"} {
		t.Run(field, func(t *testing.T) {
			root := t.TempDir()
			setupBeadStore(t, root)
			manager := NewWorkerManager(root)
			defer manager.StopWatchdog()
			supervisor := NewWorkerSupervisor(manager)
			require.NoError(t, os.MkdirAll(manager.rootDir, 0o755))

			raw := fmt.Sprintf(`{
  "version": %d,
  "project_root": %q,
  "desired_count": 0,
  "default_spec": {"mode":"watch", %q:"removed-route-pin"},
  "restart": {"enabled": false}
}`, workerDesiredStateVersion, root, field)
			require.NoError(t, os.WriteFile(supervisor.desiredStatePath(), []byte(raw), 0o644))

			_, err := supervisor.LoadDesiredState()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "default_spec."+field+" is no longer supported")
		})
	}

	t.Run("unrelated future fields remain forward compatible", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)
		manager := NewWorkerManager(root)
		defer manager.StopWatchdog()
		supervisor := NewWorkerSupervisor(manager)
		require.NoError(t, os.MkdirAll(manager.rootDir, 0o755))

		raw := fmt.Sprintf(`{
  "version": %d,
  "project_root": %q,
  "desired_count": 0,
  "default_spec": {"mode":"watch", "future_spec_hint":"preserved-by-newer-client"},
  "restart": {"enabled": false},
  "updated_at": %q,
  "future_supervisor_hint": "preserved-by-newer-client"
}`, workerDesiredStateVersion, root, time.Now().UTC().Format(time.RFC3339Nano))
		require.NoError(t, os.WriteFile(supervisor.desiredStatePath(), []byte(raw), 0o644))

		loaded, err := supervisor.LoadDesiredState()
		require.NoError(t, err)
		assert.Equal(t, root, loaded.ProjectRoot)
		assert.Equal(t, executeloop.ModeWatch, loaded.DefaultSpec.Mode)
	})
}

func TestWorkerDesiredStateV1ProfileMigrationReconcilesToOpaqueExecute(t *testing.T) {
	tests := []struct {
		name       string
		legacy     string
		wantPolicy string
	}{
		{
			name:       "synthesized default becomes an unpinned Execute",
			legacy:     "default",
			wantPolicy: "",
		},
		{
			name:       "operator profile remains byte-identical",
			legacy:     " operator-custom-profile ",
			wantPolicy: " operator-custom-profile ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
			svc := &workerIntakeServiceStub{}
			installWorkerIntakeStub(t, svc)
			root := setupWorkerResolveRouteRepo(t)
			store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
			require.NoError(t, store.Init(context.Background()))
			require.NoError(t, store.Create(context.Background(), &bead.Bead{
				ID:          "ddx-desired-state-profile-migration",
				Title:       "execution: prove desired-state profile migration",
				IssueType:   bead.DefaultType,
				Description: "PROBLEM\nA migrated worker must preserve the route-neutral boundary.\n\nROOT CAUSE\ncli/internal/server/workers_supervisor.go migrates legacy desired state.\n\nPROPOSED FIX\nCapture the implementation Execute request.\n\nNON-SCOPE\nDo not select a concrete route.\n",
				Acceptance:  "1. TestWorkerDesiredStateV1ProfileMigrationReconcilesToOpaqueExecute passes\n2. cd cli && go test ./internal/server/... passes\n3. lefthook run pre-commit passes\n",
				Labels:      []string{"phase:build", "area:server", "kind:test"},
			}))
			runCmd(t, root, "git", "add", ".")
			runCmd(t, root, "git", "commit", "-m", "seed desired-state migration bead")

			manager := NewWorkerManager(root)
			defer manager.StopWatchdog()
			manager.LandCoordinators.gitOpsOverride = &fakeLandingGitOps{}
			t.Cleanup(func() {
				stopProjectWorkers(t, manager, root)
				manager.LandCoordinators.StopAll()
			})
			supervisor := NewWorkerSupervisor(manager)
			legacy := DefaultWorkerDesiredState(root)
			legacy.Version = legacyWorkerDesiredStateVersion
			legacy.DesiredCount = 1
			legacy.DefaultSpec.Mode = executeloop.ModeOnce
			legacy.DefaultSpec.IdleInterval = executeloop.Duration{}
			legacy.DefaultSpec.NoReview = true
			legacy.DefaultSpec.Profile = tt.legacy
			require.NoError(t, os.MkdirAll(manager.rootDir, 0o755))
			data, err := json.MarshalIndent(legacy, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(supervisor.desiredStatePath(), append(data, '\n'), 0o644))

			require.NoError(t, supervisor.Reconcile())
			require.Eventually(t, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				for _, req := range svc.requests {
					if req.Role == "implementer" && req.Policy == tt.wantPolicy {
						return true
					}
				}
				return false
			}, 10*time.Second, 10*time.Millisecond, "migrated desired state must reach Fizeau Execute with policy %q", tt.wantPolicy)

			svc.mu.Lock()
			defer svc.mu.Unlock()
			for _, req := range svc.requests {
				if req.Role != "implementer" {
					continue
				}
				assert.Equal(t, tt.wantPolicy, req.Policy)
				return
			}
			t.Fatal("worker never emitted an implementation Execute request")
		})
	}
}

func TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 2
	desired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	seed, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:              executeloop.ModeWatch,
		IdleInterval:      executeLoopIdleInterval(30 * time.Second),
		OpaquePassthrough: true,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.NoError(t, supervisor.Reconcile())
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 2
	}, 2*time.Second, 20*time.Millisecond)

	workers := runningManagedWorkers(t, m, root)
	require.Len(t, workers, 2)
	assert.Equal(t, seed.ID, workers[1].ID, "seed worker should remain the older worker")
	newestID := workers[0].ID

	desired.DesiredCount = 1
	require.NoError(t, supervisor.SaveDesiredState(&desired))
	require.NoError(t, supervisor.Reconcile())

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		rec, err := m.Show(newestID)
		if err != nil {
			return false
		}
		return rec.State != "running"
	}, 2*time.Second, 20*time.Millisecond)

	oldest, err := m.Show(seed.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", oldest.State)
}

func TestWorkerSupervisorRestartBackoff(t *testing.T) {
	t.Run("respects backoff before restarting", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.Restart.Enabled = true
		desired.Restart.MaxRestartsPerHour = 6
		desired.Restart.Backoff = executeloop.Duration{Duration: 150 * time.Millisecond}
		desired.Restart.BackoffMax = executeloop.Duration{Duration: 150 * time.Millisecond}
		require.NoError(t, supervisor.SaveDesiredState(&desired))

		seedTerminalFailedWorker(t, m, root, "worker-20260627T000001-failed")

		require.NoError(t, supervisor.Reconcile())
		assert.Zero(t, runningManagedWorkerCount(t, m, root))

		time.Sleep(200 * time.Millisecond)
		require.NoError(t, supervisor.Reconcile())

		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)
	})

	t.Run("caps restarts per hour", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.Restart.Enabled = true
		desired.Restart.MaxRestartsPerHour = 1
		desired.Restart.Backoff = executeloop.Duration{Duration: 10 * time.Millisecond}
		desired.Restart.BackoffMax = executeloop.Duration{Duration: 10 * time.Millisecond}
		require.NoError(t, supervisor.SaveDesiredState(&desired))

		seedTerminalFailedWorker(t, m, root, "worker-20260627T000002-failed")
		seedTerminalFailedWorker(t, m, root, "worker-20260627T000003-failed")

		require.NoError(t, supervisor.Reconcile())
		assert.Zero(t, runningManagedWorkerCount(t, m, root))
	})
}

func TestSupervisor_ResolvedOperatorAttentionAllowsRespawnAfterDesiredUpdate(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	now := time.Now().UTC()
	terminalAt := now.Add(-time.Minute)
	seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260707T000001-oa", terminalAt)

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.UpdatedAt = terminalAt.Add(time.Second)
	require.NoError(t, writeDesiredStateForTest(supervisor, desired))

	require.NoError(t, supervisor.ReconcileAt(now))
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestSupervisor_HistoricalOperatorAttentionDoesNotBlockFreshDesiredStateAcrossRestart(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	now := time.Now().UTC()
	terminalAt := now.Add(-10 * time.Minute)
	seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260707T000002-oa", terminalAt)

	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.UpdatedAt = terminalAt.Add(time.Minute)

	first := NewWorkerSupervisor(m)
	require.NoError(t, writeDesiredStateForTest(first, desired))

	restarted := NewWorkerSupervisor(m)
	require.NoError(t, restarted.ReconcileAt(now))
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestWorkerSupervisor_DoesNotPermanentlyBlockOnStaleOperatorAttentionTerminal(t *testing.T) {
	root := t.TempDir()
	runGit(t, "init", root)
	runGit(t, "-C", root, "config", "user.email", "test@test.com")
	runGit(t, "-C", root, "config", "user.name", "Test User")
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	now := time.Now().UTC()
	terminalAt := now.Add(-10 * time.Minute)
	seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260707T000003-oa", terminalAt)

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.UpdatedAt = terminalAt.Add(-time.Minute)
	require.NoError(t, writeDesiredStateForTest(supervisor, desired))

	require.NoError(t, supervisor.ReconcileAt(now))
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestWorkerSupervisor_CurrentDirtyRootStillBlocksStart(t *testing.T) {
	root := t.TempDir()
	runGit(t, "init", root)
	runGit(t, "-C", root, "config", "user.email", "test@test.com")
	runGit(t, "-C", root, "config", "user.name", "Test User")
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	require.NoError(t, os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0o644))

	now := time.Now().UTC()
	terminalAt := now.Add(-10 * time.Minute)
	seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260707T000004-oa", terminalAt)

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.UpdatedAt = terminalAt.Add(-time.Minute)
	require.NoError(t, writeDesiredStateForTest(supervisor, desired))

	var buf bytes.Buffer
	restore := redirectStdLogger(&buf)
	defer restore()

	require.NoError(t, supervisor.ReconcileAt(now))
	assert.Zero(t, runningManagedWorkerCount(t, m, root))
	assert.Contains(t, buf.String(), "restart-blocked terminal")
	assert.Contains(t, buf.String(), "dirty.txt")
}

func TestWorkerSupervisor_DDXMetricsDirtDoesNotBlockRestart(t *testing.T) {
	root := t.TempDir()
	runGit(t, "init", root)
	runGit(t, "-C", root, "config", "user.email", "test@test.com")
	runGit(t, "-C", root, "config", "user.name", "Test User")
	setupBeadStore(t, root)

	locksPath := filepath.Join(root, ddxroot.DirName, "metrics", "locks.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(locksPath), 0o755))
	require.NoError(t, os.WriteFile(locksPath, []byte("{\"event\":\"seed\"}\n"), 0o644))
	runGit(t, "-C", root, "add", ".ddx")
	runGit(t, "-C", root, "commit", "-m", "seed ddx metadata")
	require.NoError(t, os.WriteFile(locksPath, []byte("{\"event\":\"seed\"}\n{\"event\":\"release\"}\n"), 0o644))

	dirtyPaths, known := projectRestartBlockingDirtyPaths(root)
	require.True(t, known)
	assert.Empty(t, dirtyPaths)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	installBlockingWorkerFactory(m)
	t.Cleanup(func() {
		stopProjectWorkers(t, m, root)
	})

	now := time.Now().UTC()
	terminalAt := now.Add(-10 * time.Minute)
	seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260707T000005-oa", terminalAt)

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.UpdatedAt = terminalAt.Add(-time.Minute)
	require.NoError(t, writeDesiredStateForTest(supervisor, desired))

	require.NoError(t, supervisor.ReconcileAt(now))
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, m, root) == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestWorkerSupervisor_ActiveBlockedTerminalConsumesOneDesiredSlot(t *testing.T) {
	t.Run("does not suppress all starts when desired count exceeds one", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		now := time.Now().UTC()
		terminalAt := now.Add(-time.Minute)
		seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260708T000001-oa", terminalAt)

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 2
		desired.DefaultSpec.OpaquePassthrough = true
		desired.UpdatedAt = terminalAt.Add(-time.Minute)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		require.NoError(t, supervisor.ReconcileAt(now))
		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)

		// The active blocked terminal continues to occupy one of the two
		// desired slots, so the supervisor must not top up to full count.
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, runningManagedWorkerCount(t, m, root))
	})

	t.Run("does not exceed desired count when active blocks plus live workers already satisfy it", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 2
		desired.DefaultSpec.OpaquePassthrough = true
		require.NoError(t, supervisor.SaveDesiredState(&desired))

		seed, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
			Mode:              executeloop.ModeWatch,
			IdleInterval:      executeLoopIdleInterval(30 * time.Second),
			OpaquePassthrough: true,
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)

		now := time.Now().UTC()
		terminalAt := now.Add(-time.Minute)
		seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260708T000002-oa", terminalAt)

		desired.UpdatedAt = terminalAt.Add(-time.Minute)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		require.NoError(t, supervisor.ReconcileAt(now))

		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 1, runningManagedWorkerCount(t, m, root))

		seedRec, err := m.Show(seed.ID)
		require.NoError(t, err)
		assert.Equal(t, "running", seedRec.State)
	})
}

func TestWorkerSupervisor_ExpiredBlockedTerminalFeedsRestartBackoff(t *testing.T) {
	t.Run("expired operator_attention block feeds a restartEvents entry with reason/evidence", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		now := time.Now().UTC()
		terminalAt := now.Add(-time.Minute)
		seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260708T000003-oa", terminalAt)

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.UpdatedAt = terminalAt.Add(time.Second)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		var buf bytes.Buffer
		restore := redirectStdLogger(&buf)
		defer restore()

		require.NoError(t, supervisor.ReconcileAt(now))
		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)

		assert.Contains(t, buf.String(), "expired restart-blocked terminal")
		assert.Contains(t, buf.String(), "operator_attention")

		supervisor.mu.Lock()
		restartEvents := append([]time.Time(nil), supervisor.restartEvents...)
		supervisor.mu.Unlock()
		require.Len(t, restartEvents, 1, "expired block must feed exactly one restartEvents entry")
		assert.True(t, restartEvents[0].Equal(terminalAt), "restart event should be backdated to the terminal's own timestamp")
	})

	t.Run("repeated expired dirty_root and operator_attention blocks are throttled by restart backoff", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		now := time.Now().UTC()
		firstTerminalAt := now.Add(-2 * time.Minute)
		secondTerminalAt := now.Add(-90 * time.Second)
		seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260708T000004-oa", firstTerminalAt)
		seedTerminalDirtyRootWorker(t, m, root, "worker-20260708T000005-dr", secondTerminalAt)

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.Restart.Enabled = true
		desired.Restart.MaxRestartsPerHour = 1
		desired.Restart.Backoff = executeloop.Duration{Duration: 30 * time.Second}
		desired.Restart.BackoffMax = executeloop.Duration{Duration: 30 * time.Second}
		desired.UpdatedAt = secondTerminalAt.Add(time.Second)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		require.NoError(t, supervisor.ReconcileAt(now))

		supervisor.mu.Lock()
		restartEvents := append([]time.Time(nil), supervisor.restartEvents...)
		supervisor.mu.Unlock()
		require.Len(t, restartEvents, 2, "both expired blocks must feed restartEvents")

		// Both expired blocks fed the backoff in the same tick, exceeding
		// MaxRestartsPerHour, so the throttled restart must not start.
		time.Sleep(50 * time.Millisecond)
		assert.Zero(t, runningManagedWorkerCount(t, m, root))
	})
}

func TestWorkerSupervisor_ExpiredBlockedTerminalDoesNotSuppressRestartForever(t *testing.T) {
	t.Run("operator_attention terminal older than TTL becomes restartable despite stale desired.UpdatedAt", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		now := time.Now().UTC()
		terminalAt := now.Add(-DefaultTerminalBlockTTL - time.Minute)
		seedTerminalOperatorAttentionWorker(t, m, root, "worker-20260709T000001-oa", terminalAt)

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		// desired.UpdatedAt has NOT advanced past the terminal timestamp, so
		// the existing freshness-based clear path alone would leave this
		// block in place forever without TTL expiry.
		desired.UpdatedAt = terminalAt.Add(-time.Minute)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		var buf bytes.Buffer
		restore := redirectStdLogger(&buf)
		defer restore()

		require.NoError(t, supervisor.ReconcileAt(now))
		require.Eventually(t, func() bool {
			return runningManagedWorkerCount(t, m, root) == 1
		}, 2*time.Second, 20*time.Millisecond)

		assert.Contains(t, buf.String(), "expired restart-blocked terminal")
		assert.Contains(t, buf.String(), "operator_attention")
	})

	t.Run("dirty_root terminal younger than TTL remains non-restartable when desired.UpdatedAt has not advanced", func(t *testing.T) {
		root := t.TempDir()
		setupBeadStore(t, root)

		m := NewWorkerManager(root)
		defer m.StopWatchdog()
		installBlockingWorkerFactory(m)
		t.Cleanup(func() {
			stopProjectWorkers(t, m, root)
		})

		now := time.Now().UTC()
		terminalAt := now.Add(-time.Minute)
		seedTerminalDirtyRootWorker(t, m, root, "worker-20260709T000002-dr", terminalAt)

		supervisor := NewWorkerSupervisor(m)
		desired := DefaultWorkerDesiredState(root)
		desired.DesiredCount = 1
		desired.DefaultSpec.OpaquePassthrough = true
		desired.UpdatedAt = terminalAt.Add(-time.Minute)
		require.NoError(t, writeDesiredStateForTest(supervisor, desired))

		require.NoError(t, supervisor.ReconcileAt(now))

		time.Sleep(50 * time.Millisecond)
		assert.Zero(t, runningManagedWorkerCount(t, m, root))
	})
}

// TestWorkerSupervisorExposesLatestBlockedTerminalReason proves the read-only
// status helper returns the newest restart-blocked terminal reason (not an
// older one still retained in memory).
func TestWorkerSupervisorExposesLatestBlockedTerminalReason(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	now := time.Now().UTC()
	olderAt := now.Add(-5 * time.Minute)
	newerAt := now.Add(-1 * time.Minute)

	supervisor := NewWorkerSupervisor(m)
	supervisor.recordTerminalHistory([]WorkerRecord{
		{
			ID:         "worker-older-oa",
			Kind:       "work",
			State:      "exited",
			Status:     "operator_attention",
			ReapReason: "operator_attention",
			FinishedAt: olderAt,
		},
		{
			ID:         "worker-newer-dirty",
			Kind:       "work",
			State:      "exited",
			Status:     "dirty_root",
			ReapReason: "dirty_root",
			FinishedAt: newerAt,
		},
	}, now)

	diag, ok := supervisor.LatestBlockedTerminal()
	require.True(t, ok, "expected a blocked terminal diagnostic")
	assert.Equal(t, "worker-newer-dirty", diag.WorkerID)
	assert.Equal(t, "dirty_root", diag.Reason)
	assert.True(t, diag.TerminalAt.Equal(newerAt), "expected newest terminal timestamp")
}

// TestWorkerSupervisorPrefersStructuredFDExhaustionDiagnosis proves the helper
// maps fd exhaustion through structured ManagedWorkerResult fields rather than
// free-text inference alone, and surfaces it even when the terminal is
// restartable (not restart-blocked).
func TestWorkerSupervisorPrefersStructuredFDExhaustionDiagnosis(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	now := time.Now().UTC()
	terminalAt := now.Add(-2 * time.Minute)
	const workerID = "worker-fd-exhausted"

	// Structured result.json from the agent path — the preferred source.
	require.NoError(t, os.MkdirAll(filepath.Join(m.rootDir, workerID), 0o755))
	require.NoError(t, WriteManagedWorkerResult(root, workerID, ManagedWorkerResult{
		StopCondition:                 "ResourceExhausted",
		LastFailureStatus:             agent.ExecuteBeadStatusResourceExhausted,
		LastFailureDetail:             agent.FDExhaustionStopMessage,
		ResourceExhaustionDiagnosis:   agent.ResourceExhaustionDiagnosisFD,
		ResourceExhaustionRestartable: true,
	}))

	// Free-text on the record is deliberately less specific than the
	// structured diagnosis so a brittle string matcher would pick the wrong
	// classification if structured fields were ignored.
	supervisor := NewWorkerSupervisor(m)
	supervisor.recordTerminalHistory([]WorkerRecord{
		{
			ID:         workerID,
			Kind:       "work",
			State:      "exited",
			Status:     agent.ExecuteBeadStatusResourceExhausted,
			LastError:  agent.ResourceExhaustedStopMessage,
			FinishedAt: terminalAt,
			LastResult: &WorkerExecutionResult{
				Status: agent.ExecuteBeadStatusResourceExhausted,
				Detail: agent.ResourceExhaustedStopMessage,
			},
		},
	}, now)

	diag, ok := supervisor.LatestBlockedTerminal()
	require.True(t, ok, "structured fd-exhaustion terminal must be queryable")
	assert.Equal(t, workerID, diag.WorkerID)
	assert.Equal(t, agent.ResourceExhaustionDiagnosisFD, diag.Diagnosis,
		"must prefer ManagedWorkerResult.ResourceExhaustionDiagnosis over free-text LastError")
	assert.True(t, diag.Restartable, "fd exhaustion is worker-local and restartable")
	assert.True(t, diag.TerminalAt.Equal(terminalAt))
	// Reason remains the coarse restart-visible status (resource_exhausted);
	// Diagnosis carries the structured fd classification.
	assert.Equal(t, agent.ExecuteBeadStatusResourceExhausted, diag.Reason)
}

func TestWorkerSupervisorMarksStaleRunningRecordsStopped(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-supervisor-stale")
	writeStaleClaimLeaseForTest(t, store, bead.ClaimLeaseRecord{
		BeadID:    "ddx-supervisor-stale",
		Owner:     "worker-test",
		Machine:   "stale-machine",
		StartedAt: time.Now().UTC().Add(-3 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-3 * time.Hour),
		PID:       9999998,
	})

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 0
	desired.Restart.Enabled = false
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	workerID := "worker-20260627T000004-stale"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	stale := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   time.Now().UTC().Add(-1 * time.Hour),
		PID:         0,
		CurrentBead: "ddx-supervisor-stale",
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: workerID + "-a1",
			BeadID:    "ddx-supervisor-stale",
			Phase:     "running",
			StartedAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	require.NoError(t, m.writeRecord(dir, stale))

	require.NoError(t, supervisor.Reconcile())

	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
	assert.False(t, rec.FinishedAt.IsZero())
	assert.False(t, m.hasWorkerHandle(workerID), "stale disk entry must not be adopted into memory")

	b, err := store.Get(context.Background(), "ddx-supervisor-stale")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)
}

// TestWorkerSupervisor_LivenessRejectsPGIDMismatch covers ddx-428fc1e5: a
// running record whose PID is alive but whose recorded PGID no longer
// matches the live process group must not be treated as live. This is the
// signature of PID reuse — the original worker exited and the kernel handed
// its PID to an unrelated process.
func TestWorkerSupervisor_LivenessRejectsPGIDMismatch(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	supervisor := NewWorkerSupervisor(m)

	pid := os.Getpid()
	actualPGID, err := syscall.Getpgid(pid)
	require.NoError(t, err)

	rec := WorkerRecord{
		ID:    "worker-pgid-mismatch",
		Kind:  "work",
		State: "running",
		PID:   pid,
		PGID:  actualPGID + 999999,
	}

	assert.False(t, supervisor.workerRecordLive(rec, time.Now().UTC()))
}

// TestWorkerSupervisor_LivenessRejectsStaleSidecar covers ddx-428fc1e5: a
// running record with a matching, alive PID/PGID is still not live when its
// liveness sidecar (.ddx/workers/<id>/status.json via workerstatus) has not
// been touched in over 2*bead.HeartbeatTTL.
func TestWorkerSupervisor_LivenessRejectsStaleSidecar(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	supervisor := NewWorkerSupervisor(m)

	pid := os.Getpid()
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)

	rec := WorkerRecord{
		ID:    "worker-stale-sidecar",
		Kind:  "work",
		State: "running",
		PID:   pid,
		PGID:  pgid,
	}

	now := time.Now().UTC()
	require.NoError(t, workerstatus.WriteLiveness(root, rec.ID, workerstatus.LivenessRecord{
		WorkerID:       rec.ID,
		LastActivityAt: now.Add(-3 * bead.HeartbeatTTL),
	}))

	assert.False(t, supervisor.workerRecordLive(rec, now))
}

// TestWorkerSupervisor_LivenessRejectsExpiredRunState covers ddx-428fc1e5: a
// running record with a matching, alive PID/PGID is still not live when its
// current attempt's run-state (cli/internal/agent run-state helpers) has
// expired.
func TestWorkerSupervisor_LivenessRejectsExpiredRunState(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	supervisor := NewWorkerSupervisor(m)

	pid := os.Getpid()
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)

	now := time.Now().UTC()
	attemptID := "worker-expired-run-state-a1"
	require.NoError(t, agent.WriteRunState(root, agent.RunState{
		BeadID:    "ddx-liveness-expired",
		AttemptID: attemptID,
		StartedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(-time.Minute),
	}))

	rec := WorkerRecord{
		ID:    "worker-expired-run-state",
		Kind:  "work",
		State: "running",
		PID:   pid,
		PGID:  pgid,
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: attemptID,
			BeadID:    "ddx-liveness-expired",
			Phase:     "running",
			StartedAt: now.Add(-time.Hour),
		},
	}

	assert.False(t, supervisor.workerRecordLive(rec, now))
}

// TestWorkerSupervisor_LivenessKeepsLiveHandleAuthoritative covers
// ddx-428fc1e5: when the manager still holds an in-process handle for the
// worker, that handle is authoritative and liveness is affirmed even though
// every other signal (PID, PGID) would otherwise fail.
func TestWorkerSupervisor_LivenessKeepsLiveHandleAuthoritative(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	supervisor := NewWorkerSupervisor(m)

	id := "worker-handle-authoritative"
	m.mu.Lock()
	m.workers[id] = &workerHandle{record: WorkerRecord{ID: id, Kind: "work", State: "running"}}
	m.mu.Unlock()

	rec := WorkerRecord{
		ID:    id,
		Kind:  "work",
		State: "running",
		PID:   0,
		PGID:  999999,
	}

	assert.True(t, supervisor.workerRecordLive(rec, time.Now().UTC()))
}

func TestSupervisorRegistry_DoesNotSpawnDuplicatesForActiveWorker(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.DefaultSpec.OpaquePassthrough = true
	desired.DefaultSpec.Mode = executeloop.ModeWatch
	desired.DefaultSpec.IdleInterval = executeloop.Duration{Duration: 30 * time.Second}

	supervisor := NewWorkerSupervisor(m)
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:              executeloop.ModeWatch,
		IdleInterval:      executeLoopIdleInterval(30 * time.Second),
		OpaquePassthrough: true,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return processAlive(record.PID)
	}, 5*time.Second, 20*time.Millisecond)
	t.Cleanup(func() {
		_ = m.Shutdown()
		_ = cleanupManagedWorkerProcessTree(record.PID, nil, 0)
		require.Eventually(t, func() bool {
			return !processAlive(record.PID)
		}, 5*time.Second, 20*time.Millisecond)
	})

	m.mu.Lock()
	delete(m.workers, record.ID)
	m.mu.Unlock()

	require.NoError(t, supervisor.Reconcile())

	require.Eventually(t, func() bool {
		recs, err := m.List()
		if err != nil {
			return false
		}
		running := 0
		for _, rec := range recs {
			if rec.Kind == "work" && rec.ProjectRoot == root && rec.State == "running" {
				running++
			}
		}
		return running == 1 && len(recs) == 1
	}, 5*time.Second, 20*time.Millisecond)
}

func TestSupervisorRegistry_AdoptsOrTerminatesHandleLessRunningWorkers(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 0
	desired.DefaultSpec.OpaquePassthrough = true
	desired.DefaultSpec.Mode = executeloop.ModeWatch
	desired.DefaultSpec.IdleInterval = executeloop.Duration{Duration: 30 * time.Second}

	supervisor := NewWorkerSupervisor(m)
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:              executeloop.ModeWatch,
		IdleInterval:      executeLoopIdleInterval(30 * time.Second),
		OpaquePassthrough: true,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return processAlive(record.PID)
	}, 5*time.Second, 20*time.Millisecond)
	t.Cleanup(func() {
		_ = m.Shutdown()
		_ = cleanupManagedWorkerProcessTree(record.PID, nil, 0)
	})

	m.mu.Lock()
	delete(m.workers, record.ID)
	m.mu.Unlock()

	require.NoError(t, supervisor.Reconcile())

	require.Eventually(t, func() bool {
		return !processAlive(record.PID)
	}, 5*time.Second, 20*time.Millisecond)

	rec, err := m.readRecord(filepath.Join(m.rootDir, record.ID))
	require.NoError(t, err)
	assert.Equal(t, "stopped", rec.State)
	assert.Equal(t, "stopped", rec.Status)
}

func TestSupervisorReconcile_FreshClaimingWorkerSurvivesTicks(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.Restart.Enabled = false
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	store := bead.NewStore(ddxroot.JoinProject(root))
	beadID := "ddx-supervisor-fresh-claim"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:        beadID,
		Title:     "fresh claim survives reconcile",
		Status:    bead.StatusOpen,
		Priority:  1,
		IssueType: bead.DefaultType,
	}))
	require.NoError(t, store.Claim(beadID, "ddx"))

	now := time.Now().UTC()
	liveID := "worker-20260705T000000-live"
	liveHandle, _ := newManagedIdleHandle(t, m, liveID, beadID, os.Getpid(), now.Add(-2*time.Second), now.Add(-time.Second))
	liveHandle.record.CurrentAttempt = &CurrentAttemptInfo{
		AttemptID: liveID + "-a1",
		BeadID:    beadID,
		Phase:     "running",
		StartedAt: now.Add(-2 * time.Second),
	}
	liveDir := filepath.Join(m.rootDir, liveID)
	require.NoError(t, os.MkdirAll(liveDir, 0o755))
	require.NoError(t, m.writeRecord(liveDir, liveHandle.record))

	staleID := "worker-20260705T000001-stale"
	staleDir := filepath.Join(m.rootDir, staleID)
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	stale := WorkerRecord{
		ID:          staleID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   now.Add(-5 * time.Minute),
		PID:         0,
		CurrentBead: beadID,
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: staleID + "-a1",
			BeadID:    beadID,
			Phase:     "running",
			StartedAt: now.Add(-5 * time.Minute),
		},
	}
	require.NoError(t, m.writeRecord(staleDir, stale))

	for i := 0; i < 3; i++ {
		require.NoError(t, supervisor.ReconcileAt(now.Add(time.Duration(i)*time.Second)))
	}

	liveRec, err := m.readRecord(liveDir)
	require.NoError(t, err)
	assert.Equal(t, "running", liveRec.State)

	staleRec, err := m.readRecord(staleDir)
	require.NoError(t, err)
	assert.Equal(t, "stopped", staleRec.State)
	assert.Equal(t, "stopped", staleRec.Status)

	b, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusInProgress, b.Status)

	lease, found, err := store.ClaimLease(beadID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "ddx", lease.Owner)
	assert.True(t, m.hasWorkerHandle(liveID))
}

// TestSupervisorReconcile_LiveDiskOnlyAttemptSurvivesTicks covers the post-
// restart case: a real live managed attempt exists only as a disk record
// (no manager handle). Secondary liveness signals may be stale, so snapshot
// classifies the worker as stale, but a fresh same-machine claim must keep
// the process alive across multiple reconcile ticks without spawning a
// replacement duplicate (regression ddx-1bc702ec / incident mid-attempt stop).
func TestSupervisorReconcile_LiveDiskOnlyAttemptSurvivesTicks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("live PID process-control semantics differ on Windows")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("sleep not available: %v", err)
	}

	root := t.TempDir()
	beadID := "ddx-supervisor-disk-only-live"
	store := seedClaimedBead(t, root, beadID)

	cmd := exec.Command("sleep", "600")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		if cmd.Process != nil {
			_, _ = cmd.Process.Wait()
		}
	})
	require.True(t, processAlive(pid), "fixture subprocess must start alive")

	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)

	machine := os.Getenv("DDX_MACHINE_ID")
	if machine == "" {
		machine, err = os.Hostname()
		require.NoError(t, err)
	}
	writeStaleClaimLeaseForTest(t, store, bead.ClaimLeaseRecord{
		BeadID:    beadID,
		Owner:     "worker-test",
		Machine:   machine,
		StartedAt: time.Now().UTC().Add(-2 * time.Minute),
		UpdatedAt: time.Now().UTC(),
		PID:       pid,
	})

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	// Intentionally omit any manager handle: disk-only representation.

	supervisor := NewWorkerSupervisor(m)
	desired := DefaultWorkerDesiredState(root)
	desired.DesiredCount = 1
	desired.Restart.Enabled = false
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	now := time.Now().UTC()
	workerID := "worker-20260724T000000-diskonly"
	attemptID := workerID + "-a1"
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	diskRec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StartedAt:   now.Add(-5 * time.Minute),
		PID:         pid,
		PGID:        pgid,
		CurrentBead: beadID,
		CurrentAttempt: &CurrentAttemptInfo{
			AttemptID: attemptID,
			BeadID:    beadID,
			Phase:     "running",
			StartedAt: now.Add(-5 * time.Minute),
		},
	}
	require.NoError(t, m.writeRecord(dir, diskRec))

	// Force workerRecordLive false via an expired run-state while the claim
	// lease remains fresh — models the post-restart secondary-signal gap
	// without overwriting the worker status.json (which doubles as the
	// liveness sidecar path).
	require.NoError(t, agent.WriteRunState(root, agent.RunState{
		BeadID:    beadID,
		AttemptID: attemptID,
		StartedAt: now.Add(-time.Hour),
		ExpiresAt: now.Add(-time.Minute),
	}))
	require.False(t, supervisor.workerRecordLive(diskRec, now),
		"fixture must be classified non-live by secondary signals")
	require.False(t, m.hasWorkerHandle(workerID), "must remain disk-only (no manager handle)")

	var logBuf bytes.Buffer
	restoreLog := redirectStdLogger(&logBuf)
	defer restoreLog()

	for i := 0; i < 3; i++ {
		require.NoError(t, supervisor.ReconcileAt(now.Add(time.Duration(i)*time.Second)))
	}

	// AC1: process remains alive across three reconcile ticks.
	assert.True(t, processAlive(pid), "live disk-only managed PID must survive ReconcileAt ticks")

	rec, err := m.readRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, "running", rec.State, "preserved attempt must not be marked stopped")
	assert.Equal(t, "running", rec.Status)
	assert.True(t, rec.FinishedAt.IsZero())

	// AC2: claim stays in_progress; no replacement duplicate started.
	b, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusInProgress, b.Status, "fresh claim must remain in_progress")

	recs, err := m.List()
	require.NoError(t, err)
	runningWork := 0
	for _, r := range recs {
		if r.Kind == "work" && r.ProjectRoot == root && (r.State == "running" || r.State == "stopping") {
			runningWork++
		}
	}
	assert.Equal(t, 1, runningWork, "supervisor must not start a replacement duplicate for the live attempt")
	assert.False(t, m.hasWorkerHandle(workerID), "preserve must not invent an in-memory handle")

	// AC3: structured supervisor/lifecycle diagnostic with worker, PID, bead, reason.
	var preserve *WorkerLifecycleEvent
	for i := range rec.Lifecycle {
		if rec.Lifecycle[i].Action == "preserve" {
			preserve = &rec.Lifecycle[i]
			break
		}
	}
	require.NotNil(t, preserve, "must emit a lifecycle preserve diagnostic")
	assert.Equal(t, "server-workers", preserve.Actor)
	assert.Equal(t, beadID, preserve.BeadID)
	assert.Contains(t, preserve.Detail, workerID)
	assert.Contains(t, preserve.Detail, fmt.Sprintf("pid=%d", pid))
	assert.Contains(t, preserve.Detail, beadID)
	assert.Contains(t, preserve.Detail, "reason=fresh-claim-or-heartbeat")

	diag := logBuf.String()
	assert.Contains(t, diag, "preserve live disk-record worker")
	assert.Contains(t, diag, workerID)
	assert.Contains(t, diag, fmt.Sprintf("pid=%d", pid))
	assert.Contains(t, diag, beadID)
	assert.Contains(t, diag, "reason=fresh-claim-or-heartbeat")
}

func installBlockingWorkerFactory(m *WorkerManager) {
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				<-ctx.Done()
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: ctx.Err().Error(),
				}, ctx.Err()
			}),
		}
	}
}

func seedTerminalFailedWorker(t *testing.T, m *WorkerManager, root, workerID string) {
	t.Helper()
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	now := time.Now().UTC()
	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "failed",
		Status:      "failed",
		ProjectRoot: root,
		StartedAt:   now.Add(-2 * time.Minute),
		FinishedAt:  now,
		LastError:   "injected test failure",
	}
	require.NoError(t, m.writeRecord(dir, rec))
}

func seedTerminalOperatorAttentionWorker(t *testing.T, m *WorkerManager, root, workerID string, terminalAt time.Time) {
	t.Helper()
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "exited",
		Status:      "operator_attention",
		ReapReason:  "operator_attention",
		ProjectRoot: root,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
	}
	require.NoError(t, m.writeRecord(dir, rec))
}

func seedTerminalDirtyRootWorker(t *testing.T, m *WorkerManager, root, workerID string, terminalAt time.Time) {
	t.Helper()
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "exited",
		Status:      "dirty_root",
		ReapReason:  "dirty_root",
		ProjectRoot: root,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
	}
	require.NoError(t, m.writeRecord(dir, rec))
}

func writeDesiredStateForTest(supervisor *WorkerSupervisor, state WorkerDesiredState) error {
	state.ApplyDefaults(supervisor.manager.projectRoot)
	if err := state.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(supervisor.manager.rootDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(supervisor.desiredStatePath(), append(data, '\n'), 0o644)
}

func runningManagedWorkerCount(t *testing.T, m *WorkerManager, projectRoot string) int {
	t.Helper()
	recs := runningManagedWorkers(t, m, projectRoot)
	return len(recs)
}

func runningManagedWorkers(t *testing.T, m *WorkerManager, projectRoot string) []WorkerRecord {
	t.Helper()
	recs, err := m.List()
	require.NoError(t, err)

	out := make([]WorkerRecord, 0, len(recs))
	for _, rec := range recs {
		if rec.Kind != "work" || rec.ProjectRoot != projectRoot || rec.State != "running" {
			continue
		}
		out = append(out, rec)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

func stopProjectWorkers(t *testing.T, m *WorkerManager, projectRoot string) {
	t.Helper()
	_ = projectRoot

	// Shutdown stops every live worker AND waits (waitForStoppedRecord) for each
	// runWorker goroutine to persist its terminal record, so t.TempDir cleanup
	// cannot race a late status.json write. Waiting on
	// runningManagedWorkerCount()==0 is not sufficient: Stop() flips the record
	// to "stopping" immediately, satisfying that predicate while the goroutine
	// is still finalizing — which surfaces as
	// "TempDir RemoveAll cleanup: directory not empty" under load.
	require.NoError(t, m.Shutdown())
}
