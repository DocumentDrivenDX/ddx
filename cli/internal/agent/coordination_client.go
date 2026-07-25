package agent

// coordination_client.go — reconnecting worker coordination client (ADR-022 rev 6).
//
// Manual ddx try / ddx work and server-managed ddx work --server-managed share
// one client. While Connected, claim / tracker-transition / land mutations go
// through the project-scoped server HTTP endpoints. On transport failure the
// current operation switches atomically to the offline journal + local
// coordinator. Reconnection reconciles pending journal entries before any new
// online write. Process ownership (managed workers terminate with the server)
// is orthogonal to this client.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// Coordination operation names match the server ADR-022 V1 protocol.
const (
	coordOpClaim             = "claim"
	coordOpTrackerTransition = "tracker_transition"
	coordOpLand              = "land"
)

// DefaultCoordinationDiscoverInterval is the periodic server discovery /
// reconnect probe interval for the shared coordination client.
const DefaultCoordinationDiscoverInterval = 30 * time.Second

// CoordinationClient is the reconnecting coordination client used by try,
// manual work, and managed work. It implements coordination.Coordinator.
type CoordinationClient struct {
	projectRoot   string
	workerID      string
	addrFunc      func() string
	httpClient    *http.Client
	discoverEvery time.Duration

	local   *coordination.LocalCoordinator
	offline *OfflineCoordinator
	journal *OfflineJournal

	mu          sync.Mutex
	connected   bool
	baseURL     string
	projectID   string
	reconciling bool
	// pendingOnline is true when the journal has unacked entries that must be
	// reconciled before a new online write.
	stopCh chan struct{}
	doneCh chan struct{}
	// test hooks
	onStateChange func(connected bool)
	// forceOffline skips online attempts (tests).
	forceOffline bool
}

// CoordinationClientConfig configures NewCoordinationClient.
type CoordinationClientConfig struct {
	// WorkerID is the coordination worker identity (assignee). Defaults to "ddx".
	WorkerID string
	// AddrFunc returns the current server base URL or "". Defaults to a no-op
	// empty reader; production wires server.ReadServerAddr.
	AddrFunc func() string
	// HTTPClient is used for mutations/reconcile. Defaults to a loopback
	// client that skips TLS verification (self-signed ddx-server cert).
	HTTPClient *http.Client
	// DiscoverInterval bounds periodic reconnect probes. Zero uses
	// DefaultCoordinationDiscoverInterval.
	DiscoverInterval time.Duration
	// LandGitOps is the landing git surface for the offline local path.
	// Nil uses RealLandingGitOps.
	LandGitOps LandingGitOps
	// Store is the project bead store (claim/transition offline backend).
	// Required.
	Store coordination.ClaimBackend
}

// Compile-time: CoordinationClient satisfies coordination.Coordinator.
var _ coordination.Coordinator = (*CoordinationClient)(nil)

// NewCoordinationClient builds a reconnecting client for projectRoot.
// Call Start to begin periodic discovery; call Close when the worker exits.
func NewCoordinationClient(projectRoot string, cfg CoordinationClientConfig) (*CoordinationClient, error) {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("coordination client: project root is empty")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("coordination client: store is required")
	}
	workerID := strings.TrimSpace(cfg.WorkerID)
	if workerID == "" {
		workerID = "ddx"
	}
	addrFunc := cfg.AddrFunc
	if addrFunc == nil {
		addrFunc = func() string { return "" }
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local self-signed cert
			},
		}
	}
	discover := cfg.DiscoverInterval
	if discover <= 0 {
		discover = DefaultCoordinationDiscoverInterval
	}
	landOps := cfg.LandGitOps
	if landOps == nil {
		landOps = RealLandingGitOps{}
	}
	journal, err := OpenOfflineJournal(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("coordination client: open journal: %w", err)
	}
	landBackend := NewCoordinationLandBackend(projectRoot, landOps)
	local := coordination.NewLocalCoordinatorWithLand(cfg.Store, landBackend)
	return &CoordinationClient{
		projectRoot:   projectRoot,
		workerID:      workerID,
		addrFunc:      addrFunc,
		httpClient:    httpClient,
		discoverEvery: discover,
		local:         local,
		offline:       NewOfflineCoordinator(projectRoot),
		journal:       journal,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}, nil
}

