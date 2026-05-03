package graphql

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// DefaultOperatorPromptCapBytes is the fallback prompt-body cap when the
// resolver was constructed without an explicit PromptCapBytes value. It
// mirrors the inline-prompt cap enforced by the REST /api/agent/run handler
// (evidence.DefaultMaxPromptBytes) so the two write surfaces share a limit.
const DefaultOperatorPromptCapBytes = evidence.DefaultMaxPromptBytes

// CSRFHeaderName is the HTTP header that must carry the server-issued CSRF
// token on operatorPromptSubmit calls (FEAT operator-prompts, Story 15).
const CSRFHeaderName = "X-CSRF-Token"

// OperatorPromptIdempotencyTTL is the window during which duplicate
// idempotency keys deduplicate to the original bead.
const OperatorPromptIdempotencyTTL = 24 * time.Hour

// OperatorPromptEventKind is the BeadEvent.Kind appended to every
// operator-prompt bead's audit log on submit, capturing the originating
// identity (localhost OR ts-net WhoIs).
const OperatorPromptEventKind = "operator_prompt_submit"

// OperatorPromptApproveEventKind is the BeadEvent.Kind appended to an
// operator-prompt bead's audit log when it transitions proposed → open
// (either via operatorPromptApprove or via the auto-approve path on
// operatorPromptSubmit).
const OperatorPromptApproveEventKind = "operator_prompt_approve"

// OperatorPromptCancelEventKind is the BeadEvent.Kind appended to an
// operator-prompt bead's audit log when it transitions proposed → cancelled
// via operatorPromptCancel.
const OperatorPromptCancelEventKind = "operator_prompt_cancel"

// OperatorPromptAllowlistLocalhostSentinel is the sentinel allowlist entry
// that grants any localhost-kind identity auto-approve / approve rights.
// Per the locked Story 15 decision, ts-net identities are never eligible
// regardless of the allowlist contents.
const OperatorPromptAllowlistLocalhostSentinel = "localhost"

// httpRequestKey is the context key used by the server to plumb the
// originating *http.Request into GraphQL resolver calls so the resolver can
// validate CSRF tokens and capture the originating identity.
type httpRequestKey struct{}

// WithHTTPRequest returns ctx with r attached for downstream resolver access.
// The server's GraphQL HTTP handler must call this before delegating to the
// gqlgen handler so resolvers that depend on request headers (CSRF token,
// X-Tailscale-User) work correctly.
func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

// httpRequestFromContext retrieves the *http.Request previously attached via
// WithHTTPRequest, or nil if none was set.
func httpRequestFromContext(ctx context.Context) *http.Request {
	if v, ok := ctx.Value(httpRequestKey{}).(*http.Request); ok {
		return v
	}
	return nil
}

// CSRFTokenStore validates a CSRF token presented on a write request. The
// production implementation stores a per-process token issued at server
// startup and rotated via an explicit endpoint; tests inject a stub.
type CSRFTokenStore interface {
	Validate(token string) bool
}

// IdempotencyCache deduplicates operatorPromptSubmit calls by key within
// OperatorPromptIdempotencyTTL. Lookup returns the original bead ID stored
// under key (and true) when the key was previously seen and has not expired,
// or "", false otherwise. Store records key -> beadID at submit time.
type IdempotencyCache interface {
	Lookup(key string) (string, bool)
	Store(key, beadID string)
}

// MemoryIdempotencyCache is a process-local in-memory IdempotencyCache with
// TTL-based eviction. It is safe for concurrent use.
type MemoryIdempotencyCache struct {
	mu  sync.Mutex
	now func() time.Time
	ttl time.Duration
	m   map[string]idempotencyEntry
}

type idempotencyEntry struct {
	beadID    string
	expiresAt time.Time
}

// NewMemoryIdempotencyCache constructs a cache with the operator-prompt
// default 24h TTL and the real wall clock.
func NewMemoryIdempotencyCache() *MemoryIdempotencyCache {
	return &MemoryIdempotencyCache{
		now: time.Now,
		ttl: OperatorPromptIdempotencyTTL,
		m:   make(map[string]idempotencyEntry),
	}
}

