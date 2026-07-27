package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// beadMutationSelection is the common GraphQL field selection used to
// round-trip beadCreate / beadUpdate through federation.
const beadMutationSelection = `{
  id
  title
  status
  priority
  issueType
  owner
  createdAt
  createdBy
  updatedAt
  labels
  projectID
  parent
  description
  acceptance
  notes
  dependencies {
    issueId
    dependsOnId
    type
    createdAt
    createdBy
    metadata
  }
}`

// beadMutationOriginServerHeader is stamped on hub→spoke forwards so the
// receiving node can detect an already-forwarded request and write locally
// rather than re-routing (loop prevention).
const beadMutationOriginServerHeader = "X-DDx-Origin-Server-ID"

type beadMutationForwardEnvelope struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type beadMutationForwardResponse struct {
	Data struct {
		BeadCreate *Bead `json:"beadCreate,omitempty"`
		BeadUpdate *Bead `json:"beadUpdate,omitempty"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// beadMutationIsForwarded reports whether the current request was already
// forwarded by a hub. Forwarded requests carry X-DDx-Origin-Server-ID and
// must write locally rather than re-route.
func beadMutationIsForwarded(ctx context.Context) bool {
	httpReq := httpRequestFromContext(ctx)
	if httpReq == nil {
		return false
	}
	return strings.TrimSpace(httpReq.Header.Get(beadMutationOriginServerHeader)) != ""
}

// beadMutationOwner resolves the owning spoke for the request's working-dir
// project. When the project is local (this node owns it) or federation is
// unset, owner is nil and the caller mutates the local store. When federation
// is set but no registered spoke owns the project, returns
// ErrForwardMutationMissingOwner so the hub never creates a phantom bead.
func (r *mutationResolver) beadMutationOwner(workingDir string) (projectID string, owner *federation.SpokeRecord, err error) {
	if r.Federation == nil {
		return "", nil, nil
	}
	projectID, ok := r.projectIDForWorkingDir(workingDir)
	if !ok || strings.TrimSpace(projectID) == "" {
		return "", nil, nil
	}

	registry := federation.NewRegistry()
	for _, spoke := range r.Federation.Spokes() {
		if err := registry.UpsertSpoke(spoke); err != nil {
			return projectID, nil, err
		}
	}

	owner, err = federation.RouteMutationToProjectOwner(registry, projectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return projectID, nil, federation.ErrForwardMutationBroadcastLike
		case strings.Contains(err.Error(), "no registered spoke owns project"):
			return projectID, nil, federation.ErrForwardMutationMissingOwner
		default:
			return projectID, nil, err
		}
	}
	if owner == nil || strings.TrimSpace(owner.NodeID) == "" {
		return projectID, nil, nil
	}
	// Same-node ownership → local write.
	if strings.TrimSpace(owner.NodeID) == strings.TrimSpace(r.NodeID) {
		return projectID, nil, nil
	}
	return projectID, owner, nil
}

func beadMutationForwardQueryCreate() string {
	return "mutation BeadCreate($input: BeadInput!) { beadCreate(input: $input) " + beadMutationSelection + " }"
}

func beadMutationForwardQueryUpdate() string {
	return "mutation BeadUpdate($id: ID!, $input: BeadUpdateInput!) { beadUpdate(id: $id, input: $input) " + beadMutationSelection + " }"
}

// forwardBeadMutation marshals a beadCreate/beadUpdate GraphQL envelope and
// routes it to the owning spoke via Federation.ForwardMutation. Origin
// identity, forwarding path, and loop-prevention headers are stamped so the
// spoke never re-forwards the write.
func (r *mutationResolver) forwardBeadMutation(ctx context.Context, owner *federation.SpokeRecord, projectID, mutationName, query string, variables map[string]any) (*Bead, error) {
	if r.Federation == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner == nil || strings.TrimSpace(owner.NodeID) == "" {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	body, err := json.Marshal(beadMutationForwardEnvelope{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return nil, fmt.Errorf("bead mutation forward: encode request: %w", err)
	}

	originServerID := strings.TrimSpace(r.NodeID)
	if originServerID == "" {
		originServerID = "unknown"
	}
	forwardPath := []string{originServerID}
	if ownerNodeID := strings.TrimSpace(owner.NodeID); ownerNodeID != "" {
		forwardPath = append(forwardPath, ownerNodeID)
	}

	httpReq := httpRequestFromContext(ctx)
	reqID := beadMutationRequestID(httpReq)
	idemKey := beadMutationIdempotencyKey(httpReq)
	originIdentity := beadMutationOriginIdentity(httpReq, originServerID)

	resp, err := r.Federation.ForwardMutation(ctx, &federation.ForwardMutationRequest{
		OriginIdentity:       originIdentity,
		ForwardingPath:       forwardPath,
		RequestID:            reqID,
		IdempotencyKey:       idemKey,
		TargetNodeID:         strings.TrimSpace(owner.NodeID),
		TargetProjectID:      strings.TrimSpace(projectID),
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			beadMutationOriginServerHeader: originServerID,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("bead mutation forward: empty response")
	}
	if resp.StatusCode != 0 && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
		return nil, fmt.Errorf("bead mutation forward: spoke returned HTTP %d", resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		return nil, fmt.Errorf("bead mutation forward: empty body")
	}

	var decoded beadMutationForwardResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("bead mutation forward: decode response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		msgs := make([]string, 0, len(decoded.Errors))
		for _, e := range decoded.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("bead mutation forward: %s", strings.Join(msgs, "; "))
	}

	switch mutationName {
	case "beadCreate":
		if decoded.Data.BeadCreate == nil {
			return nil, fmt.Errorf("bead mutation forward: missing beadCreate payload")
		}
		return decoded.Data.BeadCreate, nil
	case "beadUpdate":
		if decoded.Data.BeadUpdate == nil {
			return nil, fmt.Errorf("bead mutation forward: missing beadUpdate payload")
		}
		return decoded.Data.BeadUpdate, nil
	default:
		return nil, fmt.Errorf("bead mutation forward: unknown mutation %q", mutationName)
	}
}

func beadMutationRequestID(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
	}
	if v := strings.TrimSpace(httpReq.Header.Get("X-DDx-Request-ID")); v != "" {
		return v
	}
	return strings.TrimSpace(httpReq.Header.Get("X-Request-Id"))
}

func beadMutationIdempotencyKey(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
	}
	return strings.TrimSpace(httpReq.Header.Get("X-DDx-Idempotency-Key"))
}

func beadMutationOriginIdentity(httpReq *http.Request, fallback string) string {
	if httpReq != nil {
		if v := strings.TrimSpace(httpReq.Header.Get("X-DDx-Origin-Identity")); v != "" {
			return v
		}
		if node := strings.TrimSpace(httpReq.Header.Get("X-Tailscale-Node")); node != "" {
			return node
		}
		if user := strings.TrimSpace(httpReq.Header.Get("X-Tailscale-User")); user != "" {
			return user
		}
		if httpReq.RemoteAddr != "" {
			return "localhost:" + httpReq.RemoteAddr
		}
	}
	return fallback
}
