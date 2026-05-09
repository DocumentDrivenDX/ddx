package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDebugWhichBeadGetsExecuted(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	seedWatchIdleQueue(t, store)

	var executedBeads []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executedBeads = append(executedBeads, beadID)
			t.Logf("EXECUTED: %s", beadID)
			return ExecuteBeadReport{Status: ExecuteBeadStatusNoChanges}, nil
		}),
		Now: fixedWatchStdoutTime,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, _ := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	t.Logf("Executed beads: %v", executedBeads)
	if result != nil && result.QueueSnapshot != nil {
		t.Logf("QueueSnapshot.HumanReviewBlockerCount: %d", result.QueueSnapshot.HumanReviewBlockerCount)
		t.Logf("QueueSnapshot.ProposedOperatorAttentionCount: %d", result.QueueSnapshot.ProposedOperatorAttentionCount)
		t.Logf("QueueSnapshot.HumanReviewBlockedTotal: %d", result.QueueSnapshot.HumanReviewBlockedTotal)
		t.Logf("QueueSnapshot.HumanReviewBlockers: %v", result.QueueSnapshot.HumanReviewBlockers)
	}
}
