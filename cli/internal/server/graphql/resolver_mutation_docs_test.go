package graphql

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
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
