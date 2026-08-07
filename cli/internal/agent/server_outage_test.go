package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerOutageTracker_DistinctNoViableProviderTripsAfterThreshold(t *testing.T) {
	tracker := newServerOutageTracker(10*time.Minute, 3, 30*time.Second)
	now := time.Now().UTC()

	for i, beadID := range []string{"ddx-a1", "ddx-b2", "ddx-c3"} {
		activated, reason := tracker.Record(ExecuteBeadReport{
			BeadID: beadID,
			Status: ExecuteBeadStatusExecutionFailed,
			Detail: "ResolveRoute: no viable routing candidate: 3 candidates rejected",
		}, beadID, now.Add(time.Duration(i)*time.Minute))
		if i < 2 {
			assert.False(t, activated, "threshold must not trip early")
			assert.Empty(t, reason)
			assert.False(t, tracker.Active())
			continue
		}
		assert.True(t, activated, "third distinct bead must trip server outage mode")
		assert.Equal(t, FailureModeServerUnavailable, reason)
		assert.True(t, tracker.Active())
		assert.Equal(t, FailureModeServerUnavailable, tracker.Reason())
	}
}

func TestServerOutageTracker_DirectTransportFailureIsServerLevel(t *testing.T) {
	for _, provider := range []string{"", "opaque-provider-a", "opaque-provider-b"} {
		t.Run(provider, func(t *testing.T) {
			tracker := newServerOutageTracker(10*time.Minute, 3, 30*time.Second)
			report := ExecuteBeadReport{
				BeadID:   "ddx-transport",
				Status:   ExecuteBeadStatusExecutionFailed,
				Provider: provider,
				Detail:   "http: TLS handshake error from 127.0.0.1:7743: remote error: tls: bad certificate",
			}

			assert.Equal(t, FailureModeServerUnavailable, ClassifyFailureMode(report.Status, 1, report.Detail))
			activated, reason := tracker.Record(report, report.BeadID, time.Now().UTC())
			assert.True(t, activated, "concrete returned provider must not change outage control")
			assert.Equal(t, FailureModeServerUnavailable, reason)
			assert.True(t, tracker.Active())
		})
	}
}

func TestExecuteBeadWorker_ServerOutagePausesAndResumesWatchQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	beadIDs := []string{"ddx-server-a", "ddx-server-b", "ddx-server-c", "ddx-server-d"}
	for i, id := range beadIDs {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:       id,
			Title:    "server outage bead " + id,
			Priority: i,
		}))
	}

	var (
		attempts   atomic.Int32
		probeCalls atomic.Int32
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			attempt := attempts.Add(1)
			if attempt >= 4 {
				cancel()
			}
			if attempt < 4 {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusExecutionFailed,
					Detail:    "ResolveRoute: no viable routing candidate: 3 candidates rejected",
					SessionID: "sess-server-outage",
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-server-outage",
				BaseRev:   "aaaa1111",
				ResultRev: "bbbb2222",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                      executeloop.ModeWatch,
		IdleInterval:              time.Millisecond,
		ServerHealthProbeInterval: 5 * time.Millisecond,
		ServerHealthProbe: func(context.Context) (bool, error) {
			switch n := probeCalls.Add(1); n {
			case 1, 2:
				return false, nil
			default:
				return true, nil
			}
		},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, int32(4), attempts.Load(), "watch loop must pause after server outage and resume only after probe recovery")
	assert.Equal(t, int32(3), probeCalls.Load(), "probe should be polled until recovery, then stop")
	require.Len(t, result.Results, 4)
	assert.Equal(t, FailureModeNoViableProvider, result.Results[0].OutcomeReason)
	assert.Equal(t, FailureModeNoViableProvider, result.Results[1].OutcomeReason)
	assert.Equal(t, FailureModeNoViableProvider, result.Results[2].OutcomeReason)
	assert.Equal(t, ExecuteBeadStatusSuccess, result.Results[3].Status)

	for _, id := range beadIDs[:3] {
		got, getErr := store.Get(context.Background(), id)
		require.NoError(t, getErr)
		assert.Equal(t, bead.StatusOpen, got.Status, "server outage must leave %s reclaimable", id)
		assert.Empty(t, got.Owner)
		assert.Empty(t, got.Extra["work-retry-after"], "server outage must not set a per-bead cooldown on %s", id)
	}
}

func TestExecuteBeadLoopReleasesServerUnavailableWhenProbeHealthy(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	beadIDs := []string{"ddx-server-recover-a", "ddx-server-recover-b"}
	for i, id := range beadIDs {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:       id,
			Title:    "server recovery bead " + id,
			Priority: i,
		}))
	}

	projectRoot := store.Dir
	sessionID := "sess-server-recover"
	var (
		attempts   atomic.Int32
		probeCalls atomic.Int32
		started    = make(chan struct{})
		release    = make(chan struct{})
		done       = make(chan error, 1)
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			attempt := attempts.Add(1)
			switch attempt {
			case 1:
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusExecutionFailed,
					Detail:    "ResolveRoute: no viable routing candidate: 3 candidates rejected",
					SessionID: sessionID,
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			case 2:
				close(started)
				<-release
				cancel()
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: sessionID,
					BaseRev:   "aaaa1111",
					ResultRev: "bbbb2222",
				}, nil
			default:
				t.Fatalf("unexpected extra attempt for bead %s", beadID)
				return ExecuteBeadReport{}, nil
			}
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	go func() {
		_, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:                      executeloop.ModeWatch,
			IdleInterval:              time.Millisecond,
			ProjectRoot:               projectRoot,
			SessionID:                 sessionID,
			WorkerID:                  "worker-server-recover",
			ServerFailureThreshold:    1,
			ServerHealthProbeInterval: 5 * time.Millisecond,
			ServerHealthProbe: func(context.Context) (bool, error) {
				switch probeCalls.Add(1) {
				case 1:
					return false, nil
				default:
					return true, nil
				}
			},
		})
		done <- err
	}()

	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "worker never resumed onto the next ready bead")

	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		return rec.CurrentBead == beadIDs[1] && rec.Phase != serverUnavailableStatePhase
	}, time.Second, 5*time.Millisecond, "server.unavailable must clear after a healthy probe")

	close(release)
	require.ErrorIs(t, <-done, context.Canceled)
	assert.GreaterOrEqual(t, probeCalls.Load(), int32(2), "probe must have observed recovery")
	assert.GreaterOrEqual(t, attempts.Load(), int32(2), "worker must claim the next ready bead after recovery")
}
