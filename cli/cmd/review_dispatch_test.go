package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewDispatchDoesNotLoadProfileSnapshot proves reviewer dispatch uses
// the already-resolved primary config and never consults Fizeau's catalogs.
// Catalog failures are armed so any accidental query fails the review.
func TestReviewDispatchDoesNotLoadProfileSnapshot(t *testing.T) {
	projectRoot := t.TempDir()
	out, err := exec.Command("git", "init", projectRoot).CombinedOutput()
	require.NoError(t, err, string(out))

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:         "ddx-review-dispatch",
		Title:      "Review dispatch test",
		Acceptance: "1. Review the change",
	}))
	out, err = exec.Command("git", "-C", projectRoot,
		"-c", "user.name=Test", "-c", "user.email=t@example.com",
		"commit", "--allow-empty", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))
	rawHead, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(rawHead))

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	svc := &executeCapturingStub{
		listModelsErr:   fmt.Errorf("model catalog must not be queried"),
		listPoliciesErr: fmt.Errorf("policy catalog must not be queried"),
		executeFn: func(agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
			ch := make(chan agentlib.ServiceEvent, 1)
			ch <- agentlib.ServiceEvent{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"schema_version\":1,\"verdict\":\"APPROVE\",\"summary\":\"ok\"}"}`),
			}
			close(ch)
			return ch, nil
		},
	}
	reviewer := &agent.DefaultBeadReviewer{
		ProjectRoot:           projectRoot,
		BeadStore:             store,
		Service:               svc,
		PrimaryConfigSnapshot: &rcfg,
	}

	res, err := reviewer.ReviewBead(context.Background(), "ddx-review-dispatch", head, agent.ImplementerRouting{ActualPower: 70})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, svc.executeCalled)
	assert.Equal(t, 71, svc.lastReq.MinPower)
	assert.Zero(t, svc.modelQueries)
	assert.Zero(t, svc.policyQueries)
}