// Lookup returns the bead ID associated with key when present and unexpired.
func (c *MemoryIdempotencyCache) Lookup(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return "", false
	}
	if c.now().After(e.expiresAt) {
		delete(c.m, key)
		return "", false
	}
	return e.beadID, true
}

// Store records key -> beadID with the configured TTL window.
func (c *MemoryIdempotencyCache) Store(key, beadID string) {
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = idempotencyEntry{beadID: beadID, expiresAt: c.now().Add(c.ttl)}
}

// StaticCSRFTokenStore is a single-secret CSRFTokenStore — the token is
// generated once at server startup and validated by constant-time compare.
type StaticCSRFTokenStore struct {
	token string
}

// NewStaticCSRFTokenStore returns a store seeded with a freshly-generated
// 32-byte random token.
func NewStaticCSRFTokenStore() (*StaticCSRFTokenStore, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("csrf: generate token: %w", err)
	}
	return &StaticCSRFTokenStore{token: hex.EncodeToString(buf[:])}, nil
}

// NewStaticCSRFTokenStoreWithToken constructs a store using the supplied
// token (used by tests so a deterministic header value can be sent).
func NewStaticCSRFTokenStoreWithToken(token string) *StaticCSRFTokenStore {
	return &StaticCSRFTokenStore{token: token}
}

// Token returns the stored CSRF token (used by the server to expose it via a
// dedicated endpoint or embed it in served HTML).
func (s *StaticCSRFTokenStore) Token() string { return s.token }

// Validate compares presented to the stored token using a length-equal
// constant-time check.
func (s *StaticCSRFTokenStore) Validate(presented string) bool {
	if presented == "" || len(presented) != len(s.token) {
		return false
	}
	var diff byte
	for i := 0; i < len(s.token); i++ {
		diff |= s.token[i] ^ presented[i]
	}
	return diff == 0
}

// errCSRF is returned by OperatorPromptSubmit when the CSRF check fails.
// The HTTP transport for the GraphQL endpoint surfaces this as a 403; the
// resolver itself only emits the error.
var errCSRF = errors.New("operator-prompts: CSRF token missing or invalid (status: 403)")

// errCrossOrigin is returned by OperatorPromptSubmit when the request carries
// an Origin header whose host portion does not match the request Host. This
// blocks browser cross-origin POSTs even from a trusted-loopback peer (CSRF
// defense in depth alongside the per-session token).
var errCrossOrigin = errors.New("operator-prompts: cross-origin request rejected (status: 403)")

// promptCapError builds the structured cap-error envelope returned by
// operatorPromptSubmit when the prompt body exceeds the configured cap. The
// envelope is surfaced inside `errors[].extensions` so the SvelteKit
// frontend can show observed/cap bytes without parsing the message string.
func promptCapError(observed, cap int) error {
	return &gqlerror.Error{
		Message: fmt.Sprintf(
			"operator-prompts: prompt body exceeds cap: observed %d bytes, cap %d bytes (status: 400)",
			observed, cap,
		),
		Extensions: map[string]any{
			"code":           "PROMPT_TOO_LARGE",
			"status":         400,
			"observed_bytes": observed,
			"cap_bytes":      cap,
			"config_hint":    ".ddx/config.yaml:evidence_caps.max_prompt_bytes",
		},
	}
}

// errAutoApproveDenied is returned by OperatorPromptApprove (and is the
// reason auto-approve gets demoted to a plain proposed bead inside
// OperatorPromptSubmit) when the caller's identity is not on the per-project
// allowlist. ts-net identities always hit this path regardless of allowlist
// contents (locked Story 15 decision: ts-net peers may not blanket
// auto-approve operator prompts).
var errAutoApproveDenied = errors.New("operator-prompts: identity not on per-project auto-approve allowlist (status: 403)")

