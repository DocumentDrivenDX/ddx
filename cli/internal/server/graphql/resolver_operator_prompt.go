package graphql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

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

// OperatorPromptSubmit implements the operatorPromptSubmit mutation: it
// validates the CSRF token from the request header, deduplicates within a
// 24h window by idempotency key, and (on first submission) creates a new
// operator-prompt bead in the proposed status with an audit event capturing
// the originating identity (localhost user OR ts-net WhoIs).
func (r *mutationResolver) OperatorPromptSubmit(ctx context.Context, input OperatorPromptSubmitInput) (*OperatorPromptSubmitResult, error) {
	if r.WorkingDir == "" {
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
	presented := httpReq.Header.Get(CSRFHeaderName)
	if r.CSRFTokens == nil || !r.CSRFTokens.Validate(presented) {
		return nil, errCSRF
	}

	cache := r.OperatorPromptIdempotency
	if cache == nil {
		return nil, fmt.Errorf("operator-prompts: idempotency cache not configured")
	}

	store := r.beadStore()
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

	identity := operatorPromptIdentity(httpReq)
	auditBody := fmt.Sprintf(
		"identity=%s remote_addr=%s tailscale_user=%s tailscale_node=%s idempotency_key=%s",
		identity.kind,
		httpReq.RemoteAddr,
		httpReq.Header.Get("X-Tailscale-User"),
		httpReq.Header.Get("X-Tailscale-Node"),
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

	// Re-read so the audit event materializes in the returned snapshot.
	persisted, err := store.Get(b.ID)
	if err != nil {
		return nil, err
	}
	return &OperatorPromptSubmitResult{
		Bead:         beadModelFromBead(persisted),
		Deduplicated: false,
	}, nil
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
