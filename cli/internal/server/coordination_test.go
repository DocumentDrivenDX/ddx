package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinationContract_HTTP runs the same behavioral contract as
// TestCoordinationContract_Local against a real httptest DDx server, real
// bead store, and real git repository (ddx-b45dcbd4 / ADR-022).
//
// Covers claim contention, tracker transition + already_applied, and landing
// + already_applied. Production handlers route through the bead store and
// WorkerManager.LandCoordinators — no call-recording mocks.
func TestCoordinationContract_HTTP(t *testing.T) {
	projectRoot := setupCoordinationProject(t)
	srv := New(":0", projectRoot)
	t.Cleanup(func() {
		if srv.workers != nil && srv.workers.LandCoordinators != nil {
			srv.workers.LandCoordinators.StopAll()
		}
	})
	proj := srv.state.RegisterProject(projectRoot)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	// --- Claim contention (mirrors TestCoordinationContract_LocalClaimContention)
	const claimBead = "ddx-http-claim-contention-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    claimBead,
		Title: "http coordination claim contention fixture",
	}))
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed claim contention fixture")

	first := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-a",
		Operation:      coordOpClaim,
		IdempotencyKey: "claim-key-worker-a-1",
		Payload: mustJSON(t, claimPayload{
			BeadID:   claimBead,
			Assignee: "worker-a",
		}),
	})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstClaim coordinationMutationResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstClaim))
	assert.Equal(t, string(coordination.OutcomeApplied), firstClaim.Outcome)
	assert.Equal(t, claimBead, firstClaim.BeadID)
	assert.Equal(t, "worker-a", firstClaim.Owner)
	assert.NotEmpty(t, firstClaim.Version)

	second := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-b",
		Operation:      coordOpClaim,
		IdempotencyKey: "claim-key-worker-b-1",
		Payload: mustJSON(t, claimPayload{
			BeadID:   claimBead,
			Assignee: "worker-b",
		}),
	})
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	var secondClaim coordinationMutationResponse
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondClaim))
	assert.Equal(t, string(coordination.OutcomeConflict), secondClaim.Outcome)
	assert.Equal(t, coordination.ReasonAlreadyClaimed, secondClaim.Reason)
	assert.Equal(t, "worker-a", secondClaim.Owner)

	got, err := store.Get(context.Background(), claimBead)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusInProgress, got.Status)
	assert.Equal(t, "worker-a", got.Owner)

	// Production path uses LandCoordinators (not a parallel land table).
	require.NotNil(t, srv.workers)
	require.NotNil(t, srv.workers.LandCoordinators)
	landCoord := srv.landCoordinatorFor(projectRoot)
	require.NotNil(t, landCoord, "handlers must resolve LandCoordinator via WorkerManager")

	// --- Tracker transition + already_applied
	const transitionBead = "ddx-http-tracker-transition-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    transitionBead,
		Title: "http coordination tracker transition fixture",
	}))
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed tracker transition fixture")

	claimSetup := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-transition",
		Operation:      coordOpClaim,
		IdempotencyKey: "claim-key-transition-setup",
		Payload: mustJSON(t, claimPayload{
			BeadID:   transitionBead,
			Assignee: "worker-transition",
		}),
	})
	require.Equal(t, http.StatusOK, claimSetup.Code, claimSetup.Body.String())

	const transitionKey = "transition-key-in-progress-to-open"
	trans1 := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-transition",
		Operation:      coordOpTrackerTransition,
		IdempotencyKey: transitionKey,
		Payload: mustJSON(t, transitionPayload{
			BeadID:   transitionBead,
			ToStatus: bead.StatusOpen,
			Reason:   "release after claim",
			Actor:    "worker-transition",
		}),
	})
	require.Equal(t, http.StatusOK, trans1.Code, trans1.Body.String())
	var firstTrans coordinationMutationResponse
	require.NoError(t, json.Unmarshal(trans1.Body.Bytes(), &firstTrans))
	assert.Equal(t, string(coordination.OutcomeApplied), firstTrans.Outcome)
	assert.Equal(t, bead.StatusInProgress, firstTrans.FromStatus)
	assert.Equal(t, bead.StatusOpen, firstTrans.ToStatus)

	transReplay := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-transition",
		Operation:      coordOpTrackerTransition,
		IdempotencyKey: transitionKey,
		Payload: mustJSON(t, transitionPayload{
			BeadID:   transitionBead,
			ToStatus: bead.StatusOpen,
			Reason:   "release after claim",
			Actor:    "worker-transition",
		}),
	})
	require.Equal(t, http.StatusOK, transReplay.Code, transReplay.Body.String())
	var replayTrans coordinationMutationResponse
	require.NoError(t, json.Unmarshal(transReplay.Body.Bytes(), &replayTrans))
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), replayTrans.Outcome)
	assert.Equal(t, bead.StatusOpen, replayTrans.ToStatus)

	got, err = store.Get(context.Background(), transitionBead)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusOpen, got.Status)

	// --- Landing + already_applied (mirrors TestCoordinationContract_LocalLandingIdempotency)
	const landBead = "ddx-http-land-idempotency-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    landBead,
		Title: "http coordination landing fixture",
	}))
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed land fixture bead")
	baseSHA := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-parse", "HEAD"))
	resultSHA := commitOnBaseCoord(t, projectRoot, baseSHA, "feature.txt", "landed-by-http-coordination\n", "feat: land via http coordination")

	const landKey = "land-key-http-idempotency-1"
	land1 := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-land",
		Operation:      coordOpLand,
		IdempotencyKey: landKey,
		Payload: mustJSON(t, landPayload{
			BeadID:       landBead,
			BaseRev:      baseSHA,
			ResultRev:    resultSHA,
			AttemptID:    "20260725T000000-land1",
			TargetBranch: "main",
			WorktreeDir:  projectRoot,
		}),
	})
	require.Equal(t, http.StatusOK, land1.Code, land1.Body.String())
	var firstLand coordinationMutationResponse
	require.NoError(t, json.Unmarshal(land1.Body.Bytes(), &firstLand))
	assert.Equal(t, string(coordination.OutcomeApplied), firstLand.Outcome, firstLand.Reason)
	assert.Equal(t, coordination.LandStatusLanded, firstLand.LandStatus)
	assert.Equal(t, resultSHA, firstLand.NewTip)
	assert.Equal(t, resultSHA, firstLand.Version)

	mainTip := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-parse", "refs/heads/main"))
	assert.Equal(t, resultSHA, mainTip)

	// LandCoordinator metrics should reflect the production submit path.
	metrics := landCoord.Metrics()
	assert.GreaterOrEqual(t, metrics.Landed, int64(1), "LandCoordinator must have processed the land")

	landReplay := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-land",
		Operation:      coordOpLand,
		IdempotencyKey: landKey,
		Payload: mustJSON(t, landPayload{
			BeadID:       landBead,
			BaseRev:      baseSHA,
			ResultRev:    resultSHA,
			AttemptID:    "20260725T000000-land1",
			TargetBranch: "main",
			WorktreeDir:  projectRoot,
		}),
	})
	require.Equal(t, http.StatusOK, landReplay.Code, landReplay.Body.String())
	var replayLand coordinationMutationResponse
	require.NoError(t, json.Unmarshal(landReplay.Body.Bytes(), &replayLand))
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), replayLand.Outcome)
	assert.Equal(t, coordination.LandStatusLanded, replayLand.LandStatus)
	assert.Equal(t, resultSHA, replayLand.NewTip)

	mainTipAfter := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-parse", "refs/heads/main"))
	assert.Equal(t, resultSHA, mainTipAfter, "idempotent land replay must not re-land")
	// LandCoordinator must not record a second land for the replay.
	metricsAfter := landCoord.Metrics()
	assert.Equal(t, metrics.Landed, metricsAfter.Landed, "already_applied must not re-submit to LandCoordinator")
}

