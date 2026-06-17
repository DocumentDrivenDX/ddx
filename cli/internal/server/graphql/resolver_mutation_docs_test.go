package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/require"
)

const (
	testFeaturePath = "docs/helix/01-frame/features/FEAT-026-federation.md"
	testPrdPath     = "docs/helix/01-frame/features/FEAT-000-prd.md"
	testPrdID       = "helix.prd"
	testFeatureID   = "helix.feat026"
)

func setupDocumentProject(t *testing.T, featureBody string) string {
	t.Helper()

	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)

	cfg := `version: "1.0"
library:
  path: "."
`
	if err := os.WriteFile(filepath.Join(root, ddxroot.DirName, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	mustWriteDoc(t, root, testPrdPath, `---
ddx:
  id: helix.prd
---
# PRD
`)
	mustWriteDoc(t, root, testFeaturePath, featureBody)
	stampDocsFresh(t, root, testPrdID, testFeatureID)
	return root
}

func mustWriteDoc(t *testing.T, root, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func stampDocsFresh(t *testing.T, root string, ids ...string) {
	t.Helper()

	graph, err := docgraph.BuildGraphWithConfig(root)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	stamped, warnings, err := graph.Stamp(ids, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("stamp docs: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected stamp warnings: %v", warnings)
	}
	if len(stamped) != len(ids) {
		t.Fatalf("stamped %d docs, want %d (%v)", len(stamped), len(ids), stamped)
	}
}

func staleContains(stale []*StaleReason, id string) bool {
	for _, reason := range stale {
		if reason != nil && reason.ID == id {
			return true
		}
	}
	return false
}

func TestDocumentWrite_RebuildsGraphAfterSave(t *testing.T) {
	root := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`)

	mut := &mutationResolver{&Resolver{WorkingDir: root}}
	qry := &queryResolver{&Resolver{WorkingDir: root}}
	ctx := context.Background()

	updated := `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
    - helix.extra
---
# Federation

Updated content.
`
	doc, err := mut.DocumentWrite(ctx, testFeaturePath, updated, nil)
	if err != nil {
		t.Fatalf("DocumentWrite: %v", err)
	}
	if doc == nil || doc.Content == nil {
		t.Fatalf("DocumentWrite returned nil content: %+v", doc)
	}
	if got := *doc.Content; got != updated {
		t.Fatalf("DocumentWrite content = %q, want %q", got, updated)
	}
	if len(doc.DependsOn) != 2 || doc.DependsOn[0] != "helix.extra" || doc.DependsOn[1] != "helix.prd" {
		t.Fatalf("DocumentWrite dependsOn = %+v, want [helix.extra helix.prd]", doc.DependsOn)
	}

	readBack, err := qry.DocumentByPath(ctx, testFeaturePath)
	if err != nil {
		t.Fatalf("DocumentByPath: %v", err)
	}
	if readBack == nil || readBack.Content == nil {
		t.Fatalf("DocumentByPath returned nil content: %+v", readBack)
	}
	if got := *readBack.Content; got != updated {
		t.Fatalf("DocumentByPath content = %q, want %q", got, updated)
	}
	if len(readBack.DependsOn) != 2 || readBack.DependsOn[0] != "helix.extra" || readBack.DependsOn[1] != "helix.prd" {
		t.Fatalf("DocumentByPath dependsOn = %+v, want [helix.extra helix.prd]", readBack.DependsOn)
	}

	stale, err := qry.DocStale(ctx)
	if err != nil {
		t.Fatalf("DocStale: %v", err)
	}
	if !staleContains(stale, testFeatureID) {
		t.Fatalf("expected %s to be stale after save, got %+v", testFeatureID, stale)
	}
}

func TestDocumentWrite_RejectsStaleExpectedHash(t *testing.T) {
	root := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`)

	mut := &mutationResolver{&Resolver{WorkingDir: root}}
	ctx := context.Background()

	docPath := filepath.Join(root, testFeaturePath)
	expectedHash, err := docgraph.HashDocumentFile(docPath)
	if err != nil {
		t.Fatalf("HashDocumentFile: %v", err)
	}

	current := `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Fresh content.
`
	if err := os.WriteFile(docPath, []byte(current), 0o644); err != nil {
		t.Fatalf("write current content: %v", err)
	}

	refused := `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Refused content.
`
	if doc, err := mut.DocumentWrite(ctx, testFeaturePath, refused, &expectedHash); err == nil {
		t.Fatalf("DocumentWrite unexpectedly succeeded: %+v", doc)
	} else if !strings.Contains(strings.ToLower(err.Error()), "conflict") {
		t.Fatalf("DocumentWrite error = %v, want conflict", err)
	}

	body, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read current content: %v", err)
	}
	if got := string(body); got != current {
		t.Fatalf("stale write overwrote current content:\nwant %q\n got %q", current, got)
	}
}

