package server

// coordination.go — project-scoped worker coordination mutation and reconcile
// endpoints (ADR-022 rev 6, ddx-b45dcbd4).
//
// POST /api/projects/{project}/coordination/mutations
// POST /api/projects/{project}/coordination/reconcile
//
// Handlers serialize typed claim/transition/land mutations against the existing
// bead store and per-project LandCoordinator. Idempotency keys are remembered
// process-locally so unknown-response retries return already_applied without
// replaying store mutations or landing commits. No parallel claim table.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// Coordination protocol operation names (ADR-022 V1).
const (
	coordOpClaim             = "claim"
	coordOpRenewClaim        = "renew_claim"
	coordOpReleaseClaim      = "release_claim"
	coordOpTrackerTransition = "tracker_transition"
	coordOpLand              = "land"
)

// coordinationMutationRequest is the wire body for POST .../coordination/mutations.
type coordinationMutationRequest struct {
	WorkerID        string          `json:"worker_id"`
	Operation       string          `json:"operation"`
	IdempotencyKey  string          `json:"idempotency_key"`
	ObservedVersion string          `json:"observed_version,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

// coordinationMutationResponse is the wire body returned for one mutation.
// Outcome is applied, already_applied, or conflict.
type coordinationMutationResponse struct {
	Outcome        string `json:"outcome"`
	Operation      string `json:"operation"`
	IdempotencyKey string `json:"idempotency_key"`
	BeadID         string `json:"bead_id,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Owner          string `json:"owner,omitempty"`
	FromStatus     string `json:"from_status,omitempty"`
	ToStatus       string `json:"to_status,omitempty"`
	LandStatus     string `json:"land_status,omitempty"`
	NewTip         string `json:"new_tip,omitempty"`
	TargetBranch   string `json:"target_branch,omitempty"`
	PreserveRef    string `json:"preserve_ref,omitempty"`
	Merged         bool   `json:"merged,omitempty"`
	// Version is durable bead/git evidence after the attempt (owner@status
	// for claim/transition; tip SHA for land).
	Version string `json:"version,omitempty"`
}

// coordinationReconcileRequest is the wire body for POST .../coordination/reconcile.
type coordinationReconcileRequest struct {
	WorkerID string                        `json:"worker_id"`
	Entries  []coordinationReconcileEntry  `json:"entries"`
}

