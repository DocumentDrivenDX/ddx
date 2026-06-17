package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// documentWriteWriteFile is a test seam so refusal paths can be exercised
// without depending on host-specific filesystem failures.
var documentWriteWriteFile = os.WriteFile

// libraryPath resolves the configured library path relative to the per-request
// working dir (from ctx via WithWorkingDir, falling back to r.WorkingDir).
func (r *mutationResolver) libraryPath(ctx context.Context) (string, error) {
	wd := r.workingDir(ctx)
	cfg, err := config.LoadWithWorkingDir(wd)
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.Library == nil || cfg.Library.Path == "" {
		return "", fmt.Errorf("library not configured")
	}
	p := cfg.Library.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(wd, p)
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("library path not found: %w", err)
	}
	return p, nil
}

// documentWriteConflictError returns the GraphQL conflict envelope used when a
// caller supplies a stale expected hash for a document write.
func documentWriteConflictError() error {
	return &gqlerror.Error{
		Message: "document write conflict: expected hash does not match current content",
		Extensions: map[string]any{
			"code":   "DOCUMENT_WRITE_CONFLICT",
			"status": 409,
		},
	}
}

// documentWriteContentHash returns the current content hash for a file on disk.
// Missing files are reported with exists=false so create-vs-update writes can
// distinguish "new document" from "stale document" without conflating the two.
func documentWriteContentHash(path string) (hash string, exists bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true, nil
}

// documentWriteIsForwarded reports whether the current request was already
// forwarded by a hub. A forwarded request carries the X-DDx-Origin-Server-ID
// header, which serves as the loop-prevention signal: spokes that receive a
// forwarded documentWrite must write locally rather than re-routing.
func documentWriteIsForwarded(ctx context.Context) bool {
	httpReq := httpRequestFromContext(ctx)
	if httpReq == nil {
		return false
	}
	return strings.TrimSpace(httpReq.Header.Get("X-DDx-Origin-Server-ID")) != ""
}

// documentWriteForwardTarget resolves the owning spoke for the current
// request's working-dir project. Returns (projectID, owner, nil) when the
// write should be forwarded to owner; returns (projectID, nil, nil) when the
// write is local; returns a non-nil error only when the registry is
// unambiguously broken (e.g. multiple owners).
func (r *mutationResolver) documentWriteForwardTarget(ctx context.Context) (string, *federation.SpokeRecord, error) {
	if r.Federation == nil {
		return "", nil, nil
	}
	wd := r.workingDir(ctx)
	projectID, ok := r.projectIDForWorkingDir(wd)
	if !ok || projectID == "" {
		return "", nil, nil
	}

	registry := federation.NewRegistry()
	for _, spoke := range r.Federation.Spokes() {
		if err := registry.UpsertSpoke(spoke); err != nil {
			return projectID, nil, err
		}
	}

	owner, err := federation.RouteMutationToProjectOwner(registry, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "multiple registered owners") {
			return projectID, nil, federation.ErrForwardMutationBroadcastLike
		}
		// "no registered spoke owns" → write locally; not an error.
		return projectID, nil, nil
	}
	if owner == nil || strings.TrimSpace(owner.NodeID) == "" {
		return projectID, nil, nil
	}
	// If the owning spoke IS this node, write locally.
	if strings.TrimSpace(owner.NodeID) == strings.TrimSpace(r.NodeID) {
		return projectID, nil, nil
	}
	return projectID, owner, nil
}

const documentWriteForwardMutation = `mutation DocumentWrite($path: String!, $content: String!, $expectedHash: String) {
  documentWrite(path: $path, content: $content, expectedHash: $expectedHash) {
    id path title content dependsOn inputs dependents parkingLot
  }
}`

type documentWriteForwardResponseEnvelope struct {
	Data struct {
		DocumentWrite *Document `json:"documentWrite,omitempty"`
	} `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions,omitempty"`
	} `json:"errors,omitempty"`
}

