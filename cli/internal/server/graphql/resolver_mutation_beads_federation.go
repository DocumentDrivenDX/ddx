package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const beadLifecycleMutationSelection = `id title status priority issueType owner createdAt createdBy updatedAt labels projectID parent description acceptance notes`

type beadLifecycleGraphQLError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type beadLifecycleGraphQLResponse struct {
	Data   map[string]json.RawMessage  `json:"data"`
	Errors []beadLifecycleGraphQLError `json:"errors"`
}

func beadLifecycleValidationError(code, message string) error {
	return &gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code":   code,
			"status": http.StatusBadRequest,
		},
	}
}

func beadLifecycleRefusalError(code, message string) error {
	return &gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code": code,
		},
	}
}

func beadModelFromBeadWithProject(b *bead.Bead, projectID string) *Bead {
	if b == nil {
		return nil
	}
	gql := beadModelFromBead(b)
	if projectID != "" && gql.ProjectID == nil {
		gql.ProjectID = &projectID
	}
	return gql
}

func (r *mutationResolver) beadLifecycleForwardTarget(projectID string) (*federation.SpokeRecord, error) {
	if r.Federation == nil || strings.TrimSpace(projectID) == "" {
		return nil, nil
	}

	reg := federation.NewRegistry()
	for _, spoke := range r.Federation.Spokes() {
		if err := reg.UpsertSpoke(spoke); err != nil {
			return nil, beadLifecycleRefusalError("BEAD_LIFECYCLE_FORWARD_REGISTRY_INVALID", err.Error())
		}
	}

	target, err := federation.RouteMutationToProjectOwner(reg, projectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return nil, beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_AMBIGUOUS", err.Error())
		case strings.Contains(err.Error(), "no registered spoke owns"):
			return nil, beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_NOT_OWNED", err.Error())
		default:
			return nil, beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_UNKNOWN", err.Error())
		}
	}
	if target == nil {
		return nil, nil
	}
	if strings.TrimSpace(target.NodeID) == "" {
		return nil, beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_UNKNOWN", "federation: target node missing")
	}
	if strings.TrimSpace(target.NodeID) == strings.TrimSpace(r.NodeID) {
		return nil, nil
	}
	return target, nil
}

func (r *mutationResolver) beadLifecycleRequestIdentity(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
	}
	if origin := strings.TrimSpace(httpReq.Header.Get("X-DDx-Origin-Identity")); origin != "" {
		return origin
	}
	return beadLifecycleTransportIdentity(httpReq)
}

func (r *mutationResolver) beadLifecycleRequestHeaders(httpReq *http.Request) map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if httpReq == nil {
		return headers
	}
	for k, values := range httpReq.Header {
		if len(values) == 0 {
			continue
		}
		if strings.EqualFold(k, "X-DDx-Origin-Identity") || strings.EqualFold(k, "X-DDx-Coordinator-Identity") {
			continue
		}
		headers[k] = strings.Join(values, ",")
	}
	return headers
}

func (r *mutationResolver) beadLifecycleRequestID(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
	}
	if v := strings.TrimSpace(httpReq.Header.Get("X-DDx-Request-ID")); v != "" {
		return v
	}
	return strings.TrimSpace(httpReq.Header.Get("X-Request-Id"))
}

func (r *mutationResolver) beadLifecycleIdempotencyKey(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
	}
	return strings.TrimSpace(httpReq.Header.Get("X-DDx-Idempotency-Key"))
}

func (r *mutationResolver) beadLifecycleCoordinatorIdentity(httpReq *http.Request) string {
	if id := strings.TrimSpace(r.NodeID); id != "" {
		return id
	}
	if httpReq == nil {
		return ""
	}
	if id := beadLifecycleTransportIdentity(httpReq); id != "" {
		return id
	}
	return "unknown"
}