// Start begins periodic server discovery and reconnect. Safe to call once.
// A cancelled ctx or Stop/Close ends the loop.
func (c *CoordinationClient) Start(ctx context.Context) {
	if c == nil {
		return
	}
	go c.discoverLoop(ctx)
}

// Close stops discovery and releases the offline journal.
func (c *CoordinationClient) Close() error {
	if c == nil {
		return nil
	}
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	// Wait briefly for discover loop; do not block forever if never Started.
	select {
	case <-c.doneCh:
	case <-time.After(100 * time.Millisecond):
	}
	if c.journal != nil {
		return c.journal.Close()
	}
	return nil
}

// Connected reports whether the client currently prefers the server path.
func (c *CoordinationClient) Connected() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && !c.forceOffline
}

// SetForceOffline is a test seam that forces the offline path.
func (c *CoordinationClient) SetForceOffline(v bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.forceOffline = v
	if v {
		c.connected = false
	}
	c.mu.Unlock()
}

// MarkDisconnectedForTest forces Connected → NotConnected (test seam).
func (c *CoordinationClient) MarkDisconnectedForTest() {
	c.markDisconnected()
}

// ProbeOnceForTest runs one discovery probe (test seam).
func (c *CoordinationClient) ProbeOnceForTest(ctx context.Context) {
	c.probeOnce(ctx)
}

