package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

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

// DocumentWrite is the resolver for the documentWrite mutation.
func (r *mutationResolver) DocumentWrite(ctx context.Context, path string, content string) (*Document, error) {
	if r.workingDir(ctx) == "" {
		return nil, fmt.Errorf("working directory not configured")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return nil, fmt.Errorf("invalid path: must not contain ..")
	}

	libPath, err := r.libraryPath(ctx)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(libPath, cleaned)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("writing document: %w", err)
	}

	// Rebuild graph to pick up the newly written file and return the document.
	graph, err := docgraph.BuildGraphWithConfig(r.workingDir(ctx))
	if err != nil {
		// File was written; return a minimal document rather than failing.
		return &Document{
			ID:         cleaned,
			Path:       cleaned,
			DependsOn:  []string{},
			Inputs:     []string{},
			Dependents: []string{},
		}, nil
	}

	// Look up the document by its cleaned path.
	if id, ok := graph.PathToID[cleaned]; ok {
		if doc, ok := graph.Documents[id]; ok {
			return docToGQL(*doc), nil
		}
	}

	// File written but not yet in graph (e.g., missing DDx frontmatter).
	return &Document{
		ID:         cleaned,
		Path:       cleaned,
		DependsOn:  []string{},
		Inputs:     []string{},
		Dependents: []string{},
	}, nil
}
