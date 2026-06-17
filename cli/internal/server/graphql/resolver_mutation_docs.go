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

// DocumentWrite is the resolver for the documentWrite mutation.
func (r *mutationResolver) DocumentWrite(ctx context.Context, path string, content string, expectedHash *string) (*Document, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Hub-mode: forward spoke-owned document writes to the owning spoke.
	owner, projectID, forwardErr := r.documentWriteForwardTarget(r.workingDir(ctx))
	if forwardErr != nil {
		return nil, documentWriteForwardError(forwardErr)
	}
	if owner != nil {
		return r.forwardDocumentWriteMutation(ctx, owner, projectID, path, content, expectedHash)
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

// documentWriteMutationDocument is the GraphQL envelope forwarded to the owning
// spoke for hub-mode spoke-owned document writes.
const documentWriteMutationDocument = `mutation DocumentWrite($path:String!,$content:String!,$expectedHash:String){documentWrite(path:$path,content:$content,expectedHash:$expectedHash){id path title content dependsOn inputs dependents}}`

type documentWriteForwardEnvelope struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type documentWriteForwardResponse struct {
	Data struct {
		DocumentWrite *Document `json:"documentWrite,omitempty"`
	} `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions,omitempty"`
	} `json:"errors,omitempty"`
}

// documentWriteForwardTarget returns the spoke that owns the current working
// dir's project in hub mode, or nil when the write should be local.
func (r *mutationResolver) documentWriteForwardTarget(workingDir string) (*federation.SpokeRecord, string, error) {
	if r.Federation == nil || workingDir == "" {
		return nil, "", nil
	}
	projectID, ok := r.projectIDForWorkingDir(workingDir)
	if !ok || projectID == "" {
		return nil, "", nil
	}
	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, r.Federation.Spokes()...)
	owner, err := federation.RouteMutationToProjectOwner(registry, projectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return nil, projectID, federation.ErrForwardMutationBroadcastLike
		case strings.Contains(err.Error(), "no registered spoke owns"):
			return nil, projectID, federation.ErrForwardMutationMissingOwner
		default:
			return nil, projectID, err
		}
	}
	if owner == nil || strings.TrimSpace(owner.NodeID) == "" {
		return nil, projectID, nil
	}
	if strings.TrimSpace(owner.NodeID) == strings.TrimSpace(r.NodeID) {
		return nil, projectID, nil
	}
	return owner, projectID, nil
}

// forwardDocumentWriteMutation forwards a documentWrite mutation to the spoke
// that owns the target project, carrying origin identity, forwarding path,
// request ID, idempotency key, target node/project IDs, and expected version.
func (r *mutationResolver) forwardDocumentWriteMutation(ctx context.Context, owner *federation.SpokeRecord, projectID, path, content string, expectedHash *string) (*Document, error) {
	vars := map[string]any{
		"path":    path,
		"content": content,
	}
	if expectedHash != nil {
		vars["expectedHash"] = *expectedHash
	}
	body, err := json.Marshal(documentWriteForwardEnvelope{
		Query:     documentWriteMutationDocument,
		Variables: vars,
	})
	if err != nil {
		return nil, fmt.Errorf("document write forward: encode request: %w", err)
	}

	originIdentity := strings.TrimSpace(r.NodeID)
	forwardPath := []string{}
	if originIdentity != "" {
		forwardPath = append(forwardPath, originIdentity)
	}
	if nodeID := strings.TrimSpace(owner.NodeID); nodeID != "" {
		forwardPath = append(forwardPath, nodeID)
	}

	requestID, idempotencyKey := "", ""
	if httpReq := httpRequestFromContext(ctx); httpReq != nil {
		if v := strings.TrimSpace(httpReq.Header.Get("X-DDx-Request-ID")); v != "" {
			requestID = v
		} else if v := strings.TrimSpace(httpReq.Header.Get("X-Request-Id")); v != "" {
			requestID = v
		}
		idempotencyKey = strings.TrimSpace(httpReq.Header.Get("X-DDx-Idempotency-Key"))
	}

	resp, err := r.Federation.ForwardMutation(ctx, &federation.ForwardMutationRequest{
		OriginIdentity:       originIdentity,
		ForwardingPath:       forwardPath,
		RequestID:            requestID,
		IdempotencyKey:       idempotencyKey,
		TargetNodeID:         strings.TrimSpace(owner.NodeID),
		TargetProjectID:      projectID,
		ExpectedVersion:      expectedHash,
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers:              map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return nil, documentWriteForwardError(err)
	}
	if resp == nil {
		return nil, fmt.Errorf("document write forward: empty response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("document write forward: spoke returned HTTP %d", resp.StatusCode)
	}
	var decoded documentWriteForwardResponse
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("document write forward: decode response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		first := decoded.Errors[0]
		if len(first.Extensions) > 0 {
			return nil, &gqlerror.Error{Message: first.Message, Extensions: first.Extensions}
		}
		return nil, fmt.Errorf("document write forward: %s", first.Message)
	}
	if decoded.Data.DocumentWrite == nil {
		return nil, fmt.Errorf("document write forward: missing documentWrite payload")
	}
	return decoded.Data.DocumentWrite, nil
}

func documentWriteForwardError(err error) error {
	switch {
	case errors.Is(err, federation.ErrForwardMutationMissingOwner):
		return documentWriteRefusalError("DOCUMENT_WRITE_TARGET_NOT_OWNED", err.Error())
	case errors.Is(err, federation.ErrForwardMutationBroadcastLike):
		return documentWriteRefusalError("DOCUMENT_WRITE_TARGET_AMBIGUOUS", err.Error())
	case errors.Is(err, federation.ErrForwardMutationReadOnly):
		return documentWriteRefusalError("DOCUMENT_WRITE_TARGET_READ_ONLY", err.Error())
	case errors.Is(err, federation.ErrForwardMutationOffline):
		return documentWriteRefusalError("DOCUMENT_WRITE_TARGET_OFFLINE", err.Error())
	case errors.Is(err, federation.ErrForwardMutationStale):
		return documentWriteRefusalError("DOCUMENT_WRITE_TARGET_STALE", err.Error())
	default:
		return fmt.Errorf("document write forward: %w", err)
	}
}

func documentWriteRefusalError(code, message string) error {
	return &gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code": code,
		},
	}
}
