package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"gopkg.in/yaml.v3"
)

// searchBodySizeCapBytes is the maximum number of bytes per file scanned
// during body search. See TD-029: the cap keeps the linear-scan O(N) safe
// without an inverted index.
const searchBodySizeCapBytes = 256 * 1024

// snippetWindowChars controls the leading/trailing context around a body
// match in the rendered snippet. The window is character-based; the marker
// "**…**" wraps the matched substring.
const snippetWindowChars = 60

// binarySearchExtensions lists file extensions we never read for body
// search. Per TD-029 these are skipped *before* opening the file. SVG and
// Excalidraw JSON are deliberately treated as text and are NOT in this set.
var binarySearchExtensions = map[string]struct{}{
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".webp":  {},
	".ico":   {},
	".pdf":   {},
	".zip":   {},
	".tar":   {},
	".gz":    {},
	".bz2":   {},
	".xz":    {},
	".7z":    {},
	".mp3":   {},
	".mp4":   {},
	".mov":   {},
	".avi":   {},
	".webm":  {},
	".wasm":  {},
	".so":    {},
	".dylib": {},
	".dll":   {},
	".exe":   {},
	".bin":   {},
	".o":     {},
	".a":     {},
	".class": {},
	".jar":   {},
	".ttf":   {},
	".otf":   {},
	".woff":  {},
	".woff2": {},
}

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
func (r *queryResolver) Artifacts(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, mediaType *string, search *string, sortKey *ArtifactSort, staleness *string, phase *string, prefix []string) (*ArtifactConnection, error) {
	root := r.projectRoot(ctx, projectID)

	artifacts, err := collectArtifacts(root)
	if err != nil {
		return nil, fmt.Errorf("collecting artifacts: %w", err)
	}

	// Apply mediaType filter (supports X/* wildcard suffix).
	if mediaType != nil && *mediaType != "" {
		mt := *mediaType
		filtered := artifacts[:0]
		if strings.HasSuffix(mt, "/*") {
			prefix := strings.TrimSuffix(mt, "*") // keep trailing slash
			for _, a := range artifacts {
				if strings.HasPrefix(a.MediaType, prefix) {
					filtered = append(filtered, a)
				}
			}
		} else {
			for _, a := range artifacts {
				if a.MediaType == mt {
					filtered = append(filtered, a)
				}
			}
		}
		artifacts = filtered
	}

	// Apply staleness filter.
	if staleness != nil && *staleness != "" {
		st := *staleness
		filtered := artifacts[:0]
		for _, a := range artifacts {
			if a.Staleness == st {
				filtered = append(filtered, a)
			}
		}
		artifacts = filtered
	}

	// Apply phase filter: matches when path is under docs/helix/<phase>/
	// where <phase> is either an exact match (e.g. "01-frame") or a numeric
	// prefix (e.g. "01") matching docs/helix/01-*/.
	if phase != nil && *phase != "" {
		ph := *phase
		filtered := artifacts[:0]
		for _, a := range artifacts {
			if matchesPhase(a.Path, ph) {
				filtered = append(filtered, a)
			}
		}
		artifacts = filtered
	}

	// Apply prefix filter (multi-value OR over id-prefix segments,
	// e.g. ADR, SD, FEAT, US, RSCH).
	if len(prefix) > 0 {
		set := make(map[string]struct{}, len(prefix))
		for _, p := range prefix {
			if p != "" {
				set[strings.ToUpper(p)] = struct{}{}
			}
		}
		if len(set) > 0 {
			filtered := artifacts[:0]
			for _, a := range artifacts {
				if _, ok := set[idPrefixSegment(a.ID)]; ok {
					filtered = append(filtered, a)
				}
			}
			artifacts = filtered
		}
	}

	// Apply search filter across title, path, description, frontmatter, and
	// body text per TD-029 precedence. Matches are recorded so each edge can
	// surface a context-window snippet on ArtifactEdge.snippet.
	matchSnippets := map[string]string{}
	if search != nil && *search != "" {
		q := strings.ToLower(*search)
		filtered := artifacts[:0]
		for _, a := range artifacts {
			snippet, ok := matchArtifactForSearch(a, root, q)
			if ok {
				filtered = append(filtered, a)
				matchSnippets[a.ID] = snippet
			}
		}
		artifacts = filtered
	}

	// Sort with (sortKey, id) tie-breaker for stable cursor pagination.
	sortArtifacts(artifacts, sortKey)

	// Build Relay edges.
	all := make([]*ArtifactEdge, len(artifacts))
	for i, a := range artifacts {
		cp := a
		edge := &ArtifactEdge{
			Node:   cp,
			Cursor: encodeStableCursor(cp.ID),
		}
		if sn, ok := matchSnippets[cp.ID]; ok && sn != "" {
			s := sn
			edge.Snippet = &s
		}
		all[i] = edge
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

// sortArtifacts orders artifacts by the requested sort key with a stable
// (sortKey, id) tie-breaker so cursor pagination is deterministic across
// pages even when the primary key has duplicate values.
func sortArtifacts(artifacts []*Artifact, key *ArtifactSort) {
	k := ArtifactSortPath
	if key != nil {
		k = *key
	}
	sort.SliceStable(artifacts, func(i, j int) bool {
		a, b := artifacts[i], artifacts[j]
		var less, equal bool
		switch k {
		case ArtifactSortID:
			less, equal = a.ID < b.ID, a.ID == b.ID
		case ArtifactSortTitle:
			less, equal = a.Title < b.Title, a.Title == b.Title
		case ArtifactSortModified:
			ai, bi := "", ""
			if a.UpdatedAt != nil {
				ai = *a.UpdatedAt
			}
			if b.UpdatedAt != nil {
				bi = *b.UpdatedAt
			}
			less, equal = ai < bi, ai == bi
		case ArtifactSortDepsCount:
			ac, bc := dependsOnCount(a), dependsOnCount(b)
			less, equal = ac < bc, ac == bc
		default: // PATH
			less, equal = a.Path < b.Path, a.Path == b.Path
		}
		if equal {
			return a.ID < b.ID
		}
		return less
	})
}

// matchesPhase reports whether a path falls under docs/helix/<phase>/.
// It accepts either an exact directory name match (e.g. "01-frame") or
// a numeric prefix match (e.g. "01") matching docs/helix/01-*/.
func matchesPhase(path, phase string) bool {
	const root = "docs/helix/"
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, root) {
		return false
	}
	rest := p[len(root):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return false
	}
	dir := rest[:slash]
	if dir == phase {
		return true
	}
	// numeric prefix match: phase="01" matches dir="01-frame"
	if dash := strings.IndexByte(dir, '-'); dash > 0 && dir[:dash] == phase {
		return true
	}
	return false
}

// idPrefixSegment extracts the leading prefix segment from an artifact ID.
// IDs are typically formatted as "doc:<PREFIX>-..." (e.g. "doc:ADR-001-foo")
// or "<PREFIX>-..." for sidecar artifacts. The "doc:" / "sidecar:" namespace
// prefix, if present, is stripped before extraction. Returns empty string
// when no prefix segment can be identified.
func idPrefixSegment(id string) string {
	s := id
	for _, ns := range []string{"doc:", "sidecar:"} {
		if strings.HasPrefix(s, ns) {
			s = s[len(ns):]
			break
		}
	}
	if i := strings.IndexByte(s, '-'); i > 0 {
		return strings.ToUpper(s[:i])
	}
	return ""
}

// matchArtifactForSearch implements the TD-029 search precedence:
//
//  1. title
//  2. path
//  3. description
//  4. ddxFrontmatter (raw JSON)
//  5. body text (only when the file is text and within the size cap)
//
// The first matching field wins; later fields are not consulted. Returns
// the snippet (with the match marked) and whether the artifact matched.
// qLower must already be lowercased by the caller.
func matchArtifactForSearch(a *Artifact, root, qLower string) (string, bool) {
	if qLower == "" {
		return "", false
	}
	if hit, ok := substringSnippet(a.Title, qLower); ok {
		return hit, true
	}
	if hit, ok := substringSnippet(a.Path, qLower); ok {
		return hit, true
	}
	if a.Description != nil {
		if hit, ok := substringSnippet(*a.Description, qLower); ok {
			return hit, true
		}
	}
	if a.DdxFrontmatter != nil {
		if hit, ok := substringSnippet(*a.DdxFrontmatter, qLower); ok {
			return hit, true
		}
	}
	// Body search — bounded by size cap and binary skip.
	if shouldSearchBody(a.Path) {
		body, ok := readBodyForSearch(filepath.Join(root, filepath.FromSlash(a.Path)))
		if ok {
			if hit, ok := substringSnippet(body, qLower); ok {
				return hit, true
			}
		}
	}
	return "", false
}

// substringSnippet finds the (case-insensitive) first occurrence of qLower
// in text and returns a windowed snippet around it with the match wrapped
// in "**…**" markers. Reports false when no match is found.
func substringSnippet(text, qLower string) (string, bool) {
	if text == "" || qLower == "" {
		return "", false
	}
	lower := strings.ToLower(text)
	idx := strings.Index(lower, qLower)
	if idx < 0 {
		return "", false
	}
	matchLen := len(qLower)
	// Compute window in rune-aware-ish bounds; falling back to bytes is
	// safe for the predominantly-ASCII content we search, and the marker
	// preserves the original casing of the matched substring.
	start := idx - snippetWindowChars
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + snippetWindowChars
	if end > len(text) {
		end = len(text)
	}
	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	b.WriteString(text[start:idx])
	b.WriteString("**")
	b.WriteString(text[idx : idx+matchLen])
	b.WriteString("**")
	b.WriteString(text[idx+matchLen : end])
	if end < len(text) {
		b.WriteString("…")
	}
	return b.String(), true
}

// shouldSearchBody reports whether a file at the given relative path is
// eligible for body content search. Binary file extensions are skipped per
// the TD-029 allowlist. SVG and Excalidraw JSON are explicitly text.
func shouldSearchBody(relPath string) bool {
	lower := strings.ToLower(relPath)
	// Excalidraw JSON variants are text.
	if strings.HasSuffix(lower, ".excalidraw") || strings.HasSuffix(lower, ".excalidraw.json") {
		return true
	}
	// .excalidraw.png is binary (covered by .png extension below).
	ext := filepath.Ext(lower)
	if ext == "" {
		return true
	}
	if _, isBinary := binarySearchExtensions[ext]; isBinary {
		return false
	}
	return true
}

// readBodyForSearch reads up to searchBodySizeCapBytes from absPath. Files
// whose first 512 bytes contain a NUL byte are classified as binary and
// skipped. Returns the trimmed content and whether it should be searched.
func readBodyForSearch(absPath string) (string, bool) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, searchBodySizeCapBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", false
	}
	if n == 0 {
		return "", false
	}
	data := buf[:n]
	// Binary content sniff: NUL byte in the first 512 bytes signals binary.
	sniffEnd := 512
	if len(data) < sniffEnd {
		sniffEnd = len(data)
	}
	for i := 0; i < sniffEnd; i++ {
		if data[i] == 0 {
			return "", false
		}
	}
	return string(data), true
}

// dependsOnCount returns the number of depends_on entries declared in the
// artifact's frontmatter, or 0 if none / unparseable.
func dependsOnCount(a *Artifact) int {
	if a.DdxFrontmatter == nil {
		return 0
	}
	var fm map[string]interface{}
	if err := json.Unmarshal([]byte(*a.DdxFrontmatter), &fm); err != nil {
		return 0
	}
	deps, ok := fm["depends_on"].([]interface{})
	if !ok {
		return 0
	}
	return len(deps)
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
	root := r.projectRoot(ctx, projectID)
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
