package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDrainQueueHistoricalConfigDoesNotFanOut is the AC #10 regression guard
// for ddx-fdd3ea36 / ddx-dd4c423d.
//
// Guards against reintroduction of the "19-burn" fan-out pattern: an empty
// dispatch spec must drive exactly one executor invocation, never a per-tier
// iteration. The deprecated profile_ladders / model_overrides fields are gone;
// the default execute-loop path resolves to a single ResolveRoute call.
//
// gemini-binary handling: this test stubs the executor at the loop
// boundary so no harness binary is ever shelled out.
func TestDrainQueueHistoricalConfigDoesNotFanOut(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ddx"), 0o755))
	cfgYAML := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
agent:
  endpoints:
    - type: lmstudio
      host: "127.0.0.1"
      port: 1234
    - type: omlx
      host: "127.0.0.1"
      port: 8000
    - type: lmstudio
      host: "127.0.0.1"
      port: 1235
    - type: lmstudio
      host: "127.0.0.1"
      port: 1236
`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfgYAML), 0o644))

	// LoadAndResolve with empty overrides except Assignee (the empty-spec case).
	rcfg, err := config.LoadAndResolve(root, config.CLIOverrides{Assignee: "worker"})
	require.NoError(t, err, "config must load cleanly")

	// Sanity: the resolved spec on the default path carries no harness,
	// no model, no profile — the synthesis surfaces that previously drove
	// the 19-burn must all be empty.
	assert.Empty(t, rcfg.Harness(), "default path must not synthesise a harness")
	assert.Empty(t, rcfg.Model(), "default path must not synthesise a model from model_overrides")
	assert.Empty(t, rcfg.Profile(), "default path must not synthesise a profile")
	assert.Empty(t, rcfg.MinTier(), "default path must not synthesise MinTier")
	assert.Empty(t, rcfg.MaxTier(), "default path must not synthesise MaxTier")

	// 3. Single ready bead in a fresh store. We use one bead so that
	//    Once:true draining ends after exactly one queue pass.
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{ID: "ddx-drain-regression", Title: "Drain regression bead", Priority: 0}
	require.NoError(t, store.Create(target))

	// 4. Counting executor stub. Returns success on the first call so the
	//    loop closes the bead cleanly. Any reintroduction of tier-ladder
	//    fan-out into the default loop would drive >1 invocation here.
	var execCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			n := atomic.AddInt32(&execCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "stubbed drain attempt",
				SessionID: "sess-drain",
				ResultRev: "deadbeef",
				// Fail loud if anything calls us a second time within a
				// single bead — that IS the historical fan-out.
				CostUSD:    float64(n),
				DurationMS: int64(n),
			}, nil
		}),
	}

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)

	// 5. Core regression assertion: at most ONE attempt, never 19.
	got := atomic.LoadInt32(&execCount)
	assert.LessOrEqualf(t, got, int32(1),
		"executor must be invoked at most once per bead on the default drain path; got %d. "+
			"A value >1 (historically 19) means tier-ladder fan-out has been reintroduced.",
		got)
	assert.Equal(t, 1, result.Attempts, "loop should report exactly one attempt for one ready bead")
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	// Bead closed cleanly — drain ended in success, not in 19 burned attempts.
	final, err := store.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, final.Status)
}