func TestDocumentWrite_PathConfinementStillRejectsEscape(t *testing.T) {
	root := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`)

	mut := &mutationResolver{&Resolver{WorkingDir: root}}
	ctx := context.Background()

	cases := []struct {
		name        string
		path        string
		forbiddenAt string
	}{
		{
			name:        "absolute",
			path:        filepath.Join(root, "outside.md"),
			forbiddenAt: filepath.Join(root, "outside.md"),
		},
		{
			name:        "traversal",
			path:        "../escape.md",
			forbiddenAt: filepath.Join(filepath.Dir(root), "escape.md"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if doc, err := mut.DocumentWrite(ctx, tc.path, "pwned", nil); err == nil {
				t.Fatalf("DocumentWrite unexpectedly succeeded: %+v", doc)
			} else if !strings.Contains(strings.ToLower(err.Error()), "invalid path") {
				t.Fatalf("DocumentWrite error = %v, want invalid path", err)
			}

			if _, err := os.Stat(tc.forbiddenAt); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("path confinement allowed write at %s: %v", tc.forbiddenAt, err)
			}
		})
	}
}

func TestDocumentWrite_GraphRefreshUsesAffectedProjectOnly(t *testing.T) {
	projA := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation A

Original A.
`)
	projB := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation B

Original B.
`)

	resolver := &Resolver{WorkingDir: projA}
	mut := &mutationResolver{resolver}
	qry := &queryResolver{resolver}

	ctxA := WithWorkingDir(context.Background(), projA)
	ctxB := WithWorkingDir(context.Background(), projB)

	updatedB := `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation B