// Claim implements coordination.Coordinator.
func (c *CoordinationClient) Claim(ctx context.Context, req coordination.ClaimRequest) (coordination.ClaimResult, error) {
	if c == nil {
		return coordination.ClaimResult{}, fmt.Errorf("coordination client: nil client")
	}
	payload, err := json.Marshal(map[string]string{
		"bead_id":  strings.TrimSpace(req.BeadID),
		"assignee": strings.TrimSpace(req.Assignee),
		"session":  strings.TrimSpace(req.Session),
		"worktree": strings.TrimSpace(req.Worktree),
	})
	if err != nil {
		return coordination.ClaimResult{}, err
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	raw, err := c.mutate(ctx, coordOpClaim, key, payload, func(ctx context.Context) (any, error) {
		return c.local.Claim(ctx, req)
	})
	if err != nil {
		return coordination.ClaimResult{}, err
	}
	switch r := raw.(type) {
	case coordination.ClaimResult:
		return r, nil
	case coordMutationResponse:
		return coordination.ClaimResult{
			Code:           coordination.OutcomeCode(r.Outcome),
			BeadID:         coordFirstNonEmpty(r.BeadID, req.BeadID),
			Owner:          r.Owner,
			Reason:         r.Reason,
			IdempotencyKey: key,
		}, nil
	default:
		return coordination.ClaimResult{}, fmt.Errorf("coordination client: unexpected claim result type %T", raw)
	}
}

// Transition implements coordination.Coordinator.
func (c *CoordinationClient) Transition(ctx context.Context, req coordination.TransitionRequest) (coordination.TransitionResult, error) {
	if c == nil {
		return coordination.TransitionResult{}, fmt.Errorf("coordination client: nil client")
	}
	payload, err := json.Marshal(map[string]any{
		"bead_id":                 strings.TrimSpace(req.BeadID),
		"to_status":               strings.TrimSpace(req.ToStatus),
		"operator_required":       req.OperatorRequired,
		"external_blocker_reason": req.ExternalBlockerReason,
		"manual_close":            req.ManualClose,
		"manual_reopen":           req.ManualReopen,
		"reason":                  req.Reason,
		"actor":                   req.Actor,
		"source":                  coordFirstNonEmpty(req.Source, "coordination.client"),
	})
	if err != nil {
		return coordination.TransitionResult{}, err
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	raw, err := c.mutate(ctx, coordOpTrackerTransition, key, payload, func(ctx context.Context) (any, error) {
		return c.local.Transition(ctx, req)
	})
	if err != nil {
		return coordination.TransitionResult{}, err
	}
	switch r := raw.(type) {
	case coordination.TransitionResult:
		return r, nil
	case coordMutationResponse:
		return coordination.TransitionResult{
			Code:           coordination.OutcomeCode(r.Outcome),
			BeadID:         coordFirstNonEmpty(r.BeadID, req.BeadID),
			FromStatus:     r.FromStatus,
			ToStatus:       coordFirstNonEmpty(r.ToStatus, req.ToStatus),
			Reason:         r.Reason,
			IdempotencyKey: key,
		}, nil
	default:
		return coordination.TransitionResult{}, fmt.Errorf("coordination client: unexpected transition result type %T", raw)
	}
}

// Land implements coordination.Coordinator.
func (c *CoordinationClient) Land(ctx context.Context, req coordination.LandRequest) (coordination.LandResult, error) {
	if c == nil {
		return coordination.LandResult{}, fmt.Errorf("coordination client: nil client")
	}
	if strings.TrimSpace(req.ProjectRoot) == "" {
		req.ProjectRoot = c.projectRoot
	}
	payload, err := json.Marshal(map[string]string{
		"bead_id":       strings.TrimSpace(req.BeadID),
		"base_rev":      strings.TrimSpace(req.BaseRev),
		"result_rev":    strings.TrimSpace(req.ResultRev),
		"attempt_id":    strings.TrimSpace(req.AttemptID),
		"target_branch": strings.TrimSpace(req.TargetBranch),
		"worktree_dir":  strings.TrimSpace(req.WorktreeDir),
		"evidence_dir":  strings.TrimSpace(req.EvidenceDir),
	})
	if err != nil {
		return coordination.LandResult{}, err
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	raw, err := c.mutate(ctx, coordOpLand, key, payload, func(ctx context.Context) (any, error) {
		return c.local.Land(ctx, req)
	})
	if err != nil {
		return coordination.LandResult{}, err
	}
	switch r := raw.(type) {
	case coordination.LandResult:
		return r, nil
	case coordMutationResponse:
		return coordination.LandResult{
			Code:           coordination.OutcomeCode(r.Outcome),
			BeadID:         coordFirstNonEmpty(r.BeadID, req.BeadID),
			Status:         r.LandStatus,
			NewTip:         r.NewTip,
			TargetBranch:   r.TargetBranch,
			PreserveRef:    r.PreserveRef,
			Merged:         r.Merged,
			Reason:         r.Reason,
			IdempotencyKey: key,
		}, nil
	default:
		return coordination.LandResult{}, fmt.Errorf("coordination client: unexpected land result type %T", raw)
	}
}

// SubmitLand adapts agent.LandRequest to the coordination Land path for the
// existing SubmitWithPreMergeChecks callback shape.
func (c *CoordinationClient) SubmitLand(req LandRequest) (*LandResult, error) {
	if c == nil {
		return nil, fmt.Errorf("coordination client: nil client")
	}
	key := landIdempotencyKey(req)
	result, err := c.Land(context.Background(), coordination.LandRequest{
		ProjectRoot:    c.projectRoot,
		WorktreeDir:    coordFirstNonEmpty(req.WorktreeDir, c.projectRoot),
		BaseRev:        req.BaseRev,
		ResultRev:      req.ResultRev,
		BeadID:         req.BeadID,
		AttemptID:      req.AttemptID,
		TargetBranch:   req.TargetBranch,
		EvidenceDir:    req.EvidenceDir,
		IdempotencyKey: key,
	})
	if err != nil {
		return nil, err
	}
	return &LandResult{
		Status:       result.Status,
		NewTip:       result.NewTip,
		TargetBranch: result.TargetBranch,
		Merged:       result.Merged,
		PreserveRef:  result.PreserveRef,
		Reason:       result.Reason,
	}, nil
}

// ClaimBead is the store-shaped claim helper used by execute-loop wiring.
// Conflict maps to bead.ErrAlreadyClaimed so existing claim-race handling works.
func (c *CoordinationClient) ClaimBead(ctx context.Context, beadID, assignee, session, worktree string) error {
	if c == nil {
		return fmt.Errorf("coordination client: nil client")
	}
	key := fmt.Sprintf("claim:%s:%s:%d", beadID, assignee, time.Now().UnixNano())
	result, err := c.Claim(ctx, coordination.ClaimRequest{
		BeadID:         beadID,
		Assignee:       assignee,
		IdempotencyKey: key,
		Session:        session,
		Worktree:       worktree,
	})
	if err != nil {
		return err
	}
	switch result.Code {
	case coordination.OutcomeApplied, coordination.OutcomeAlreadyApplied:
		return nil
	case coordination.OutcomeConflict:
		return bead.ErrAlreadyClaimed
	default:
		return fmt.Errorf("coordination client: claim outcome %q: %s", result.Code, result.Reason)
	}
}

// TransitionBead is the store-shaped lifecycle transition helper.
func (c *CoordinationClient) TransitionBead(ctx context.Context, beadID, toStatus string, opts bead.LifecycleTransitionOptions) error {
	if c == nil {
		return fmt.Errorf("coordination client: nil client")
	}
	key := fmt.Sprintf("transition:%s:%s:%d", beadID, toStatus, time.Now().UnixNano())
	result, err := c.Transition(ctx, coordination.TransitionRequest{
		BeadID:                beadID,
		ToStatus:              toStatus,
		IdempotencyKey:        key,
		OperatorRequired:      opts.OperatorRequired,
		ExternalBlockerReason: opts.ExternalBlockerReason,
		ManualClose:           opts.ManualClose,
		ManualReopen:          opts.ManualReopen,
		Reason:                opts.Reason,
		Actor:                 opts.Actor,
		Source:                coordFirstNonEmpty(opts.Source, "coordination.client"),
	})
	if err != nil {
		return err
	}
	switch result.Code {
	case coordination.OutcomeApplied, coordination.OutcomeAlreadyApplied:
		return nil
	case coordination.OutcomeConflict:
		return fmt.Errorf("bead: lifecycle transition rejected: %s", result.Reason)
	default:
		return fmt.Errorf("coordination client: transition outcome %q: %s", result.Code, result.Reason)
	}
}

// --- internals --------------------------------------------------------------

type coordMutationResponse struct {
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
	Version        string `json:"version,omitempty"`
}

type coordMutationRequest struct {
	WorkerID       string          `json:"worker_id"`
	Operation      string          `json:"operation"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
}

type coordReconcileRequest struct {
	WorkerID string                `json:"worker_id"`
	Entries  []coordReconcileEntry `json:"entries"`
}

type coordReconcileEntry struct {
	Sequence       uint64          `json:"sequence"`
	Operation      string          `json:"operation"`
	IdempotencyKey string          `json:"idempotency_key"`
	PayloadHash    string          `json:"payload_hash"`
	Precondition   string          `json:"precondition,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

type coordReconcileResponse struct {
	AcknowledgedThrough uint64 `json:"acknowledged_through"`
}

func (c *CoordinationClient) discoverLoop(ctx context.Context) {
	defer close(c.doneCh)
	// Immediate first probe.
	c.probeOnce(ctx)
	t := time.NewTicker(c.discoverEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-t.C:
			c.probeOnce(ctx)
		}
	}
}

func (c *CoordinationClient) probeOnce(ctx context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	forced := c.forceOffline
	c.mu.Unlock()
	if forced {
		return
	}
	addr := strings.TrimSpace(c.addrFunc())
	if addr == "" {
		c.markDisconnected()
		return
	}
	// Cheap health check.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(addr, "/")+"/api/health", nil)
	if err != nil {
		c.markDisconnected()
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.markDisconnected()
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.markDisconnected()
		return
	}
	// Ensure project is registered and cache project id.
	projectID, err := c.ensureProjectRegistered(ctx, addr)
	if err != nil {
		c.markDisconnected()
		return
	}
	wasConnected := c.Connected()
	c.mu.Lock()
	c.connected = true
	c.baseURL = strings.TrimRight(addr, "/")
	c.projectID = projectID
	c.mu.Unlock()
	// Always attempt reconcile while reachable so a previously-failed
	// reconnect (or ignored probe reconcile error) cannot leave the journal
	// unacked across later Connected probes. mutate() also reconciles before
	// online writes; this keeps discovery proactive.
	_ = c.Reconcile(ctx)
	if !wasConnected && c.onStateChange != nil {
		c.onStateChange(true)
	}
}

func (c *CoordinationClient) markDisconnected() {
	c.mu.Lock()
	was := c.connected
	c.connected = false
	c.mu.Unlock()
	if was && c.onStateChange != nil {
		c.onStateChange(false)
	}
}

func (c *CoordinationClient) ensureProjectRegistered(ctx context.Context, addr string) (string, error) {
	body, _ := json.Marshal(map[string]string{"path": c.projectRoot})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(addr, "/")+"/api/projects/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("register project: status=%d body=%s", resp.StatusCode, string(b))
	}
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return "", err
	}
	if entry.ID == "" {
		return "", fmt.Errorf("register project: empty id")
	}
	return entry.ID, nil
}

// mutate runs one coordination mutation online or offline.
// offlineApply is invoked under the offline project lock when falling back.
func (c *CoordinationClient) mutate(
	ctx context.Context,
	op, key string,
	payload json.RawMessage,
	offlineApply func(context.Context) (any, error),
) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("coordination client: idempotency_key is required")
	}

	if c.preferOnline() {
		// Reconcile any offline backlog before a new online write.
		if err := c.Reconcile(ctx); err != nil {
			// Reconcile failure keeps us offline for this mutation.
			c.markDisconnected()
		} else if c.preferOnline() {
			resp, err := c.httpMutate(ctx, op, key, payload)
			if err == nil {
				return resp, nil
			}
			if !isCoordinationTransportError(err) {
				return nil, err
			}
			// Unknown / transport failure: switch this operation offline.
			c.markDisconnected()
		}
	}

	return c.offlineMutate(ctx, op, key, payload, offlineApply)
}

