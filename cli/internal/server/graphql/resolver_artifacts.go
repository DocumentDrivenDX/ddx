package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"gopkg.in/yaml.v3"
)

// ddxSidecarFile is the YAML structure of a .ddx.yaml sidecar file.
type ddxSidecarFile struct {
	DDx ddxSidecarMeta `yaml:"ddx"`
}

type ddxSidecarMeta struct {
	ID          string          `yaml:"id"`
	Title       string          `yaml:"title"`
	MediaType   string          `yaml:"media_type"`
	Description string          `yaml:"description"`
	GeneratedBy *ddxGeneratedBy `yaml:"generated_by"`
}

type ddxGeneratedBy struct {
	RunID         string `yaml:"run_id"`
	PromptSummary string `yaml:"prompt_summary"`
	SourceHash    string `yaml:"source_hash"`
}

// Artifacts is the resolver for the artifacts query.
// It returns a paginated, optionally-filtered list of DDx-tracked artifacts
// for the given project. Documents come from the FEAT-007 doc graph; other
// artifacts come from .ddx.yaml sidecar files found under .ddx/plugins/.
func (r *queryResolver) Artifacts(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, mediaType *string, search *string) (*ArtifactConnection, error) {
	root := r.projectRoot(projectID)

	artifacts, err := collectArtifacts(root)
	if err != nil {
		return nil, fmt.Errorf("collecting artifacts: %w", err)
	}

	// Sort by path for stable ordering.
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Path < artifacts[j].Path
	})

	// Apply mediaType filter.
	if mediaType != nil && *mediaType != "" {
		mt := *mediaType
		filtered := artifacts[:0]
		for _, a := range artifacts {
			if a.MediaType == mt {
				filtered = append(filtered, a)
			}
		}
		artifacts = filtered
	}

	// Apply search filter across title and path.
	if search != nil && *search != "" {
		q := strings.ToLower(*search)
		filtered := artifacts[:0]
		for _, a := range artifacts {
			if strings.Contains(strings.ToLower(a.Title), q) ||
				strings.Contains(strings.ToLower(a.Path), q) {
				filtered = append(filtered, a)
			}
		}
		artifacts = filtered
	}

	// Build Relay edges.
	all := make([]*ArtifactEdge, len(artifacts))
	for i, a := range artifacts {
		cp := a
		all[i] = &ArtifactEdge{
			Node:   cp,
			Cursor: encodeStableCursor(cp.ID),
		}
	}

	startIdx := 0
	if after != nil {
		if afterID, ok := decodeStableCursor(*after); ok {
			for i, e := range all {
				if e.Node.ID == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}
	endIdx := len(all)
	if before != nil {
		if beforeID, ok := decodeStableCursor(*before); ok {
			for i, e := range all {
				if e.Node.ID == beforeID {
					endIdx = i
					break
				}
			}
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}

	windowSize := endIdx - startIdx
	slice := all[startIdx:endIdx]
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
	}

	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0 || (last != nil && *last >= 0 && *last < windowSize),
		HasNextPage:     endIdx < len(all) || (first != nil && *first >= 0 && *first < windowSize),
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &ArtifactConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}, nil
}

// collectArtifacts gathers all DDx-tracked artifacts from two sources:
//  1. The doc graph (markdown documents under the project root).
//  2. .ddx.yaml sidecar files under .ddx/plugins/.
func collectArtifacts(root string) ([]*Artifact, error) {
	var artifacts []*Artifact

	// 1. Documents from the doc graph.
	if docs, err := collectDocArtifacts(root); err == nil {
		artifacts = append(artifacts, docs...)
	}

	// 2. Sidecar artifacts from .ddx/plugins/.
	if sidecars, err := collectSidecarArtifacts(root); err == nil {
		artifacts = append(artifacts, sidecars...)
	}

	return artifacts, nil
}

// collectDocArtifacts builds Artifact records from the FEAT-007 doc graph.
func collectDocArtifacts(root string) ([]*Artifact, error) {
	graph, err := docgraph.BuildGraphWithConfig(root)
	if err != nil {
		return nil, err
	}

	staleSet := map[string]string{} // id → staleness ("stale" or "missing")
	for _, entry := range graph.StaleDocs() {
		staleness := "stale"
		for _, reason := range entry.Reasons {
			if strings.Contains(reason, "missing dependency") ||
				strings.Contains(reason, "missing review hash") {
				staleness = "missing"
				break
			}
		}
		staleSet[entry.ID] = staleness
	}

	docs := graph.AllNodesForOutput()
	out := make([]*Artifact, 0, len(docs))
	for _, d := range docs {
		staleness := "fresh"
		if s, ok := staleSet[d.ID]; ok {
			staleness = s
		}

		a := &Artifact{
			ID:        "doc:" + d.ID,
			Path:      d.Path,
			Title:     titleFromDoc(d),
			MediaType: "text/markdown",
			Staleness: staleness,
		}

		// Populate updatedAt from file mod time.
		absPath := d.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(graph.RootDir, absPath)
		}
		if info, statErr := os.Stat(absPath); statErr == nil {
			ts := info.ModTime().UTC().Format(time.RFC3339)
			a.UpdatedAt = &ts
		}

		// Populate ddxFrontmatter as a JSON summary of available metadata.
		frontmatter := map[string]interface{}{
			"id": d.ID,
		}
		if len(d.DependsOn) > 0 {
			frontmatter["depends_on"] = d.DependsOn
		}
		if d.Prompt != "" {
			frontmatter["prompt"] = d.Prompt
		}
		if fm, err := json.Marshal(frontmatter); err == nil {
			s := string(fm)
			a.DdxFrontmatter = &s
		}

		out = append(out, a)
	}
	return out, nil
}

