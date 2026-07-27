package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// documentWriteSelection is the GraphQL field selection used to round-trip
// documentWrite through federation.
const documentWriteSelection = `{
  id
  path
  title
  content
  dependsOn
  inputs
  dependents
  parkingLot
}`

const documentWriteForwardQuery = "mutation DocumentWrite($path: String!, $content: String!) { documentWrite(path: $path, content: $content) " + documentWriteSelection + " }"

type documentWriteForwardEnvelope struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type documentWriteForwardResponse struct {
	Data struct {
		DocumentWrite *Document `json:"documentWrite,omitempty"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// forwardDocumentWrite marshals a documentWrite GraphQL envelope and routes it
// to the owning spoke via Federation.ForwardMutation. Origin identity,
// forwarding path, and loop-prevention headers are stamped so the spoke never
// re-forwards the write.
func (r *mutationResolver) forwardDocumentWrite(ctx context.Context, owner *federation.SpokeRecord, projectID, path, content string) (*Document, error) {
	if r.Federation == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner == nil || strings.TrimSpace(owner.NodeID) == "" {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	body, err := json.Marshal(documentWriteForwardEnvelope{
		Query: documentWriteForwardQuery,
		Variables: map[string]any{
			"path":    path,
			"content": content,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("document write forward: encode request: %w", err)
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
		return nil, fmt.Errorf("document write forward: empty response")
	}
	if resp.StatusCode != 0 && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
		return nil, fmt.Errorf("document write forward: spoke returned HTTP %d", resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		return nil, fmt.Errorf("document write forward: empty body")
	}

	var decoded documentWriteForwardResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("document write forward: decode response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		msgs := make([]string, 0, len(decoded.Errors))
		for _, e := range decoded.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("document write forward: %s", strings.Join(msgs, "; "))
	}
	if decoded.Data.DocumentWrite == nil {
		return nil, fmt.Errorf("document write forward: missing documentWrite payload")
	}
	return decoded.Data.DocumentWrite, nil
}
