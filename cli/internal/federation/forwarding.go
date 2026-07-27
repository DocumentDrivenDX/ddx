package federation

import (
	"fmt"
	"net/http"
	"strings"
)

// ForwardMutationRequest describes a hub-to-spoke owner-targeted mutation
// envelope. The request is deliberately narrow: it carries the origin and
// forwarding trail plus the single owning node/project pair that is allowed to
// execute the write.
type ForwardMutationRequest struct {
	OriginIdentity  string
	ForwardingPath  []string
	RequestID       string
	IdempotencyKey  string
	TargetNodeID    string
	TargetProjectID string
	ExpectedVersion *string

	// RequiredCapabilities lets the hub declare the minimum spoke capability
	// set needed for this mutation. The writer can reject requests that do not
	// advertise the required surface.
	RequiredCapabilities []string

	// Body and Headers carry the actual mutation envelope to the owning spoke.
	Body    []byte
	Headers map[string]string
}

// ForwardMutationResponse is the owning node's reply to a forwarded mutation
// request. It mirrors the request metadata so callers can correlate a result
// with the original write envelope.
type ForwardMutationResponse struct {
	OriginIdentity  string
	ForwardingPath  []string
	RequestID       string
	IdempotencyKey  string
	TargetNodeID    string
	TargetProjectID string
	ExpectedVersion *string

	RequiredCapabilities []string
	StatusCode           int
	Headers              http.Header
	Body                 []byte
}

// Validate checks the request for the narrow owner-targeted shape. The typed
// refusals are intentionally specific so callers can surface a stable reason.
func (r *ForwardMutationRequest) Validate() error {
	if r == nil {
		return ErrForwardMutationBroadcastLike
	}
	if strings.TrimSpace(r.TargetNodeID) == "" {
		return ErrForwardMutationMissingOwner
	}
	if strings.TrimSpace(r.TargetProjectID) == "" {
		return ErrForwardMutationMissingProject
	}
	if len(r.ForwardingPath) == 0 {
		return ErrForwardMutationBroadcastLike
	}
	return nil
}

// ForwardMutationRefusalKind is the stable refusal class surfaced by the
// federation forwarding contract.
type ForwardMutationRefusalKind string

const (
	ForwardMutationRefusalOffline        ForwardMutationRefusalKind = "offline"
	ForwardMutationRefusalStale          ForwardMutationRefusalKind = "stale"
	ForwardMutationRefusalReadOnly       ForwardMutationRefusalKind = "read_only"
	ForwardMutationRefusalMissingProject ForwardMutationRefusalKind = "missing-project"
	ForwardMutationRefusalMissingOwner   ForwardMutationRefusalKind = "missing-owner"
	ForwardMutationRefusalBroadcastLike  ForwardMutationRefusalKind = "broadcast-like"
)

func (k ForwardMutationRefusalKind) String() string { return string(k) }

// ForwardMutationRefusalError is a typed refusal that lets callers check a
// stable reason with errors.Is while still preserving a human-readable detail.
type ForwardMutationRefusalError struct {
	Kind   ForwardMutationRefusalKind
	Detail string
}

func (e *ForwardMutationRefusalError) Error() string {
	if e == nil {
		return "federation: forward mutation refused"
	}
	if e.Detail == "" {
		return fmt.Sprintf("federation: forward mutation refused: %s", e.Kind)
	}
	return fmt.Sprintf("federation: forward mutation refused: %s: %s", e.Kind, e.Detail)
}

// Is makes errors.Is match on refusal kind rather than pointer identity.
func (e *ForwardMutationRefusalError) Is(target error) bool {
	t, ok := target.(*ForwardMutationRefusalError)
	if !ok || e == nil || t == nil {
		return false
	}
	return e.Kind == t.Kind
}

var (
	ErrForwardMutationOffline        = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalOffline}
	ErrForwardMutationStale          = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalStale}
	ErrForwardMutationReadOnly       = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalReadOnly}
	ErrForwardMutationMissingProject = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalMissingProject}
	ErrForwardMutationMissingOwner   = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalMissingOwner}
	ErrForwardMutationBroadcastLike  = &ForwardMutationRefusalError{Kind: ForwardMutationRefusalBroadcastLike}
)