// errSubmitDenied is returned by OperatorPromptSubmit when the caller is a
// ts-net peer whose identity is not on the per-project allowlist
// (web.operator_prompt.allow_identities). Localhost callers are always
// allowed to submit because they have already cleared the requireTrusted
// loopback gate; ts-net peers carry only network-layer trust, so the
// per-project allowlist is the authorization step. Empty allowlist →
// every ts-net peer is rejected (the locked Story 15 default that yields a
// "ts-net peers see read-only UI" outcome).
var errSubmitDenied = errors.New("operator-prompts: ts-net identity not on per-project allow_identities allowlist (status: 403)")

// OperatorPromptSubmit implements the operatorPromptSubmit mutation: it
// validates the CSRF token from the request header, deduplicates within a
// 24h window by idempotency key, and (on first submission) creates a new
// operator-prompt bead in the proposed status with an audit event capturing
// the originating identity (localhost user OR ts-net WhoIs).
func (r *mutationResolver) OperatorPromptSubmit(ctx context.Context, input OperatorPromptSubmitInput) (*OperatorPromptSubmitResult, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, fmt.Errorf("operator-prompts: prompt is required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, fmt.Errorf("operator-prompts: idempotency key is required")
	}

	httpReq := httpRequestFromContext(ctx)
	if httpReq == nil {
		// No request context means we cannot enforce CSRF, which is the
		// whole point of the gate. Refuse rather than fail open.
		return nil, errCSRF
	}
	if err := validateSameOrigin(httpReq); err != nil {
		return nil, err
	}
	presented := httpReq.Header.Get(CSRFHeaderName)
	if r.CSRFTokens == nil || !r.CSRFTokens.Validate(presented) {
		return nil, errCSRF
	}

	identity := operatorPromptIdentity(httpReq)
	if !r.canSubmit(identity) {
		return nil, errSubmitDenied
	}

	cap := r.PromptCapBytes
	if cap <= 0 {
		cap = DefaultOperatorPromptCapBytes
	}
	if observed := len(input.Prompt); observed > cap {
		return nil, promptCapError(observed, cap)
	}

	cache := r.OperatorPromptIdempotency
	if cache == nil {
		return nil, fmt.Errorf("operator-prompts: idempotency cache not configured")
	}

	store := r.beadStore(ctx)
	if id, ok := cache.Lookup(input.IdempotencyKey); ok {
		existing, err := store.Get(id)
		if err == nil {
			return &OperatorPromptSubmitResult{
				Bead:         beadModelFromBead(existing),
				Deduplicated: true,
			}, nil
		}
		// Cached ID points to a missing bead — fall through and create a
		// fresh bead, overwriting the stale entry below.
	}

	tier := bead.DefaultPriority
	if input.Tier != nil {
		tier = *input.Tier
	}
	b := bead.NewOperatorPromptBead(input.Prompt, tier)
	if err := store.Create(b); err != nil {
		return nil, err
	}

	originNodeID := httpReq.Header.Get("X-Tailscale-Node")
	if originNodeID == "" {
		// Loopback callers do not carry an X-Tailscale-Node header; the
		// origin and the receiving node are the same machine, so record the
		// receiving node ID for symmetry.
		originNodeID = r.NodeID
	}
	buildSHA := r.BuildSHA
	if buildSHA == "" {
		buildSHA = "unknown"
	}
	approvalMode := "manual"
	if input.AutoApprove != nil && *input.AutoApprove {
		approvalMode = "auto-requested"
	}
	requestID := httpReq.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = newRequestID()
	}
	promptHash := sha256.Sum256([]byte(input.Prompt))
	promptSHA := hex.EncodeToString(promptHash[:])
	auditBody := fmt.Sprintf(
		"identity=%s remote_addr=%s tailscale_user=%s tailscale_node=%s "+
			"origin_node_id=%s receiving_node_id=%s build_sha=%s "+
			"approval_mode=%s request_id=%s prompt_sha256=%s "+
			"idempotency_key=%s",
		identity.kind,
		httpReq.RemoteAddr,
		httpReq.Header.Get("X-Tailscale-User"),
		httpReq.Header.Get("X-Tailscale-Node"),
		originNodeID,
		r.NodeID,
		buildSHA,
		approvalMode,
		requestID,
		promptSHA,
		input.IdempotencyKey,
	)
	if err := store.AppendEvent(b.ID, bead.BeadEvent{
		Kind:    OperatorPromptEventKind,
		Summary: "operator prompt submitted",
		Body:    auditBody,
		Actor:   identity.actor,
		Source:  "graphql:operatorPromptSubmit",
	}); err != nil {
		return nil, fmt.Errorf("operator-prompts: append audit event: %w", err)
	}

	cache.Store(input.IdempotencyKey, b.ID)

	// Auto-approve path: only when the caller asked for it AND the identity
	// is on the per-project allowlist. ts-net identities never qualify
	// (locked Story 15 decision). On denial we leave the bead in proposed
	// and return autoApproved=false; the caller can still issue a manual
	// operatorPromptApprove later from a permitted identity.
	autoApproved := false
	if input.AutoApprove != nil && *input.AutoApprove {
		if r.canAutoApprove(identity) {
			if err := store.Update(b.ID, func(updated *bead.Bead) {
				updated.Status = bead.StatusOpen
			}); err != nil {
				return nil, fmt.Errorf("operator-prompts: auto-approve update: %w", err)
			}
			if err := store.AppendEvent(b.ID, bead.BeadEvent{
				Kind:    OperatorPromptApproveEventKind,
				Summary: "operator prompt auto-approved on submit",
				Body:    fmt.Sprintf("identity=%s actor=%s", identity.kind, identity.actor),
				Actor:   identity.actor,
				Source:  "graphql:operatorPromptSubmit",
			}); err != nil {
				return nil, fmt.Errorf("operator-prompts: append auto-approve event: %w", err)
			}
			autoApproved = true
			r.signalExecuteLoopWake()
		}
	}

	// Re-read so the audit event materializes in the returned snapshot.
	persisted, err := store.Get(b.ID)
	if err != nil {
		return nil, err
	}
	return &OperatorPromptSubmitResult{
		Bead:         beadModelFromBead(persisted),
		Deduplicated: false,
		AutoApproved: autoApproved,
	}, nil
}

