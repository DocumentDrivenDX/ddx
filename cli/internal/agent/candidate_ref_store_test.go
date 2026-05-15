package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestGitRepo creates a temp git repo with one commit and returns the
// repo path and the HEAD sha.
func initTestGitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644))
	run("add", "README.md")
	run("commit", "-m", "initial")

	rawRev, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	return dir, strings.TrimSpace(string(rawRev))
}

// gitRevParse resolves ref in dir, failing the test if not found.
func gitRevParse(t *testing.T, dir, ref string) (string, error) {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// TestCandidateRefStore_ProjectRootReachable verifies that a candidate ref
// pinned via GitCandidateRefStore resolves from the project root via
// `git rev-parse <ref>`, including after the temp worktree directory is removed.
func TestCandidateRefStore_ProjectRootReachable(t *testing.T) {
	projectRoot, rev := initTestGitRepo(t)

	// Simulate a temp worktree directory that will be removed after pinning.
	tmpWT := t.TempDir()

	store := &GitCandidateRefStore{}
	ref, err := store.PinCandidateRef(projectRoot, "attempt-abc", 0, rev)
	require.NoError(t, err)
	assert.Equal(t, "refs/ddx/iterations/attempt-abc/0", ref)

	// Ref resolves in project root before worktree removal.
	got, err := gitRevParse(t, projectRoot, ref)
	require.NoError(t, err)
	assert.Equal(t, rev, got)

	// Simulate worktree cleanup.
	require.NoError(t, os.RemoveAll(tmpWT))

	// Ref is still reachable from project root after worktree is gone.
	got2, err := gitRevParse(t, projectRoot, ref)
	require.NoError(t, err)
	assert.Equal(t, rev, got2, "candidate ref must survive worktree removal")
}

// TestCandidateRefStore_MetadataRecorded verifies that AttemptCycleCoordinator
// populates CandidateRef, CycleIndex, AttemptID, BaseRev, and ResultRev on the
// landed report (the data that flows into result.json / ExecuteBeadResult).
func TestCandidateRefStore_MetadataRecorded(t *testing.T) {
	projectRoot, rev := initTestGitRepo(t)

	const attemptID = "attempt-meta-001"
	const baseRev = "base000000000000"
	const cycleIndex = 0

	// Create a second commit so baseRev and ResultRev are different strings.
	// We use the actual HEAD as ResultRev (the candidate).
	resultRev := rev

	var landedCandidate CandidateResult
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: attemptID,
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   baseRev,
					ResultRev: resultRev,
				},
				CycleIndex: cycleIndex,
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			landedCandidate = candidate
			return candidate.Report, nil
		}),
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
	}

	result, err := coord.Run(context.Background(), "ddx-meta-bead")
	require.NoError(t, err)
	assert.True(t, result.Landed)

	// Lander must have received candidate with CandidateRef set.
	assert.NotEmpty(t, landedCandidate.Report.CandidateRef, "CandidateRef must be set on candidate before lander runs")
	assert.Equal(t, cycleIndex, landedCandidate.Report.CycleIndex)

	// Landed report (what goes into result.json) must carry all cycle fields.
	assert.NotEmpty(t, result.Report.CandidateRef, "CandidateRef must be in landed report")
	assert.Equal(t, cycleIndex, result.Report.CycleIndex)
	assert.Equal(t, attemptID, result.Report.AttemptID)
	assert.Equal(t, baseRev, result.Report.BaseRev)
	assert.Equal(t, resultRev, result.Report.ResultRev)
}

// inMemoryEventAppender is a test double for BeadEventAppender.
type inMemoryEventAppender struct {
	events []bead.BeadEvent
}

func (a *inMemoryEventAppender) AppendEvent(_ string, event bead.BeadEvent) error {
	a.events = append(a.events, event)
	return nil
}

// TestCandidateRefStore_EventRecorded verifies that the coordinator emits a
// candidate_cycle_pinned bead event containing candidate_ref, cycle_index,
// base_rev, result_rev, and attempt_id when BeadEvents is set.
func TestCandidateRefStore_EventRecorded(t *testing.T) {
	projectRoot, rev := initTestGitRepo(t)

	const attemptID = "attempt-evt-002"
	const baseRev = "base111111111111"
	const cycleIndex = 1

	events := &inMemoryEventAppender{}
	coord := &AttemptCycleCoordinator{
		Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    beadID,
					AttemptID: attemptID,
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   baseRev,
					ResultRev: rev,
				},
				CycleIndex: cycleIndex,
			}, nil
		}),
		Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
			return candidate.Report, nil
		}),
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
		BeadEvents:  events,
	}

	_, err := coord.Run(context.Background(), "ddx-evt-bead")
	require.NoError(t, err)

	require.Len(t, events.events, 1, "exactly one candidate_cycle_pinned event expected")
	evt := events.events[0]
	assert.Equal(t, "candidate_cycle_pinned", evt.Kind)

	var body CandidateCycleEventBody
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &body))
	assert.NotEmpty(t, body.CandidateRef)
	assert.Equal(t, cycleIndex, body.CycleIndex)
	assert.Equal(t, attemptID, body.AttemptID)
	assert.Equal(t, baseRev, body.BaseRev)
	assert.Equal(t, rev, body.ResultRev)
}