func (c *CoordinationClient) preferOnline() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && !c.forceOffline && c.baseURL != "" && c.projectID != ""
}

func (c *CoordinationClient) httpMutate(ctx context.Context, op, key string, payload json.RawMessage) (coordMutationResponse, error) {
	c.mu.Lock()
	base := c.baseURL
	projectID := c.projectID
	workerID := c.workerID
	c.mu.Unlock()
	if base == "" || projectID == "" {
		return coordMutationResponse{}, fmt.Errorf("coordination client: not connected")
	}
	body, err := json.Marshal(coordMutationRequest{
		WorkerID:       workerID,
		Operation:      op,
		IdempotencyKey: key,
		Payload:        payload,
	})
	if err != nil {
		return coordMutationResponse{}, err
	}
	path := fmt.Sprintf("%s/api/projects/%s/coordination/mutations", base, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return coordMutationResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return coordMutationResponse{}, &coordTransportError{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return coordMutationResponse{}, &coordTransportError{err: err}
	}
	if resp.StatusCode != http.StatusOK {
		// 5xx / connection-class failures are transport; 4xx are hard.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
			return coordMutationResponse{}, &coordTransportError{
				err: fmt.Errorf("coordination mutate status=%d body=%s", resp.StatusCode, string(raw)),
			}
		}
		return coordMutationResponse{}, fmt.Errorf("coordination mutate status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out coordMutationResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		// Body unreadable after send: unknown-response class.
		return coordMutationResponse{}, &coordTransportError{err: err}
	}
	return out, nil
}

func (c *CoordinationClient) offlineMutate(
	ctx context.Context,
	op, key string,
	payload json.RawMessage,
	offlineApply func(context.Context) (any, error),
) (any, error) {
	var result any
	var applyErr error
	lockErr := c.offline.WithLock(ctx, func() error {
		result, applyErr = offlineApply(ctx)
		if applyErr != nil {
			return applyErr
		}
		outcome := outcomeFromAny(result)
		hash := payloadSHA256(payload)
		_, jerr := c.journal.Append(OfflineJournalAppend{
			Operation:      op,
			IdempotencyKey: key,
			PayloadHash:    hash,
			Payload:        payload,
			Outcome:        outcome,
		})
		return jerr
	})
	if lockErr != nil {
		return nil, lockErr
	}
	return result, nil
}

// Reconcile pushes pending offline journal entries to the server and advances
// the durable acknowledged-through cursor. No-op when offline or empty.
func (c *CoordinationClient) Reconcile(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if c.reconciling {
		c.mu.Unlock()
		return nil
	}
	if !c.connected || c.forceOffline || c.baseURL == "" || c.projectID == "" {
		c.mu.Unlock()
		return nil
	}
	c.reconciling = true
	base := c.baseURL
	projectID := c.projectID
	workerID := c.workerID
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.reconciling = false
		c.mu.Unlock()
	}()

	pending, err := c.journal.ListPending()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	entries := make([]coordReconcileEntry, 0, len(pending))
	for _, rec := range pending {
		entries = append(entries, coordReconcileEntry{
			Sequence:       rec.Sequence,
			Operation:      rec.Operation,
			IdempotencyKey: rec.IdempotencyKey,
			PayloadHash:    rec.PayloadHash,
			Precondition:   rec.Precondition,
			Payload:        rec.Payload,
		})
	}
	body, err := json.Marshal(coordReconcileRequest{WorkerID: workerID, Entries: entries})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/api/projects/%s/coordination/reconcile", base, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &coordTransportError{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return &coordTransportError{err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coordination reconcile status=%d body=%s", resp.StatusCode, string(raw))
	}
	var out coordReconcileResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if out.AcknowledgedThrough > 0 {
		if err := c.journal.AcknowledgeThrough(out.AcknowledgedThrough); err != nil {
			return err
		}
		_ = c.journal.Compact()
	}
	return nil
}