// TestCoordinationMutation_UnknownResponseRetryIsAlreadyApplied proves retrying
// one idempotency key does not duplicate tracker events or landing commits.
func TestCoordinationMutation_UnknownResponseRetryIsAlreadyApplied(t *testing.T) {
	projectRoot := setupCoordinationProject(t)
	srv := New(":0", projectRoot)
	t.Cleanup(func() {
		if srv.workers != nil && srv.workers.LandCoordinators != nil {
			srv.workers.LandCoordinators.StopAll()
		}
	})
	proj := srv.state.RegisterProject(projectRoot)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-http-retry-claim-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "unknown-response retry fixture",
	}))
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed retry fixture")

	const key = "retry-claim-key-1"
	payload := mustJSON(t, claimPayload{BeadID: beadID, Assignee: "retry-worker"})

	first := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-retry",
		Operation:      coordOpClaim,
		IdempotencyKey: key,
		Payload:        payload,
	})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstResp coordinationMutationResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.Equal(t, string(coordination.OutcomeApplied), firstResp.Outcome)

	eventsAfterFirst, err := store.Events(beadID)
	require.NoError(t, err)
	eventCount := len(eventsAfterFirst)

	// Simulate unknown-response retry: same key, same payload.
	retry := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-retry",
		Operation:      coordOpClaim,
		IdempotencyKey: key,
		Payload:        payload,
	})
	require.Equal(t, http.StatusOK, retry.Code, retry.Body.String())
	var retryResp coordinationMutationResponse
	require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryResp))
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), retryResp.Outcome)
	assert.Equal(t, firstResp.Owner, retryResp.Owner)
	assert.Equal(t, firstResp.BeadID, retryResp.BeadID)

	eventsAfterRetry, err := store.Events(beadID)
	require.NoError(t, err)
	assert.Equal(t, eventCount, len(eventsAfterRetry), "retry must not append tracker events")

	// Landing commit non-duplication.
	const landBead = "ddx-http-retry-land-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    landBead,
		Title: "retry land fixture",
	}))
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed land retry bead")
	baseSHA := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-parse", "HEAD"))
	resultSHA := commitOnBaseCoord(t, projectRoot, baseSHA, "retry-land.txt", "retry-land\n", "feat: land retry fixture")

	const landKey = "retry-land-key-1"
	landPayloadRaw := mustJSON(t, landPayload{
		BeadID:       landBead,
		BaseRev:      baseSHA,
		ResultRev:    resultSHA,
		AttemptID:    "20260725T010000-land-retry",
		TargetBranch: "main",
		WorktreeDir:  projectRoot,
	})
	land1 := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-retry",
		Operation:      coordOpLand,
		IdempotencyKey: landKey,
		Payload:        landPayloadRaw,
	})
	require.Equal(t, http.StatusOK, land1.Code, land1.Body.String())
	var landResp coordinationMutationResponse
	require.NoError(t, json.Unmarshal(land1.Body.Bytes(), &landResp))
	require.Equal(t, string(coordination.OutcomeApplied), landResp.Outcome, landResp.Reason)
	require.Equal(t, resultSHA, landResp.NewTip)

	// Count commits on main after first land.
	logAfterFirst := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-list", "--count", "refs/heads/main"))

	landRetry := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-retry",
		Operation:      coordOpLand,
		IdempotencyKey: landKey,
		Payload:        landPayloadRaw,
	})
	require.Equal(t, http.StatusOK, landRetry.Code, landRetry.Body.String())
	var landRetryResp coordinationMutationResponse
	require.NoError(t, json.Unmarshal(landRetry.Body.Bytes(), &landRetryResp))
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), landRetryResp.Outcome)

	logAfterRetry := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-list", "--count", "refs/heads/main"))
	assert.Equal(t, logAfterFirst, logAfterRetry, "retry must not create landing commits")
	tip := strings.TrimSpace(runGitOutputCoord(t, projectRoot, "rev-parse", "refs/heads/main"))
	assert.Equal(t, resultSHA, tip)
}

