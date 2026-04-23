package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// DocumentByPath is the resolver for the documentByPath field.
// It reads the document at the given path (relative to the docgraph root or
// library) and returns its metadata plus raw file content.
func (r *queryResolver) DocumentByPath(ctx context.Context, path string) (*Document, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return nil, fmt.Errorf("invalid path")
	}
	// Legacy clients may emit absolute filesystem paths (bead ddx-12cae4dd)
	// produced before doc.Path was normalised. Reject them — any real document
	// resolved through the graph uses a relative path.
	if filepath.IsAbs(cleaned) {
		return nil, nil
	}

	// Prefer the docgraph-tracked location: the documents list surfaces files
	// walked under workingDir, so the detail view must read from the same root
	// (otherwise the URL for a valid list entry 404s).
	if graph, graphErr := docgraph.BuildGraphWithConfig(r.WorkingDir); graphErr == nil {
		if id, ok := graph.PathToID[cleaned]; ok {
			if d, ok := graph.Documents[id]; ok {
				absPath := d.Path
				if !filepath.IsAbs(absPath) {
					absPath = filepath.Join(graph.RootDir, absPath)
				}
				if data, readErr := os.ReadFile(absPath); readErr == nil {
					content := string(data)
					doc := docToGQL(*d)
					doc.Content = &content
					return doc, nil
				}
			}
		}
	}

	// Fall back to the configured library path so documents created via the
	// documentWrite mutation (which target the library) remain readable even
	// when they have no DDx frontmatter and so do not appear in the graph.
	cfg, err := config.LoadWithWorkingDir(r.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if cfg.Library == nil || cfg.Library.Path == "" {
		return nil, nil
	}
	libPath := cfg.Library.Path
	if !filepath.IsAbs(libPath) {
		libPath = filepath.Join(r.WorkingDir, libPath)
	}

	fullPath := filepath.Join(libPath, cleaned)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, nil // not found → null
	}
	content := string(data)

	return &Document{
		ID:         cleaned,
		Path:       cleaned,
		DependsOn:  []string{},
		Inputs:     []string{},
		Dependents: []string{},
		Content:    &content,
	}, nil
}

// Documents is the resolver for the documents field with Relay cursor pagination.
func (r *queryResolver) Documents(ctx context.Context, first *int, after *string, last *int, before *string, typeArg *string) (*DocumentConnection, error) {
	graph, err := docgraph.BuildGraphWithConfig(r.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("building document graph: %w", err)
	}

	docs := graph.AllNodesForOutput()
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })

	// Apply optional type filter by path component.
	if typeArg != nil && *typeArg != "" {
		filtered := docs[:0]
		for _, d := range docs {
			if strings.Contains(d.Path, string([]rune{'/'})+*typeArg+string([]rune{'/'})) ||
				strings.HasPrefix(d.Path, *typeArg+string([]rune{'/'})) {
				filtered = append(filtered, d)
			}
		}
		docs = filtered
	}

	all := make([]*DocumentEdge, len(docs))
	for i, d := range docs {
		all[i] = &DocumentEdge{
			Node:   docToGQL(d),
			Cursor: encodeStableCursor(d.ID),
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

	slice := all[startIdx:endIdx]
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
	}

	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0,
		HasNextPage:     endIdx < len(all),
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &DocumentConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}, nil
}

// DocGraph is the resolver for the docGraph field.
func (r *queryResolver) DocGraph(ctx context.Context) (*DocGraph, error) {
	graph, err := docgraph.BuildGraphWithConfig(r.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("building document graph: %w", err)
	}

	docs := graph.AllNodesForOutput()
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	gqlDocs := make([]*Document, len(docs))
	for i, d := range docs {
		gqlDocs[i] = docToGQL(d)
	}

	pathToIDJSON, err := json.Marshal(graph.PathToID)
	if err != nil {
		return nil, fmt.Errorf("serializing pathToId: %w", err)
	}
	dependentsJSON, err := json.Marshal(graph.Dependents)
	if err != nil {
		return nil, fmt.Errorf("serializing dependents: %w", err)
	}

	warnings := graph.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return &DocGraph{
		RootDir:    graph.RootDir,
		Documents:  gqlDocs,
		PathToID:   string(pathToIDJSON),
		Dependents: string(dependentsJSON),
		Warnings:   warnings,
	}, nil
}

// docToGQL converts a docgraph.Document to the GraphQL Document model.
func docToGQL(d docgraph.Document) *Document {
	doc := &Document{
		ID:         d.ID,
		Path:       d.Path,
		Title:      d.Title,
		DependsOn:  d.DependsOn,
		Inputs:     d.Inputs,
		Dependents: d.Dependents,
		ParkingLot: d.ParkingLot,
	}
	if doc.DependsOn == nil {
		doc.DependsOn = []string{}
	}
	if doc.Inputs == nil {
		doc.Inputs = []string{}
	}
	if doc.Dependents == nil {
		doc.Dependents = []string{}
	}
	if d.Prompt != "" {
		p := d.Prompt
		doc.Prompt = &p
	}
	if d.Review.ReviewedAt != "" || d.Review.SelfHash != "" {
		depsJSON, _ := json.Marshal(d.Review.Deps)
		doc.Review = &DocumentReview{
			SelfHash:   d.Review.SelfHash,
			Deps:       string(depsJSON),
			ReviewedAt: d.Review.ReviewedAt,
		}
	}
	if d.ExecDef != nil {
		active := d.ExecDef.Active
		required := d.ExecDef.Required
		graphSource := true
		doc.ExecDef = &DocumentExecDef{
			ArtifactIds: d.ExecDef.ArtifactIDs,
			Active:      &active,
			Required:    &required,
			GraphSource: &graphSource,
		}
	}
	return doc
}
