package agent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/require"
)

type countingOrphanHarnessScanner struct {
	calls atomic.Int32
}

func (s *countingOrphanHarnessScanner) Scan(context.Context) ([]orphanHarnessProcess, error) {
	s.calls.Add(1)
	return nil, nil
}

func TestExecuteLoop_StartupOrphanReaperUsesInjectedScanner(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	scanner := &countingOrphanHarnessScanner{}
	originalFactory := defaultOrphanHarnessProcessScanner
	defaultOrphanHarnessProcessScanner = func() orphanHarnessProcessScanner {
		t.Fatal("startup reaper fell through to the default scanner factory")
		return nil
	}
	t.Cleanup(func() {
		defaultOrphanHarnessProcessScanner = originalFactory
	})

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run while exercising startup orphan scanner injection")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-startup"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		ProjectRoot:                 t.TempDir(),
		Once:                        true,
		OrphanHarnessProcessScanner: scanner,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), scanner.calls.Load())
}

func TestExecutionLoop_DefaultFixturesUseHermeticOrphanScanner(t *testing.T) {
	scanner := defaultOrphanHarnessProcessScanner()
	require.IsType(t, hermeticOrphanHarnessScanner{}, scanner)

	processes, err := scanner.Scan(context.Background())
	require.NoError(t, err)
	require.Empty(t, processes)
}