// TestCoordinationReconcile_OrderedResumeAndConflict proves contiguous
// acknowledgement, resumable batches, and structured conflict evidence.
func TestCoordinationReconcile_OrderedResumeAndConflict(t *testing.T) {
	projectRoot := setupCoordinationProject(t)
	srv := New(":0", projectRoot)
	t.Cleanup(func() {
		if srv.workers != nil && srv.workers.LandCoordinators != nil {
			srv.workers.LandCoordinators.StopAll()
		}
	})
	proj := srv.state.RegisterProject(projectRoot)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const beadA = "ddx-http-reconcile-a"
	const beadB = "ddx-http-reconcile-b"
	const beadConflict = "ddx-http-reconcile-conflict"
	for _, id := range []string{beadA, beadB, beadConflict} {
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:    id,
			Title: "reconcile fixture " + id,
		}))
	}
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "seed reconcile fixtures")

	// Batch 1: two successful claims (seq 1-2).
	payloadA := mustJSON(t, claimPayload{BeadID: beadA, Assignee: "worker-reconcile"})
	payloadB := mustJSON(t, claimPayload{BeadID: beadB, Assignee: "worker-reconcile"})
	batch1 := postCoordinationReconcile(t, srv, proj.ID, coordinationReconcileRequest{
		WorkerID: "wkr-reconcile",
		Entries: []coordinationReconcileEntry{
			{
				Sequence:       1,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-a",
				PayloadHash:    payloadHash(payloadA),
				Payload:        payloadA,
			},
			{
				Sequence:       2,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-b",
				PayloadHash:    payloadHash(payloadB),
				Payload:        payloadB,
			},
		},
	})
	require.Equal(t, http.StatusOK, batch1.Code, batch1.Body.String())
	var resp1 coordinationReconcileResponse
	require.NoError(t, json.Unmarshal(batch1.Body.Bytes(), &resp1))
	assert.Equal(t, uint64(2), resp1.AcknowledgedThrough, "contiguous success acks through 2")
	require.Len(t, resp1.Results, 2)
	assert.Equal(t, string(coordination.OutcomeApplied), resp1.Results[0].Outcome)
	assert.Equal(t, string(coordination.OutcomeApplied), resp1.Results[1].Outcome)

	// Resumable: re-send seq 1-2 (already_applied) plus seq 3 (new claim on
	// already-owned bead → conflict) and seq 4 (transition).
	// First claim beadConflict with a different worker via mutations so
	// reconcile claim conflicts.
	preClaim := postCoordinationMutation(t, srv, proj.ID, coordinationMutationRequest{
		WorkerID:       "wkr-other",
		Operation:      coordOpClaim,
		IdempotencyKey: "pre-claim-conflict-owner",
		Payload:        mustJSON(t, claimPayload{BeadID: beadConflict, Assignee: "owner-worker"}),
	})
	require.Equal(t, http.StatusOK, preClaim.Code, preClaim.Body.String())

	payloadConflict := mustJSON(t, claimPayload{BeadID: beadConflict, Assignee: "worker-reconcile"})
	payloadTransition := mustJSON(t, transitionPayload{
		BeadID:   beadA,
		ToStatus: bead.StatusOpen,
		Reason:   "reconcile release",
		Actor:    "worker-reconcile",
	})

	batch2 := postCoordinationReconcile(t, srv, proj.ID, coordinationReconcileRequest{
		WorkerID: "wkr-reconcile",
		Entries: []coordinationReconcileEntry{
			{
				Sequence:       1,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-a",
				PayloadHash:    payloadHash(payloadA),
				Payload:        payloadA,
			},
			{
				Sequence:       2,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-b",
				PayloadHash:    payloadHash(payloadB),
				Payload:        payloadB,
			},
			{
				Sequence:       3,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-conflict",
				PayloadHash:    payloadHash(payloadConflict),
				Payload:        payloadConflict,
			},
			{
				Sequence:       4,
				Operation:      coordOpTrackerTransition,
				IdempotencyKey: "recon-transition-a",
				PayloadHash:    payloadHash(payloadTransition),
				Payload:        payloadTransition,
			},
		},
	})
	require.Equal(t, http.StatusOK, batch2.Code, batch2.Body.String())
	var resp2 coordinationReconcileResponse
	require.NoError(t, json.Unmarshal(batch2.Body.Bytes(), &resp2))
	require.Len(t, resp2.Results, 4)
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), resp2.Results[0].Outcome)
	assert.Equal(t, string(coordination.OutcomeAlreadyApplied), resp2.Results[1].Outcome)
	assert.Equal(t, string(coordination.OutcomeConflict), resp2.Results[2].Outcome)
	assert.Equal(t, coordination.ReasonAlreadyClaimed, resp2.Results[2].Reason)
	assert.Equal(t, "owner-worker", resp2.Results[2].Owner)
	assert.Equal(t, string(coordination.OutcomeApplied), resp2.Results[3].Outcome)
	// Conflicts are still acknowledged (structured evidence); contiguous through 4.
	assert.Equal(t, uint64(4), resp2.AcknowledgedThrough)

	// Payload-hash mismatch conflict evidence for a previously-seen key.
	tampered := mustJSON(t, claimPayload{BeadID: beadA, Assignee: "tampered-worker"})
	batch3 := postCoordinationReconcile(t, srv, proj.ID, coordinationReconcileRequest{
		WorkerID: "wkr-reconcile",
		Entries: []coordinationReconcileEntry{
			{
				Sequence:       5,
				Operation:      coordOpClaim,
				IdempotencyKey: "recon-claim-a", // same key, different payload
				PayloadHash:    payloadHash(tampered),
				Payload:        tampered,
			},
		},
	})
	require.Equal(t, http.StatusOK, batch3.Code, batch3.Body.String())
	var resp3 coordinationReconcileResponse
	require.NoError(t, json.Unmarshal(batch3.Body.Bytes(), &resp3))
	require.Len(t, resp3.Results, 1)
	assert.Equal(t, string(coordination.OutcomeConflict), resp3.Results[0].Outcome)
	assert.Equal(t, "payload_hash_mismatch", resp3.Results[0].Reason)
	assert.Equal(t, uint64(5), resp3.AcknowledgedThrough)

	// Durable store: beadA released to open via transition; beadB still claimed.
	gotA, err := store.Get(context.Background(), beadA)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotA.Status)
	gotB, err := store.Get(context.Background(), beadB)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusInProgress, gotB.Status)
	assert.Equal(t, "worker-reconcile", gotB.Owner)
	gotC, err := store.Get(context.Background(), beadConflict)
	require.NoError(t, err)
	assert.Equal(t, "owner-worker", gotC.Owner)
}

