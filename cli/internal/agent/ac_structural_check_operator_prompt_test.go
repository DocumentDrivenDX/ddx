package agent

import (
	"context"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStructuralACSkippedForOperatorPromptBeads validates Story 15
// §Implementation #1: an operator-prompt bead carries an auto-generated AC
// stub, so the structural verifier must NOT block already_satisfied even
// when the prompt body would otherwise parse out structural claims (e.g.
// names a Test* function that the rationale does not cite).
//
// A baseline test in ac_structural_check_test.go covers the symmetric path
// (issue_type=task, structural claim refused). This test covers the
// operator-prompt skip.
func TestStructuralACSkippedForOperatorPromptBeads(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Operator-prompt bead. Acceptance is the Story 15 default stub, but the
	// description includes a Test* token; ParseACClaims would normally read
	// claims out of the acceptance text. We deliberately put a structural
	// claim in the acceptance text to confirm the issue_type gate (not the
	// content of acceptance) is what skips the verifier.
	b := bead.NewOperatorPromptBead("please run TestSomethingThatDoesNotExist", 2)
	b.Acceptance = "go test ./... -run TestSomethingThatDoesNotExist passes"
	// Operator-prompt beads start in `proposed`; flip to open so the
	// execute-loop will pick the bead up in this Once run.
	b.Status = bead.StatusOpen
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             id,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "verification_command: true\noutput: ok",
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			return 0, "", nil
		},
	}

	rcfg := loopConfigForACTest(t)
	runtime := ExecuteBeadLoopRuntime{Once: true, ProjectRoot: t.TempDir()}

	result, err := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status,
		"operator-prompt bead must skip structural AC verifier and close as already_satisfied")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}