type coordTransportError struct {
	err error
}

func (e *coordTransportError) Error() string {
	if e == nil || e.err == nil {
		return "coordination transport error"
	}
	return e.err.Error()
}

func (e *coordTransportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func isCoordinationTransportError(err error) bool {
	if err == nil {
		return false
	}
	var te *coordTransportError
	return asCoordTransport(err, &te)
}

func asCoordTransport(err error, target **coordTransportError) bool {
	for err != nil {
		if te, ok := err.(*coordTransportError); ok {
			*target = te
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func payloadSHA256(payload json.RawMessage) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func outcomeFromAny(v any) string {
	switch r := v.(type) {
	case coordination.ClaimResult:
		return string(r.Code)
	case coordination.TransitionResult:
		return string(r.Code)
	case coordination.LandResult:
		return string(r.Code)
	case coordMutationResponse:
		return r.Outcome
	default:
		return string(coordination.OutcomeApplied)
	}
}

func landIdempotencyKey(req LandRequest) string {
	if strings.TrimSpace(req.AttemptID) != "" && strings.TrimSpace(req.BeadID) != "" {
		return "land:" + req.BeadID + ":" + req.AttemptID
	}
	h := sha256.Sum256([]byte(req.BeadID + "|" + req.BaseRev + "|" + req.ResultRev))
	return "land:" + hex.EncodeToString(h[:8])
}

func coordFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// KeepCoordinationClientReachable roots the client in the production binary
// graph (deadcode RTA). Production try/work call NewCoordinationClient.
func KeepCoordinationClientReachable() {
	if os.Getenv("DDX_COORDINATION_CLIENT_KEEPALIVE") != "1" {
		return
	}
	_, _ = NewCoordinationClient(os.TempDir(), CoordinationClientConfig{
		Store: &keepaliveClaimStore{},
	})
}

// keepaliveClaimStore is a minimal ClaimBackend for deadcode keepalive only.
type keepaliveClaimStore struct{}

func (s *keepaliveClaimStore) Claim(_, _ string) error { return nil }
func (s *keepaliveClaimStore) Get(context.Context, string) (*bead.Bead, error) {
	return &bead.Bead{Status: bead.StatusOpen}, nil
}
func (s *keepaliveClaimStore) SetLifecycleStatus(_ string, _ string, _ bead.LifecycleTransitionOptions) error {
	return nil
}