func TestAttemptCycleCoordinator_NoLanderRetainsCandidateRef(t *testing.T) {
	projectRoot, rev := initTestGitRepo(t)

	coord := &AttemptCycleCoordinator{
		Pass: staticCandidateResultPass{
			candidate: CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    "ddx-no-lander",
					AttemptID: "attempt-no-lander-001",
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   "base-no-lander",
					ResultRev: rev,
				},
			},
		},
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
	}

	result, err := coord.Run(context.Background(), "ddx-no-lander")
	require.NoError(t, err)
	assert.False(t, result.Landed, "worker-side candidate finalization must not report a landed branch")
	require.NotEmpty(t, result.Report.CandidateRef)

	got, err := gitRevParse(t, projectRoot, result.Report.CandidateRef)
	require.NoError(t, err)
	assert.Equal(t, rev, got, "candidate ref must be retained when the coordinator finalizes without a lander")
}

// TestCandidateRefStore_RetentionPolicy verifies:
//   - Approved-landed (success) candidates: temporary ref is cleaned up.
//   - Preserved/conflicted/parked candidates: ref is retained for operator use.
func TestCandidateRefStore_RetentionPolicy(t *testing.T) {
	// ShouldRetainCandidateRef policy: success → clean up; everything else → retain.
	assert.False(t, ShouldRetainCandidateRef(ExecuteBeadStatusSuccess),
		"success outcome must not retain candidate ref")
	for _, retainStatus := range []string{
		ExecuteBeadStatusLandConflict,
		ExecuteBeadStatusPreservedNeedsReview,
		ExecuteBeadStatusLandConflictOperatorRequired,
		ExecuteBeadStatusLandConflictUnresolvable,
		"manual",
		"parked",
		ExecuteBeadStatusExecutionFailed,
	} {
		assert.True(t, ShouldRetainCandidateRef(retainStatus),
			"status %q must retain candidate ref", retainStatus)
	}

	// Integration: coordinator cleans up the ref after a successful land.
	t.Run("landed_ref_cleaned", func(t *testing.T) {
		projectRoot, rev := initTestGitRepo(t)
		store := &GitCandidateRefStore{}

		coord := &AttemptCycleCoordinator{
			Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
				return CandidateResult{
					Report: ExecuteBeadReport{
						BeadID:    beadID,
						AttemptID: "attempt-land-001",
						Status:    ExecuteBeadStatusSuccess,
						ResultRev: rev,
					},
				}, nil
			}),
			Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
				// Return success status — signals a clean land.
				return candidate.Report, nil
			}),
			RefStore:    store,
			ProjectRoot: projectRoot,
		}

		result, err := coord.Run(context.Background(), "ddx-retention-bead")
		require.NoError(t, err)
		require.True(t, result.Landed)

		pinnedRef := result.Report.CandidateRef
		require.NotEmpty(t, pinnedRef)

		// After a successful land the coordinator unpins the ref.
		_, resolveErr := gitRevParse(t, projectRoot, pinnedRef)
		assert.Error(t, resolveErr, "candidate ref must be cleaned up after successful landing")
	})

	// Integration: coordinator retains the ref when landing returns a non-success status.
	t.Run("preserved_ref_retained", func(t *testing.T) {
		projectRoot, rev := initTestGitRepo(t)
		store := &GitCandidateRefStore{}

		coord := &AttemptCycleCoordinator{
			Pass: implementationPassFunc(func(_ context.Context, beadID string) (CandidateResult, error) {
				return CandidateResult{
					Report: ExecuteBeadReport{
						BeadID:    beadID,
						AttemptID: "attempt-preserve-001",
						Status:    ExecuteBeadStatusSuccess,
						ResultRev: rev,
					},
				}, nil
			}),
			Lander: candidateLanderFunc(func(_ context.Context, candidate CandidateResult) (ExecuteBeadReport, error) {
				// Simulate a preserved (conflict) outcome.
				report := candidate.Report
				report.Status = ExecuteBeadStatusLandConflict
				return report, nil
			}),
			RefStore:    store,
			ProjectRoot: projectRoot,
		}

		result, err := coord.Run(context.Background(), "ddx-preserve-bead")
		require.NoError(t, err)
		require.True(t, result.Landed)

		pinnedRef := result.Report.CandidateRef
		require.NotEmpty(t, pinnedRef)

		// Ref must still be reachable from project root.
		got, resolveErr := gitRevParse(t, projectRoot, pinnedRef)
		require.NoError(t, resolveErr, "candidate ref must be retained for preserved/conflicted outcome")
		assert.Equal(t, rev, got)
	})
}