func beadLifecycleTransportIdentity(httpReq *http.Request) string {
	if httpReq == nil {
		return ""
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
	return ""
}

func (r *mutationResolver) beadLifecycleAuditBody(httpReq *http.Request, fields map[string]string) string {
	lines := make([]string, 0, len(fields)+5)
	if v := r.beadLifecycleRequestIdentity(httpReq); v != "" {
		lines = append(lines, "origin_identity="+v)
	}
	if v := strings.TrimSpace(httpReqHeader(httpReq, "X-DDx-Coordinator-Identity")); v != "" {
		lines = append(lines, "coordinator_identity="+v)
	}
	if v := strings.TrimSpace(httpReqHeader(httpReq, "X-DDx-Forwarding-Path")); v != "" {
		lines = append(lines, "forwarding_path="+v)
	}
	if v := r.beadLifecycleRequestID(httpReq); v != "" {
		lines = append(lines, "request_id="+v)
	}
	if v := r.beadLifecycleIdempotencyKey(httpReq); v != "" {
		lines = append(lines, "idempotency_key="+v)
	}
	for _, key := range []string{"note", "reason", "external_blocker_reason", "details"} {
		if v := strings.TrimSpace(fields[key]); v != "" {
			lines = append(lines, key+"="+v)
		}
	}
	for _, key := range []string{"action"} {
		if v := strings.TrimSpace(fields[key]); v != "" {
			lines = append(lines, key+"="+v)
		}
	}
	return strings.Join(lines, "\n")
}

func httpReqHeader(httpReq *http.Request, key string) string {
	if httpReq == nil {
		return ""
	}
	return httpReq.Header.Get(key)
}

func (r *mutationResolver) appendBeadLifecycleAuditEvent(ctx context.Context, store *bead.Store, beadID, summary, source string, fields map[string]string) error {
	httpReq := httpRequestFromContext(ctx)
	body := r.beadLifecycleAuditBody(httpReq, fields)
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "human-resolution",
		Summary: summary,
		Body:    body,
		Actor:   "operator",
		Source:  source,
	})
}

func (r *mutationResolver) forwardBeadLifecycleMutation(ctx context.Context, projectID, mutationName, responseField string, variables map[string]any) (*Bead, bool, error) {
	target, err := r.beadLifecycleForwardTarget(projectID)
	if err != nil {
		return nil, false, err
	}
	if target == nil {
		return nil, false, nil
	}

	httpReq := httpRequestFromContext(ctx)
	originIdentity := r.beadLifecycleRequestIdentity(httpReq)
	if originIdentity == "" {
		originIdentity = "unknown"
	}
	query := beadLifecycleMutationDocument(mutationName, responseField)
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, false, fmt.Errorf("graphql: marshal forwarded bead mutation body: %w", err)
	}

	req := &federation.ForwardMutationRequest{
		OriginIdentity:       originIdentity,
		ForwardingPath:       []string{r.beadLifecycleCoordinatorIdentity(httpReq), target.NodeID},
		RequestID:            r.beadLifecycleRequestID(httpReq),
		IdempotencyKey:       r.beadLifecycleIdempotencyKey(httpReq),
		TargetNodeID:         target.NodeID,
		TargetProjectID:      projectID,
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers:              r.beadLifecycleRequestHeaders(httpReq),
	}

	resp, err := r.Federation.ForwardMutation(ctx, req)
	if err != nil {
		return nil, false, beadLifecycleForwardError(err)
	}
	if resp == nil {
		return nil, false, fmt.Errorf("graphql: forwarded bead mutation returned no response")
	}

	beadModel, err := beadLifecycleBeadFromForwardResponse(resp, responseField, projectID)
	if err != nil {
		return nil, false, err
	}
	return beadModel, true, nil
}