Updated B.
`
	doc, err := mut.DocumentWrite(ctxB, testFeaturePath, updatedB, nil)
	if err != nil {
		t.Fatalf("DocumentWrite(projB): %v", err)
	}
	if doc == nil || doc.Content == nil || *doc.Content != updatedB {
		t.Fatalf("DocumentWrite(projB) returned stale content: %+v", doc)
	}

	gotA, err := qry.DocumentByPath(ctxA, testFeaturePath)
	if err != nil {
		t.Fatalf("DocumentByPath(projA): %v", err)
	}
	if gotA == nil || gotA.Content == nil {
		t.Fatalf("DocumentByPath(projA) returned nil content: %+v", gotA)
	}
	if strings.Contains(*gotA.Content, "Updated B.") {
		t.Fatalf("projA leaked projB content: %+v", gotA.Content)
	}

	gotB, err := qry.DocumentByPath(ctxB, testFeaturePath)
	if err != nil {
		t.Fatalf("DocumentByPath(projB): %v", err)
	}
	if gotB == nil || gotB.Content == nil {
		t.Fatalf("DocumentByPath(projB) returned nil content: %+v", gotB)
	}
	if got := *gotB.Content; got != updatedB {
		t.Fatalf("projB content = %q, want %q", got, updatedB)
	}

	staleA, err := qry.DocStale(ctxA)
	if err != nil {
		t.Fatalf("DocStale(projA): %v", err)
	}
	if staleContains(staleA, testFeatureID) {
		t.Fatalf("projA should remain fresh, got %+v", staleA)
	}

	staleB, err := qry.DocStale(ctxB)
	if err != nil {
		t.Fatalf("DocStale(projB): %v", err)
	}
	if !staleContains(staleB, testFeatureID) {
		t.Fatalf("projB should refresh only its own graph, got %+v", staleB)
	}
}

// docWriteMetadataFederation is a test-only FederationProvider that routes
// documentWrite mutations to a spoke resolver, capturing the forwarded request
// so tests can assert on origin/target metadata.
type docWriteMetadataFederation struct {
	spokes        []federation.SpokeRecord
	spokeResolver *mutationResolver
	lastReq       *federation.ForwardMutationRequest
}

func (f *docWriteMetadataFederation) Spokes() []federation.SpokeRecord {
	out := make([]federation.SpokeRecord, len(f.spokes))
	copy(out, f.spokes)
	return out
}

func (f *docWriteMetadataFederation) FanOut(_ context.Context, _ *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return &federation.FanOutResult{}, nil
}

func (f *docWriteMetadataFederation) ForwardMutation(_ context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	f.lastReq = req

	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, f.spokes...)
	owner, err := federation.RouteMutationToProjectOwner(registry, req.TargetProjectID)
	if err != nil || owner == nil || owner.NodeID != req.TargetNodeID {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner.Status == federation.StatusOffline {
		return nil, federation.ErrForwardMutationOffline
	}
	if owner.Status == federation.StatusStale {
		return nil, federation.ErrForwardMutationStale
	}
	if !hasWriteCapability(owner.Capabilities) {
		return nil, federation.ErrForwardMutationReadOnly
	}

	var envelope struct {
		Query     string                     `json:"query"`
		Variables map[string]json.RawMessage `json:"variables"`
	}
	if err := json.Unmarshal(req.Body, &envelope); err != nil {
		return nil, err
	}
	var path, content string
	var expectedHash *string
	if raw, ok := envelope.Variables["path"]; ok {
		_ = json.Unmarshal(raw, &path)
	}
	if raw, ok := envelope.Variables["content"]; ok {
		_ = json.Unmarshal(raw, &content)
	}
	if raw, ok := envelope.Variables["expectedHash"]; ok {
		var h string
		if err := json.Unmarshal(raw, &h); err == nil && h != "" {
			expectedHash = &h
		}
	}

	doc, err := f.spokeResolver.DocumentWrite(context.Background(), path, content, expectedHash)
	if err != nil {
		body, _ := json.Marshal(map[string]any{"errors": []map[string]any{{"message": err.Error()}}})
		return &federation.ForwardMutationResponse{
			OriginIdentity:  req.OriginIdentity,
			ForwardingPath:  append([]string(nil), req.ForwardingPath...),
			RequestID:       req.RequestID,
			IdempotencyKey:  req.IdempotencyKey,
			TargetNodeID:    req.TargetNodeID,
			TargetProjectID: req.TargetProjectID,
			ExpectedVersion: req.ExpectedVersion,
			StatusCode:      http.StatusInternalServerError,
			Body:            body,
		}, nil
	}
	body, err := json.Marshal(map[string]any{"data": map[string]any{"documentWrite": doc}})
	if err != nil {
		return nil, err
	}
	return &federation.ForwardMutationResponse{
		OriginIdentity:  req.OriginIdentity,
		ForwardingPath:  append([]string(nil), req.ForwardingPath...),
		RequestID:       req.RequestID,
		IdempotencyKey:  req.IdempotencyKey,
		TargetNodeID:    req.TargetNodeID,
		TargetProjectID: req.TargetProjectID,
		ExpectedVersion: req.ExpectedVersion,
		StatusCode:      http.StatusOK,
		Headers:         http.Header{"Content-Type": []string{"application/json"}},
		Body:            body,
	}, nil
}

func newDocWriteHubState(hubRoot string) *mutationTestStateProvider {
	now := time.Now().UTC().Format(time.RFC3339)
	return &mutationTestStateProvider{
		node: NodeStateSnapshot{ID: "hub-node", Name: "hub-node"},
		projects: []*Project{{
			ID:           "proj-a",
			Name:         "proj-a",
			Path:         hubRoot,
			RegisteredAt: now,
			LastSeen:     now,
		}},
	}
}

func newDocWriteSpokeState(spokeRoot string) *mutationTestStateProvider {
	now := time.Now().UTC().Format(time.RFC3339)
	return &mutationTestStateProvider{
		node: NodeStateSnapshot{ID: "spoke-node", Name: "spoke-node"},
		projects: []*Project{{
			ID:           "proj-a",
			Name:         "proj-a",
			Path:         spokeRoot,
			RegisteredAt: now,
			LastSeen:     now,
		}},
	}
}

func newDocWriteMetadataFederation(spokeResolver *mutationResolver) *docWriteMetadataFederation {
	return &docWriteMetadataFederation{
		spokes: []federation.SpokeRecord{{
			NodeID:       "spoke-node",
			Name:         "spoke-a",
			ProjectIDs:   []string{"proj-a"},
			Capabilities: []string{"read", "write"},
			Status:       federation.StatusActive,
		}},
		spokeResolver: spokeResolver,
	}
}

func TestFederatedDocumentWrite_ForwardsToOwnerAndReturnsMetadata(t *testing.T) {
	const initialContent = `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`
	hubRoot := t.TempDir()
	spokeRoot := setupDocumentProject(t, initialContent)

	spokeResolver := &mutationResolver{&Resolver{
		State:      newDocWriteSpokeState(spokeRoot),
		WorkingDir: spokeRoot,
		NodeID:     "spoke-node",
	}}
	fed := newDocWriteMetadataFederation(spokeResolver)

	hubResolver := &mutationResolver{&Resolver{
		State:      newDocWriteHubState(hubRoot),
		WorkingDir: hubRoot,
		Federation: fed,
		NodeID:     "hub-node",
	}}

	ctx := WithWorkingDir(context.Background(), hubRoot)
	newContent := "# Updated\n\nNew content from hub.\n"
	doc, err := hubResolver.DocumentWrite(ctx, testFeaturePath, newContent, nil)
	require.NoError(t, err)
	require.NotNil(t, doc)

	// Hub local filesystem must NOT have been written.
	_, statErr := os.Stat(filepath.Join(hubRoot, testFeaturePath))
	require.True(t, os.IsNotExist(statErr), "hub must not write spoke-owned documents locally")

	// Spoke filesystem must have the new content.
	spokeBytes, err := os.ReadFile(filepath.Join(spokeRoot, testFeaturePath))
	require.NoError(t, err)
	require.Equal(t, newContent, string(spokeBytes))
}

func TestFederatedDocumentWrite_PreservesOriginAndTargetMetadata(t *testing.T) {
	const initialContent = `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`
	hubRoot := t.TempDir()
	spokeRoot := setupDocumentProject(t, initialContent)

	spokeResolver := &mutationResolver{&Resolver{
		State:      newDocWriteSpokeState(spokeRoot),
		WorkingDir: spokeRoot,
		NodeID:     "spoke-node",
	}}
	fed := newDocWriteMetadataFederation(spokeResolver)

	hubResolver := &mutationResolver{&Resolver{
		State:      newDocWriteHubState(hubRoot),
		WorkingDir: hubRoot,
		Federation: fed,
		NodeID:     "hub-node",
	}}

	ctx := WithWorkingDir(context.Background(), hubRoot)
	_, err := hubResolver.DocumentWrite(ctx, testFeaturePath, "# Updated\n", nil)
	require.NoError(t, err)

	req := fed.lastReq
	require.NotNil(t, req, "ForwardMutation must have been called")
	require.Equal(t, "hub-node", req.OriginIdentity, "origin identity must be the hub node ID")
	require.Equal(t, "spoke-node", req.TargetNodeID, "target node ID must be the spoke")
	require.Equal(t, "proj-a", req.TargetProjectID, "target project ID must be proj-a")
	require.Contains(t, req.ForwardingPath, "hub-node", "forwarding path must include origin")
	require.Contains(t, req.ForwardingPath, "spoke-node", "forwarding path must include target")
}

func TestDocumentWrite_ConflictDoesNotRefreshGraph(t *testing.T) {
	root := setupDocumentProject(t, `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Original content.
