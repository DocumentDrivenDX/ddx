package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// Documents is the resolver for the documents field with Relay cursor pagination.
func (r *queryResolver) Documents(ctx context.Context, first *int, after *string, last *int, before *string, typeArg *string) (*DocumentConnection, error) {
	if r.WorkingDir == "" {
		return &DocumentConnection{
			Edges:      []*DocumentEdge{},
			PageInfo:   &PageInfo{},
			TotalCount: 0,
		}, nil
	}

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
			Cursor: encodeCursor(i),
		}
	}

	startIdx := 0
	if after != nil {
		if idx, ok := decodeCursor(*after); ok {
			startIdx = idx + 1
		}
	}
	endIdx := len(all)
	if before != nil {
		if idx, ok := decodeCursor(*before); ok && idx < endIdx {
			endIdx = idx
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
	if r.WorkingDir == "" {
		empty, _ := json.Marshal(map[string]string{})
		emptyS := string(empty)
		return &DocGraph{
			RootDir:    "",
			Documents:  []*Document{},
			PathToID:   emptyS,
			Dependents: emptyS,
			Warnings:   []string{},
		}, nil
	}

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