// OperatorPromptApprove implements the operatorPromptApprove mutation:
// validates CSRF, verifies the bead is a proposed operator-prompt, enforces
// the per-project allowlist (ts-net identities are never eligible per
// locked decision), transitions proposed → open, and appends an audit event
// recording the approver.
func (r *mutationResolver) OperatorPromptApprove(ctx context.Context, id string) (*OperatorPromptApproveResult, error) {
	httpReq, ident, err := r.requireOperatorPromptCSRF(ctx)
	if err != nil {
		return nil, err
	}
	if !r.canAutoApprove(ident) {
		return nil, errAutoApproveDenied
	}

	store := r.beadStore(ctx)
	existing, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if existing.IssueType != bead.IssueTypeOperatorPrompt {
		return nil, fmt.Errorf("operator-prompts: bead %s is not an operator-prompt (issue_type=%q)", id, existing.IssueType)
	}
	if existing.Status != bead.StatusProposed {
		return nil, fmt.Errorf("operator-prompts: bead %s is not in proposed status (got %q); cannot approve", id, existing.Status)
	}
	if err := store.Update(id, func(b *bead.Bead) {
		b.Status = bead.StatusOpen
	}); err != nil {
		return nil, err
	}
	if err := store.AppendEvent(id, bead.BeadEvent{
		Kind:    OperatorPromptApproveEventKind,
		Summary: "operator prompt approved",
		Body:    fmt.Sprintf("identity=%s actor=%s remote_addr=%s", ident.kind, ident.actor, httpReq.RemoteAddr),
		Actor:   ident.actor,
		Source:  "graphql:operatorPromptApprove",
	}); err != nil {
		return nil, fmt.Errorf("operator-prompts: append approve audit event: %w", err)
	}
	persisted, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	r.signalExecuteLoopWake()
	return &OperatorPromptApproveResult{Bead: beadModelFromBead(persisted)}, nil
}