// --- helpers ----------------------------------------------------------------

func setupCoordinationProject(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	projectRoot := t.TempDir()
	initGitCoord(t, projectRoot)
	writeFileCoord(t, projectRoot, ".gitignore", strings.Join([]string{
		".ddx/executions/",
		".ddx/.git-tracker.lock",
		".ddx/.git-tracker.lock.*",
		"",
	}, "\n"))
	writeFileCoord(t, projectRoot, "README.md", "# coordination http fixture\n")
	runGitCoord(t, projectRoot, "add", "-A")
	runGitCoord(t, projectRoot, "commit", "-m", "init coordination fixture")
	testutils.MakeInitializedDDxRoot(t, projectRoot)
	return projectRoot
}

func postCoordinationMutation(t *testing.T, srv *Server, projectID string, body coordinationMutationRequest) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	path := "/api/projects/" + projectID + "/coordination/mutations"
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func postCoordinationReconcile(t *testing.T, srv *Server, projectID string, body coordinationReconcileRequest) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	path := "/api/projects/" + projectID + "/coordination/reconcile"
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func initGitCoord(t *testing.T, dir string) {
	t.Helper()
	runGitCoord(t, dir, "init", "-b", "main")
	runGitCoord(t, dir, "config", "user.name", "Coordination HTTP Test")
	runGitCoord(t, dir, "config", "user.email", "coordination-http@test.local")
}

func runGitCoord(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
}

func runGitOutputCoord(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
	return string(out)
}

func writeFileCoord(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func commitOnBaseCoord(t *testing.T, repo, baseSHA, path, content, msg string) string {
	t.Helper()
	wt, err := os.MkdirTemp("", "coord-http-land-wt-*")
	require.NoError(t, err)
	_ = os.RemoveAll(wt)
	runGitCoord(t, repo, "worktree", "add", "--detach", wt, baseSHA)
	defer func() {
		runGitCoord(t, repo, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}()

	writeFileCoord(t, wt, path, content)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add: %s", out)

	cmd = exec.Command("git",
		"-c", "user.name=Coordination HTTP Test",
		"-c", "user.email=coordination-http@test.local",
		"commit", "-m", msg)
	cmd.Dir = wt
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wt
	out, err = cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