// collectSidecarArtifacts walks .ddx/plugins/ looking for *.ddx.yaml sidecar
// files, then builds an Artifact for each corresponding artifact file.
// Files whose sidecar is missing or unreadable are silently skipped.
func collectSidecarArtifacts(root string) ([]*Artifact, error) {
	pluginsDir := filepath.Join(root, ".ddx", "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var out []*Artifact
	err := filepath.WalkDir(pluginsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".ddx.yaml") {
			return nil
		}

		// Derive artifact path by stripping the .ddx.yaml suffix.
		artifactPath := strings.TrimSuffix(path, ".ddx.yaml")

		// Parse the sidecar.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable sidecars
		}
		var sidecar ddxSidecarFile
		if yamlErr := yaml.Unmarshal(data, &sidecar); yamlErr != nil {
			return nil // skip malformed sidecars
		}

		rel, relErr := filepath.Rel(root, artifactPath)
		if relErr != nil {
			rel = artifactPath
		}

		id := sidecar.DDx.ID
		if id == "" {
			id = "sidecar:" + rel
		}

		title := sidecar.DDx.Title
		if title == "" {
			title = filepath.Base(artifactPath)
		}

		mediaType := sidecar.DDx.MediaType
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}

		staleness := computeSidecarStaleness(artifactPath, sidecar.DDx.GeneratedBy)

		a := &Artifact{
			ID:        id,
			Path:      filepath.ToSlash(rel),
			Title:     title,
			MediaType: mediaType,
			Staleness: staleness,
		}

		if sidecar.DDx.Description != "" {
			desc := sidecar.DDx.Description
			a.Description = &desc
		}

		if sidecar.DDx.GeneratedBy != nil {
			gb := sidecar.DDx.GeneratedBy
			sourceHashMatch := false
			if gb.SourceHash != "" {
				if hash, hashErr := fileSHA256(artifactPath); hashErr == nil {
					sourceHashMatch = (hash == gb.SourceHash)
				}
			}
			a.GeneratedBy = &ArtifactGeneratedBy{
				RunID:           gb.RunID,
				PromptSummary:   gb.PromptSummary,
				SourceHashMatch: sourceHashMatch,
			}
		}

		// Populate ddxFrontmatter from the raw sidecar ddx block.
		if fm, marshalErr := json.Marshal(sidecar.DDx); marshalErr == nil {
			s := string(fm)
			a.DdxFrontmatter = &s
		}

		// Populate updatedAt from artifact file mod time.
		if info, statErr := os.Stat(artifactPath); statErr == nil {
			ts := info.ModTime().UTC().Format(time.RFC3339)
			a.UpdatedAt = &ts
		}

		out = append(out, a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// computeSidecarStaleness returns the staleness of a sidecar artifact.
func computeSidecarStaleness(artifactPath string, gb *ddxGeneratedBy) string {
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		return "missing"
	}
	if gb == nil || gb.SourceHash == "" {
		return "fresh"
	}
	hash, err := fileSHA256(artifactPath)
	if err != nil {
		return "fresh" // can't determine staleness without a hash
	}
	if hash == gb.SourceHash {
		return "fresh"
	}
	return "stale"
}

// fileSHA256 returns the hex-encoded SHA-256 hash of a file's contents.
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// titleFromDoc derives a human-readable title from a docgraph document.
// Uses the Title field if set, otherwise derives from the path's base name.
func titleFromDoc(d docgraph.Document) string {
	if d.Title != "" {
		return d.Title
	}
	base := filepath.Base(d.Path)
	// Strip extension.
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	return base
}

// Artifact is the resolver for the artifact(projectID, id) query.
// It returns a single artifact by ID, loading file content for text-based media types.
func (r *queryResolver) Artifact(ctx context.Context, projectID string, id string) (*Artifact, error) {
	root := r.projectRoot(projectID)
	artifacts, err := collectArtifacts(root)
	if err != nil {
		return nil, fmt.Errorf("collecting artifacts: %w", err)
	}
	for _, a := range artifacts {
		if a.ID == id {
			if isTextMediaType(a.MediaType) {
				a.Content = loadArtifactContent(root, a.Path)
			}
			return a, nil
		}
	}
	return nil, nil
}

// isTextMediaType reports whether the media type has text content suitable
// for inline rendering (markdown, SVG, Excalidraw JSON).
func isTextMediaType(mt string) bool {
	return mt == "text/markdown" || mt == "image/svg+xml" || mt == "application/vnd.excalidraw+json"
}

// loadArtifactContent reads the raw file content for an artifact at the given
// project-relative path. Returns nil if the file cannot be read.
func loadArtifactContent(root, relPath string) *string {
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}