// coordinationReconcileEntry is one ordered offline journal mutation.
type coordinationReconcileEntry struct {
	Sequence       uint64          `json:"sequence"`
	Operation      string          `json:"operation"`
	IdempotencyKey string          `json:"idempotency_key"`
	PayloadHash    string          `json:"payload_hash"`
	Precondition   string          `json:"precondition,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

// coordinationReconcileResponse carries per-entry outcomes and the highest
// contiguous sequence the server has accepted (resumable batches).
type coordinationReconcileResponse struct {
	AcknowledgedThrough uint64                         `json:"acknowledged_through"`
	Results             []coordinationReconcileResult  `json:"results"`
}

// coordinationReconcileResult is one entry outcome inside a reconcile batch.
type coordinationReconcileResult struct {
	Sequence uint64 `json:"sequence"`
	coordinationMutationResponse
}

// coordinationOutcomeRecord is a remembered mutation outcome for idempotency
// and payload-hash conflict detection.
type coordinationOutcomeRecord struct {
	Response    coordinationMutationResponse
	PayloadHash string
}

// projectCoordinationRegistry caches one LocalCoordinator + outcome map per
// project root for the lifetime of the server process.
type projectCoordinationRegistry struct {
	mu      sync.Mutex
	byRoot  map[string]*projectCoordination
	// landFor resolves the production LandCoordinator for a project root.
	// Production wiring uses WorkerManager.LandCoordinators.
	landFor func(projectRoot string) *LandCoordinator
}

type projectCoordination struct {
	root   string
	coord  *coordination.LocalCoordinator
	store  *bead.Store
	mu     sync.Mutex
	byKey  map[string]coordinationOutcomeRecord
}

func newProjectCoordinationRegistry(landFor func(string) *LandCoordinator) *projectCoordinationRegistry {
	return &projectCoordinationRegistry{
		byRoot:  make(map[string]*projectCoordination),
		landFor: landFor,
	}
}

func (r *projectCoordinationRegistry) get(projectRoot string) *projectCoordination {
	projectRoot = strings.TrimSpace(projectRoot)
	r.mu.Lock()
	defer r.mu.Unlock()
	if pc, ok := r.byRoot[projectRoot]; ok {
		return pc
	}
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	var landBackend coordination.LandBackend
	if r.landFor != nil {
		if land := r.landFor(projectRoot); land != nil {
			landBackend = &landCoordinatorBackend{land: land}
		}
	}
	if landBackend == nil {
		// Fallback: direct production land path when no coordinator is available.
		landBackend = agent.NewCoordinationLandBackend(projectRoot, agent.RealLandingGitOps{})
	}
	pc := &projectCoordination{
		root:  projectRoot,
		coord: coordination.NewLocalCoordinatorWithLand(store, landBackend),
		store: store,
		byKey: make(map[string]coordinationOutcomeRecord),
	}
	r.byRoot[projectRoot] = pc
	return pc
}

// landCoordinatorBackend adapts LandCoordinator.Submit to coordination.LandBackend
// so HTTP mutations share the same single-writer land path as in-process workers.
type landCoordinatorBackend struct {
	land *LandCoordinator
}

var _ coordination.LandBackend = (*landCoordinatorBackend)(nil)

func (b *landCoordinatorBackend) Land(ctx context.Context, req coordination.LandRequest) (coordination.LandResult, error) {
	if b == nil || b.land == nil {
		return coordination.LandResult{}, fmt.Errorf("coordination: nil land coordinator")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return coordination.LandResult{}, err
		}
	}
	worktree := strings.TrimSpace(req.WorktreeDir)
	if worktree == "" {
		worktree = strings.TrimSpace(req.ProjectRoot)
	}
	res, err := b.land.Submit(agent.LandRequest{
		WorktreeDir:  worktree,
		BaseRev:      strings.TrimSpace(req.BaseRev),
		ResultRev:    strings.TrimSpace(req.ResultRev),
		BeadID:       strings.TrimSpace(req.BeadID),
		AttemptID:    strings.TrimSpace(req.AttemptID),
		TargetBranch: strings.TrimSpace(req.TargetBranch),
		EvidenceDir:  strings.TrimSpace(req.EvidenceDir),
	})
	if err != nil {
		return coordination.LandResult{}, err
	}
	if res == nil {
		return coordination.LandResult{}, fmt.Errorf("coordination: land coordinator returned nil result")
	}
	out := coordination.LandResult{
		BeadID:       strings.TrimSpace(req.BeadID),
		Status:       res.Status,
		NewTip:       res.NewTip,
		TargetBranch: res.TargetBranch,
		Merged:       res.Merged,
		PreserveRef:  res.PreserveRef,
		Reason:       res.Reason,
	}
	switch res.Status {
	case coordination.LandStatusPreserved:
		if out.Reason == "" {
			out.Reason = coordination.ReasonLandPreserved
		}
	case "":
		out.Status = coordination.LandStatusLanded
	}
	return out, nil
}

// landCoordinatorFor returns the production LandCoordinator for projectRoot
// via the supervisor registry when available, else the server's live
// WorkerManager.LandCoordinators. Never fabricates a parallel land path.
func (s *Server) landCoordinatorFor(projectRoot string) *LandCoordinator {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}
	if s.supervisorRegistry != nil {
		if sup := s.supervisorRegistry.getOrCreate(projectRoot); sup != nil && sup.manager != nil && sup.manager.LandCoordinators != nil {
			return sup.manager.LandCoordinators.Get(projectRoot)
		}
	}
	if s.workers != nil && s.workers.LandCoordinators != nil {
		return s.workers.LandCoordinators.Get(projectRoot)
	}
	return nil
}

func (s *Server) coordinationRegistry() *projectCoordinationRegistry {
	s.coordRegOnce.Do(func() {
		s.coordReg = newProjectCoordinationRegistry(s.landCoordinatorFor)
	})
	return s.coordReg
}

// handleCoordinationMutation serves POST .../coordination/mutations.
func (s *Server) handleCoordinationMutation(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	projectRoot := s.workingDirForRequest(r)
	if projectRoot == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project root required"})
		return
	}

	var req coordinationMutationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}
	req.Operation = strings.TrimSpace(req.Operation)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.Operation == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "operation is required"})
		return
	}
	if req.IdempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key is required"})
		return
	}

	pc := s.coordinationRegistry().get(projectRoot)
	resp, err := pc.applyMutation(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCoordinationReconcile serves POST .../coordination/reconcile.
func (s *Server) handleCoordinationReconcile(w http.ResponseWriter, r *http.Request) {
	if !isTrusted(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write endpoints are localhost-only"})
		return
	}
	projectRoot := s.workingDirForRequest(r)
	if projectRoot == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project root required"})
		return
	}

	var req coordinationReconcileRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}
	if len(req.Entries) == 0 {
		writeJSON(w, http.StatusOK, coordinationReconcileResponse{
			AcknowledgedThrough: 0,
			Results:             []coordinationReconcileResult{},
		})
		return
	}

	// Process in the order supplied (worker journal order). Contiguous ack
	// advances through applied/already_applied/conflict; hard errors stop ack.
	pc := s.coordinationRegistry().get(projectRoot)
	results := make([]coordinationReconcileResult, 0, len(req.Entries))
	var ackThrough uint64
	contiguous := true

	for _, entry := range req.Entries {
		seq := entry.Sequence
		key := strings.TrimSpace(entry.IdempotencyKey)
		op := strings.TrimSpace(entry.Operation)
		hash := strings.TrimSpace(entry.PayloadHash)
		if hash == "" && len(entry.Payload) > 0 {
			hash = payloadHash(entry.Payload)
		}

		row := coordinationReconcileResult{Sequence: seq}
		if seq == 0 || key == "" || op == "" {
			row.Outcome = string(coordination.OutcomeConflict)
			row.Operation = op
			row.IdempotencyKey = key
			row.Reason = "invalid_journal_entry"
			results = append(results, row)
			contiguous = false
			continue
		}

		// Payload-hash conflict against a prior observation of this key.
		if prev, ok := pc.lookupOutcome(key); ok && prev.PayloadHash != "" && hash != "" && prev.PayloadHash != hash {
			row.coordinationMutationResponse = prev.Response
			row.Outcome = string(coordination.OutcomeConflict)
			row.Reason = "payload_hash_mismatch"
			row.IdempotencyKey = key
			row.Operation = op
			results = append(results, row)
			if contiguous {
				ackThrough = seq
			}
			continue
		}

		mutReq := coordinationMutationRequest{
			WorkerID:        req.WorkerID,
			Operation:       op,
			IdempotencyKey:  key,
			ObservedVersion: strings.TrimSpace(entry.Precondition),
			Payload:         entry.Payload,
		}
		// Force payload hash into apply so the outcome record stores it.
		resp, err := pc.applyMutationWithHash(r.Context(), mutReq, hash)
		if err != nil {
			row.Outcome = string(coordination.OutcomeConflict)
			row.Operation = op
			row.IdempotencyKey = key
			row.Reason = err.Error()
			results = append(results, row)
			contiguous = false
			continue
		}
		row.coordinationMutationResponse = resp
		results = append(results, row)
		if contiguous {
			ackThrough = seq
		}
	}

	writeJSON(w, http.StatusOK, coordinationReconcileResponse{
		AcknowledgedThrough: ackThrough,
		Results:             results,
	})
}

func (pc *projectCoordination) lookupOutcome(key string) (coordinationOutcomeRecord, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	rec, ok := pc.byKey[key]
	return rec, ok
}

func (pc *projectCoordination) applyMutation(ctx context.Context, req coordinationMutationRequest) (coordinationMutationResponse, error) {
	return pc.applyMutationWithHash(ctx, req, payloadHash(req.Payload))
}

func (pc *projectCoordination) applyMutationWithHash(ctx context.Context, req coordinationMutationRequest, hash string) (coordinationMutationResponse, error) {
	key := strings.TrimSpace(req.IdempotencyKey)
	op := strings.TrimSpace(req.Operation)
	if key == "" {
		return coordinationMutationResponse{}, fmt.Errorf("coordination: idempotency_key is required")
	}
	if op == "" {
		return coordinationMutationResponse{}, fmt.Errorf("coordination: operation is required")
	}
	if hash == "" {
		hash = payloadHash(req.Payload)
	}

	// Fast path: remembered key → already_applied (or conflict on hash mismatch).
	if prev, ok := pc.lookupOutcome(key); ok {
		if prev.PayloadHash != "" && hash != "" && prev.PayloadHash != hash {
			out := prev.Response
			out.Outcome = string(coordination.OutcomeConflict)
			out.Reason = "payload_hash_mismatch"
			out.IdempotencyKey = key
			out.Operation = op
			return out, nil
		}
		out := prev.Response
		out.Outcome = string(coordination.OutcomeAlreadyApplied)
		out.IdempotencyKey = key
		out.Operation = op
		return out, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var (
		resp coordinationMutationResponse
		err  error
	)
	switch op {
	case coordOpClaim:
		resp, err = pc.applyClaim(ctx, key, req.Payload)
	case coordOpRenewClaim:
		resp, err = pc.applyRenewClaim(ctx, key, req.Payload)
	case coordOpReleaseClaim:
		resp, err = pc.applyReleaseClaim(ctx, key, req.Payload)
	case coordOpTrackerTransition:
		resp, err = pc.applyTransition(ctx, key, req.Payload)
	case coordOpLand:
		resp, err = pc.applyLand(ctx, key, req.Payload)
	default:
		return coordinationMutationResponse{}, fmt.Errorf("coordination: unknown operation %q", op)
	}
	if err != nil {
		return coordinationMutationResponse{}, err
	}
	resp.Operation = op
	resp.IdempotencyKey = key

	// Remember applied and conflict outcomes so retries do not re-mutate.
	// Hard transport errors are not remembered (caller may retry).
	pc.mu.Lock()
	pc.byKey[key] = coordinationOutcomeRecord{Response: resp, PayloadHash: hash}
	pc.mu.Unlock()
	return resp, nil
}

type claimPayload struct {
	BeadID   string `json:"bead_id"`
	Assignee string `json:"assignee"`
	Session  string `json:"session,omitempty"`
	Worktree string `json:"worktree,omitempty"`
}

type transitionPayload struct {
	BeadID                string `json:"bead_id"`
	ToStatus              string `json:"to_status"`
	OperatorRequired      bool   `json:"operator_required,omitempty"`
	ExternalBlockerReason string `json:"external_blocker_reason,omitempty"`
	ManualClose           bool   `json:"manual_close,omitempty"`
	ManualReopen          bool   `json:"manual_reopen,omitempty"`
	Reason                string `json:"reason,omitempty"`
	Actor                 string `json:"actor,omitempty"`
	Source                string `json:"source,omitempty"`
}

type landPayload struct {
	BeadID       string `json:"bead_id"`
	BaseRev      string `json:"base_rev"`
	ResultRev    string `json:"result_rev"`
	AttemptID    string `json:"attempt_id"`
	TargetBranch string `json:"target_branch,omitempty"`
	WorktreeDir  string `json:"worktree_dir,omitempty"`
	EvidenceDir  string `json:"evidence_dir,omitempty"`
}

type releasePayload struct {
	BeadID string `json:"bead_id"`
}

func (pc *projectCoordination) applyClaim(ctx context.Context, key string, raw json.RawMessage) (coordinationMutationResponse, error) {
	var p claimPayload
	if err := decodePayload(raw, &p); err != nil {
		return coordinationMutationResponse{}, err
	}
	result, err := pc.coord.Claim(ctx, coordination.ClaimRequest{
		BeadID:         p.BeadID,
		Assignee:       p.Assignee,
		IdempotencyKey: key,
		Session:        p.Session,
		Worktree:       p.Worktree,
	})
	if err != nil {
		return coordinationMutationResponse{}, err
	}
	return coordinationMutationResponse{
		Outcome:        string(result.Code),
		BeadID:         result.BeadID,
		Owner:          result.Owner,
		Reason:         result.Reason,
		IdempotencyKey: key,
		Version:        durableBeadVersion(result.Owner, pc.lookupStatus(ctx, result.BeadID)),
	}, nil
}

func (pc *projectCoordination) applyRenewClaim(ctx context.Context, key string, raw json.RawMessage) (coordinationMutationResponse, error) {
	var p claimPayload
	if err := decodePayload(raw, &p); err != nil {
		return coordinationMutationResponse{}, err
	}
	beadID := strings.TrimSpace(p.BeadID)
	assignee := strings.TrimSpace(p.Assignee)
	if beadID == "" || assignee == "" {
		return coordinationMutationResponse{}, fmt.Errorf("coordination: renew_claim requires bead_id and assignee")
	}
	// ClaimWithOptions refreshes the external lease for a live owner when the
	// lease is stale or same-owner; contention maps to conflict.
	err := pc.store.ClaimWithOptions(beadID, assignee, p.Session, p.Worktree)
	if err != nil {
		if isClaimContention(err) {
			owner := pc.lookupOwner(ctx, beadID)
			return coordinationMutationResponse{
				Outcome:        string(coordination.OutcomeConflict),
				BeadID:         beadID,
				Owner:          owner,
				Reason:         coordination.ReasonAlreadyClaimed,
				IdempotencyKey: key,
				Version:        durableBeadVersion(owner, pc.lookupStatus(ctx, beadID)),
			}, nil
		}
		return coordinationMutationResponse{}, err
	}
	owner := assignee
	if got := pc.lookupOwner(ctx, beadID); got != "" {
		owner = got
	}
	return coordinationMutationResponse{
		Outcome:        string(coordination.OutcomeApplied),
		BeadID:         beadID,
		Owner:          owner,
		IdempotencyKey: key,
		Version:        durableBeadVersion(owner, pc.lookupStatus(ctx, beadID)),
	}, nil
}

func (pc *projectCoordination) applyReleaseClaim(ctx context.Context, key string, raw json.RawMessage) (coordinationMutationResponse, error) {
	var p releasePayload
	if err := decodePayload(raw, &p); err != nil {
		return coordinationMutationResponse{}, err
	}
	beadID := strings.TrimSpace(p.BeadID)
	if beadID == "" {
		return coordinationMutationResponse{}, fmt.Errorf("coordination: release_claim requires bead_id")
	}
	if err := pc.store.Unclaim(beadID); err != nil {
		// Not found / not claimed → conflict with evidence rather than hard error.
		return coordinationMutationResponse{
			Outcome:        string(coordination.OutcomeConflict),
			BeadID:         beadID,
			Reason:         err.Error(),
			IdempotencyKey: key,
			Version:        durableBeadVersion(pc.lookupOwner(ctx, beadID), pc.lookupStatus(ctx, beadID)),
		}, nil
	}
	return coordinationMutationResponse{
		Outcome:        string(coordination.OutcomeApplied),
		BeadID:         beadID,
		IdempotencyKey: key,
		Version:        durableBeadVersion("", pc.lookupStatus(ctx, beadID)),
	}, nil
}

func (pc *projectCoordination) applyTransition(ctx context.Context, key string, raw json.RawMessage) (coordinationMutationResponse, error) {
	var p transitionPayload
	if err := decodePayload(raw, &p); err != nil {
		return coordinationMutationResponse{}, err
	}
	result, err := pc.coord.Transition(ctx, coordination.TransitionRequest{
		BeadID:                p.BeadID,
		ToStatus:              p.ToStatus,
		IdempotencyKey:        key,
		OperatorRequired:      p.OperatorRequired,
		ExternalBlockerReason: p.ExternalBlockerReason,
		ManualClose:           p.ManualClose,
		ManualReopen:          p.ManualReopen,
		Reason:                p.Reason,
		Actor:                 p.Actor,
		Source:                p.Source,
	})
	if err != nil {
		return coordinationMutationResponse{}, err
	}
	return coordinationMutationResponse{
		Outcome:        string(result.Code),
		BeadID:         result.BeadID,
		FromStatus:     result.FromStatus,
		ToStatus:       result.ToStatus,
		Reason:         result.Reason,
		IdempotencyKey: key,
		Version:        durableBeadVersion(pc.lookupOwner(ctx, result.BeadID), result.ToStatus),
	}, nil
}

func (pc *projectCoordination) applyLand(ctx context.Context, key string, raw json.RawMessage) (coordinationMutationResponse, error) {
	var p landPayload
	if err := decodePayload(raw, &p); err != nil {
		return coordinationMutationResponse{}, err
	}
	result, err := pc.coord.Land(ctx, coordination.LandRequest{
		ProjectRoot:    pc.root,
		WorktreeDir:    coordFirstNonEmpty(p.WorktreeDir, pc.root),
		BaseRev:        p.BaseRev,
		ResultRev:      p.ResultRev,
		BeadID:         p.BeadID,
		AttemptID:      p.AttemptID,
		TargetBranch:   p.TargetBranch,
		EvidenceDir:    p.EvidenceDir,
		IdempotencyKey: key,
	})
	if err != nil {
		return coordinationMutationResponse{}, err
	}
	version := result.NewTip
	if version == "" {
		version = result.PreserveRef
	}
	return coordinationMutationResponse{
		Outcome:        string(result.Code),
		BeadID:         result.BeadID,
		LandStatus:     result.Status,
		NewTip:         result.NewTip,
		TargetBranch:   result.TargetBranch,
		PreserveRef:    result.PreserveRef,
		Merged:         result.Merged,
		Reason:         result.Reason,
		IdempotencyKey: key,
		Version:        version,
	}, nil
}

func (pc *projectCoordination) lookupOwner(ctx context.Context, beadID string) string {
	if pc == nil || pc.store == nil || beadID == "" {
		return ""
	}
	b, err := pc.store.Get(ctx, beadID)
	if err != nil || b == nil {
		return ""
	}
	return strings.TrimSpace(b.Owner)
}

func (pc *projectCoordination) lookupStatus(ctx context.Context, beadID string) string {
	if pc == nil || pc.store == nil || beadID == "" {
		return ""
	}
	b, err := pc.store.Get(ctx, beadID)
	if err != nil || b == nil {
		return ""
	}
	return strings.TrimSpace(b.Status)
}

func decodePayload(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return fmt.Errorf("coordination: payload is required")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("coordination: invalid payload: %w", err)
	}
	return nil
}

func payloadHash(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func durableBeadVersion(owner, status string) string {
	owner = strings.TrimSpace(owner)
	status = strings.TrimSpace(status)
	switch {
	case owner != "" && status != "":
		return owner + "@" + status
	case status != "":
		return status
	case owner != "":
		return owner
	default:
		return ""
	}
}

func coordFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func isClaimContention(err error) bool {
	if err == nil {
		return false
	}
	if err == bead.ErrAlreadyClaimed {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "cannot claim") || strings.Contains(msg, "already claimed")
}