func beadLifecycleForwardError(err error) error {
	switch {
	case errors.Is(err, federation.ErrForwardMutationMissingOwner):
		return beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_NOT_OWNED", err.Error())
	case errors.Is(err, federation.ErrForwardMutationBroadcastLike):
		return beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_AMBIGUOUS", err.Error())
	case errors.Is(err, federation.ErrForwardMutationReadOnly):
		return beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_READ_ONLY", err.Error())
	case errors.Is(err, federation.ErrForwardMutationOffline):
		return beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_OFFLINE", err.Error())
	case errors.Is(err, federation.ErrForwardMutationStale):
		return beadLifecycleRefusalError("BEAD_LIFECYCLE_TARGET_STALE", err.Error())
	default:
		return fmt.Errorf("graphql: forward bead lifecycle mutation: %w", err)
	}
}

func beadLifecycleMutationDocument(mutationName, responseField string) string {
	switch mutationName {
	case "beadApprove":
		return `mutation BeadApprove($id: ID!, $note: String!) {
  beadApprove(id: $id, note: $note) {
    ` + beadLifecycleMutationSelection + `
  }
}`
	case "beadCancel":
		return `mutation BeadCancel($id: ID!, $reason: String!) {
  beadCancel(id: $id, reason: $reason) {
    ` + beadLifecycleMutationSelection + `
  }
}`
	case "beadBlock":
		return `mutation BeadBlock($id: ID!, $externalBlockerReason: String!) {
  beadBlock(id: $id, externalBlockerReason: $externalBlockerReason) {
    ` + beadLifecycleMutationSelection + `
  }
}`
	case "beadReopen":
		return `mutation BeadReopen($id: ID!) {
  beadReopen(id: $id) {
    ` + beadLifecycleMutationSelection + `
  }
}`
	default:
		return fmt.Sprintf("mutation %s($id: ID!) { %s(id: $id) { %s } }", mutationName, responseField, beadLifecycleMutationSelection)
	}
}

func beadLifecycleBeadFromForwardResponse(resp *federation.ForwardMutationResponse, responseField, projectID string) (*Bead, error) {
	if resp == nil {
		return nil, fmt.Errorf("graphql: forwarded bead mutation returned no body")
	}
	var parsed beadLifecycleGraphQLResponse
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("graphql: decode forwarded bead mutation response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		first := parsed.Errors[0]
		if len(first.Extensions) > 0 {
			return nil, &gqlerror.Error{Message: first.Message, Extensions: first.Extensions}
		}
		return nil, fmt.Errorf("%s", first.Message)
	}
	raw, ok := parsed.Data[responseField]
	if !ok {
		return nil, fmt.Errorf("graphql: forwarded bead mutation missing %s payload", responseField)
	}
	var out Bead
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("graphql: decode forwarded bead payload: %w", err)
	}
	if out.ProjectID == nil && projectID != "" {
		out.ProjectID = &projectID
	}
	return &out, nil
}

func beadLifecycleProjectPathFromSnapshot(ctx context.Context, r *Resolver, beadID string) (projectID string, projectRoot string, remote bool) {
	if r != nil {
		projectRoot = r.workingDir(ctx)
	}
	if r == nil || r.State == nil || strings.TrimSpace(beadID) == "" {
		return "", projectRoot, false
	}
	snap, ok := r.State.GetBeadSnapshot(beadID)
	if !ok || snap == nil {
		return "", projectRoot, false
	}
	projectID = strings.TrimSpace(snap.ProjectID)
	if projectID == "" {
		return "", projectRoot, false
	}
	if proj, ok := r.State.GetProjectSnapshotByID(projectID); ok && strings.TrimSpace(proj.Path) != "" {
		return projectID, proj.Path, false
	}
	if r.Federation != nil {
		return projectID, projectRoot, true
	}
	return projectID, projectRoot, false
}

func beadLifecycleSetProjectID(gql *Bead, projectID string) *Bead {
	if gql != nil && projectID != "" && gql.ProjectID == nil {
		gql.ProjectID = &projectID
	}
	return gql
}