// signalExecuteLoopWake asks any running execute-loop worker bound to this
// project to skip the current idle-poll sleep and re-scan the ready queue.
// Called immediately after a successful proposed → open transition (manual
// approve or auto-approve on submit) so a freshly-eligible operator-prompt
// bead is claimed without waiting a full PollInterval. No-op when the
// waker is unset (e.g. test fixture without a WorkerManager).
func (r *Resolver) signalExecuteLoopWake() {
	if r.ExecuteLoopWaker == nil || r.WorkingDir == "" {
		return
	}
	r.ExecuteLoopWaker.WakeProject(r.WorkingDir)
}

// OperatorPromptCancel implements the operatorPromptCancel mutation:
// validates CSRF, verifies the bead is a proposed operator-prompt,
// transitions proposed → cancelled, and appends an audit event recording
// the canceller. Cancellation is open to any trusted identity (the
// allowlist gates auto-approval, not rejection) so an operator who
// regrets a submission can withdraw it without first being added to the
// approval allowlist.
func (r *mutationResolver) OperatorPromptCancel(ctx context.Context, id string) (*OperatorPromptCancelResult, error) {
	httpReq, ident, err := r.requireOperatorPromptCSRF(ctx)
	if err != nil {
		return nil, err
	}

	store := r.beadStore(ctx)
	existing, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	if existing.IssueType != bead.IssueTypeOperatorPrompt {
		return nil, fmt.Errorf("operator-prompts: bead %s is not an operator-prompt (issue_type=%q)", id, existing.IssueType)
	}
	if existing.Status != bead.StatusProposed {
		return nil, fmt.Errorf("operator-prompts: bead %s is not in proposed status (got %q); cannot cancel", id, existing.Status)
	}
	if err := store.Update(id, func(b *bead.Bead) {
		b.Status = bead.StatusCancelled
	}); err != nil {
		return nil, err
	}
	if err := store.AppendEvent(id, bead.BeadEvent{
		Kind:    OperatorPromptCancelEventKind,
		Summary: "operator prompt cancelled",
		Body:    fmt.Sprintf("identity=%s actor=%s remote_addr=%s", ident.kind, ident.actor, httpReq.RemoteAddr),
		Actor:   ident.actor,
		Source:  "graphql:operatorPromptCancel",
	}); err != nil {
		return nil, fmt.Errorf("operator-prompts: append cancel audit event: %w", err)
	}
	persisted, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return &OperatorPromptCancelResult{Bead: beadModelFromBead(persisted)}, nil
}

// requireOperatorPromptCSRF is the shared preamble for the approve and
// cancel mutations. It pulls the originating *http.Request from context
// (the server's GraphQL handler installs it via WithHTTPRequest), enforces
// the CSRF token check against the configured store, and returns the
// classified identity.
func (r *mutationResolver) requireOperatorPromptCSRF(ctx context.Context) (*http.Request, operatorPromptIdentityInfo, error) {
	if r.workingDir(ctx) == "" {
		return nil, operatorPromptIdentityInfo{}, fmt.Errorf("working directory not configured")
	}
	httpReq := httpRequestFromContext(ctx)
	if httpReq == nil {
		return nil, operatorPromptIdentityInfo{}, errCSRF
	}
	presented := httpReq.Header.Get(CSRFHeaderName)
	if r.CSRFTokens == nil || !r.CSRFTokens.Validate(presented) {
		return nil, operatorPromptIdentityInfo{}, errCSRF
	}
	return httpReq, operatorPromptIdentity(httpReq), nil
}

