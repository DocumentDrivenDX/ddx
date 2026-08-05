package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlreadySatisfied_MechanicalDocsAC_DespitePackageGateNoise(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-int-0001"
	require.NoError(t, store.Update(context.Background(), beadID, func(b *bead.Bead) {
		b.Acceptance = "1. docs/alpha.md exists\n2. `ddx-06cbaa90` is cited in docs/beta.md"
	}))

	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "docs", "alpha.md"), []byte("alpha doc\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "docs", "beta.md"), []byte("cite: ddx-06cbaa90\n"), 0o644))
	runGitInteg(t, projectRoot, "add", "docs/alpha.md", "docs/beta.md")
	runGitInteg(t, projectRoot, "commit", "-m", "docs: add cite anchors")
	headSHA := strings.TrimSpace(runGitInteg(t, projectRoot, "rev-parse", "HEAD"))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusNoChanges,
				SessionID: "sess-mechanical",
				BaseRev:   headSHA,
				ResultRev: headSHA,
				Detail:    "unrelated package gate failed: TestX red",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true, ProjectRoot: projectRoot})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status)
	assert.Contains(t, result.Results[0].Detail, "mechanical AC satisfied")
	assert.Contains(t, result.Results[0].Detail, "docs/alpha.md exists")
	assert.Contains(t, result.Results[0].Detail, "ddx-06cbaa90")

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestAlreadySatisfied_ImplementationBeadWithTestFooStillRequiresGreen(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-int-0001"
	require.NoError(t, store.Update(context.Background(), beadID, func(b *bead.Bead) {
		b.Acceptance = "1. TestFoo passes"
	}))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				SessionID:          "sess-testfoo",
				BaseRev:            "base",
				ResultRev:          "base",
				NoChangesRationale: "status: open\nreason: package gate red: unrelated TestFoo",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true, ProjectRoot: projectRoot})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.Results[0].Status)

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}
