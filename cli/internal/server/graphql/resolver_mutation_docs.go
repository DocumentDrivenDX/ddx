package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
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