// canSubmit reports whether ident is authorized to submit operator prompts
// for this project. Localhost identities are always allowed (they have
// already cleared the requireTrusted loopback gate, which is the project's
// strongest trust signal). ts-net identities must appear in the per-project
// OperatorPromptAutoApproveAllowlist (the same list that gates approval —
// per Story 15 the project's "allow_identities" config is one allowlist
// serving both submit-by-tsnet and approve-by-localhost). An empty list
// rejects every ts-net peer, yielding the documented "ts-net peers see
// read-only UI" default.
func (r *Resolver) canSubmit(ident operatorPromptIdentityInfo) bool {
	if ident.kind == "localhost" {
		return true
	}
	if ident.kind != "tsnet" {
		return false
	}
	for _, entry := range r.OperatorPromptAutoApproveAllowlist {
		if entry == ident.actor {
			return true
		}
	}
	return false
}

// canAutoApprove reports whether ident is authorized to approve operator
// prompts under the resolver's allowlist. ts-net identities are denied
// unconditionally (locked Story 15 decision). For localhost identities,
// the literal sentinel "localhost" matches any actor; otherwise the
// allowlist entry must equal the identity's actor string.
func (r *Resolver) canAutoApprove(ident operatorPromptIdentityInfo) bool {
	if ident.kind != "localhost" {
		return false
	}
	if len(r.OperatorPromptAutoApproveAllowlist) == 0 {
		return false
	}
	for _, entry := range r.OperatorPromptAutoApproveAllowlist {
		if entry == OperatorPromptAllowlistLocalhostSentinel {
			return true
		}
		if entry == ident.actor {
			return true
		}
	}
	return false
}

type operatorPromptIdentityInfo struct {
	kind  string // "tsnet" or "localhost"
	actor string
}

// operatorPromptIdentity classifies the originating identity for audit
// logging. ts-net peers are identified by the X-Tailscale-User header
// injected by tsnetMiddleware; loopback callers fall back to a
// "localhost:<remote-addr>" actor string. A bare "anonymous" actor is used
// only when neither signal is present (which should not occur because
// requireTrusted gates the GraphQL endpoint).
func operatorPromptIdentity(r *http.Request) operatorPromptIdentityInfo {
	if user := r.Header.Get("X-Tailscale-User"); user != "" {
		return operatorPromptIdentityInfo{kind: "tsnet", actor: user}
	}
	if r.RemoteAddr != "" {
		return operatorPromptIdentityInfo{kind: "localhost", actor: "localhost:" + r.RemoteAddr}
	}
	return operatorPromptIdentityInfo{kind: "unknown", actor: "anonymous"}
}

// validateSameOrigin enforces strict Origin/Host equality on prompt-driven
// mutations. When the request carries an Origin header (every browser POST
// does), the host portion must match the request Host — this rejects
// browser cross-origin POSTs even from a trusted-loopback peer (defense in
// depth alongside the per-session CSRF token). Non-browser clients (curl,
// the SvelteKit fetch wrapper running same-origin) either omit Origin or
// emit an Origin matching Host, so they pass through unchanged.
func validateSameOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin -> not a browser cross-origin POST. CSRF token is the
		// remaining gate.
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return errCrossOrigin
	}
	if !sameHost(parsed.Host, r.Host) {
		return errCrossOrigin
	}
	return nil
}

// sameHost compares two host[:port] strings tolerating an absent port (a
// browser may omit :443 from Origin while the request Host carries it, or
// vice versa). The comparison is case-insensitive on the host component.
func sameHost(a, b string) bool {
	ah, _ := splitHostPort(a)
	bh, _ := splitHostPort(b)
	if !strings.EqualFold(ah, bh) {
		return false
	}
	// Reject when both sides carry a port and they disagree; tolerate the
	// "one side omits the port" case (default-port elision by the browser).
	_, ap := splitHostPort(a)
	_, bp := splitHostPort(b)
	if ap != "" && bp != "" && ap != bp {
		return false
	}
	return true
}

func splitHostPort(hp string) (string, string) {
	if i := strings.LastIndex(hp, ":"); i > 0 && !strings.Contains(hp[i:], "]") {
		return hp[:i], hp[i+1:]
	}
	return hp, ""
}

// newRequestID returns a 16-byte hex random string used as the request ID
// recorded on the audit event when the caller does not supply X-Request-Id.
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "no-request-id"
	}
	return hex.EncodeToString(buf[:])
}