`)

	resolver := &Resolver{WorkingDir: root}
	mut := &mutationResolver{resolver}
	qry := &queryResolver{resolver}
	ctx := context.Background()

	prevWriteFile := documentWriteWriteFile
	documentWriteWriteFile = func(string, []byte, os.FileMode) error {
		return errors.New("stale write conflict")
	}
	t.Cleanup(func() { documentWriteWriteFile = prevWriteFile })

	updated := `---
ddx:
  id: helix.feat026
  depends_on:
    - helix.prd
---
# Federation

Refused content.
`
	if doc, err := mut.DocumentWrite(ctx, testFeaturePath, updated, nil); err == nil {
		t.Fatalf("DocumentWrite unexpectedly succeeded: %+v", doc)
	}

	readBack, err := qry.DocumentByPath(ctx, testFeaturePath)
	if err != nil {
		t.Fatalf("DocumentByPath after refusal: %v", err)
	}
	if readBack == nil || readBack.Content == nil {
		t.Fatalf("DocumentByPath after refusal returned nil content: %+v", readBack)
	}
	if strings.Contains(*readBack.Content, "Refused content.") {
		t.Fatalf("refused write should not change document content: %+v", readBack.Content)
	}

	stale, err := qry.DocStale(ctx)
	if err != nil {
		t.Fatalf("DocStale after refusal: %v", err)
	}
	if staleContains(stale, testFeatureID) {
		t.Fatalf("refused write should not report a successful refresh: %+v", stale)
	}
}