// forwardDocumentWrite routes a documentWrite mutation to the spoke that owns
// the target project. The hub's origin identity is included in the request
// headers (X-DDx-Origin-Server-ID) so the spoke can detect and reject
// recursive forwarding loops.
func (r *mutationResolver) forwardDocumentWrite(ctx context.Context, owner *federation.SpokeRecord, projectID, path, content string, expectedHash *string) (*Document, error) {
	if r.Federation == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	vars := map[string]any{
		"path":    path,
		"content": content,
	}
	if expectedHash != nil {
		vars["expectedHash"] = *expectedHash
	}

	body, err := json.Marshal(map[string]any{
		"query":     documentWriteForwardMutation,
		"variables": vars,
	})
	if err != nil {
		return nil, fmt.Errorf("document write forward: encode request: %w", err)
	}

	originServerID := strings.TrimSpace(r.NodeID)
	forwardPath := make([]string, 0, 2)
	if originServerID != "" {
		forwardPath = append(forwardPath, originServerID)
	}
	if ownerNodeID := strings.TrimSpace(owner.NodeID); ownerNodeID != "" {
		forwardPath = append(forwardPath, ownerNodeID)
	}

	resp, err := r.Federation.ForwardMutation(ctx, &federation.ForwardMutationRequest{
		OriginIdentity:       originServerID,
		ForwardingPath:       forwardPath,
		TargetNodeID:         strings.TrimSpace(owner.NodeID),
		TargetProjectID:      strings.TrimSpace(projectID),
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers: map[string]string{
			"Content-Type":           "application/json",
			"X-DDx-Origin-Server-ID": originServerID,
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

	var decoded documentWriteForwardResponseEnvelope
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("document write forward: decode response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		first := decoded.Errors[0]
		if code, ok := first.Extensions["code"].(string); ok && code == "DOCUMENT_WRITE_CONFLICT" {
			return nil, documentWriteConflictError()
		}
		return nil, fmt.Errorf("document write forward: %s", first.Message)
	}
	if decoded.Data.DocumentWrite == nil {
		return nil, fmt.Errorf("document write forward: missing documentWrite payload")
	}
	return decoded.Data.DocumentWrite, nil
}

// DocumentWrite is the resolver for the documentWrite mutation.
func (r *mutationResolver) DocumentWrite(ctx context.Context, path string, content string, expectedHash *string) (*Document, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// In hub mode, forward the write to the owning spoke unless this request
	// was already forwarded by a hub (loop prevention via X-DDx-Origin-Server-ID).
	if !documentWriteIsForwarded(ctx) {
		projectID, owner, err := r.documentWriteForwardTarget(ctx)
		if err != nil {
			return nil, err
		}
		if owner != nil {
			return r.forwardDocumentWrite(ctx, owner, projectID, path, content, expectedHash)
		}
	}

	libPath, err := r.libraryPath(ctx)
	if err != nil {
		return nil, err
	}

	fullPath, err := ResolveDocumentPath(libPath, path)
	if err != nil {
		return nil, fmt.Errorf("invalid path")
	}

	if expectedHash != nil && strings.TrimSpace(*expectedHash) != "" {
		currentHash, exists, err := documentWriteContentHash(fullPath)
		if err != nil {
			return nil, fmt.Errorf("reading current document: %w", err)
		}
		if !exists || currentHash != strings.TrimSpace(*expectedHash) {
			return nil, documentWriteConflictError()
		}
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}
	if err := documentWriteWriteFile(fullPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("writing document: %w", err)
	}

	// Compute relative path for graph lookup.
	cleaned, _ := filepath.Rel(libPath, fullPath)

	// Re-read through the document query path so the mutation returns the same
	// fresh document state the UI would see on an immediate follow-up read.
	return (&queryResolver{r.Resolver}).DocumentByPath(ctx, cleaned)
}
