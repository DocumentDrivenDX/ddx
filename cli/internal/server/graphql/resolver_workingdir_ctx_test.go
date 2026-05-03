package graphql

// LAYER 2 of the GraphQL multi-project leak fix (ddx-055e8d32).
// TestResolver_UsesContextWorkingDir asserts the resolver reads its
// working directory from the request context (via WithWorkingDir) rather
// than the resolver struct's startup default. This is the unit-level
// guarantee that lets the singleton resolver serve any registered project
// without per-request reconstruction (LAYER 1).

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// seedDocsRoot creates a minimal DDx-tracked project at root with one
// markdown document carrying the given DDx id. Returns the document's
// repo-relative path.
func seedDocsRoot(t *testing.T, root, docID, title string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".ddx"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
  repository:
    url: "https://example.com/lib"
    branch: "main"
`
	if err := os.WriteFile(filepath.Join(root, ".ddx", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("docs", docID+".md")
	body := "---\nddx:\n  id: " + docID + "\n---\n# " + title + "\n\nbody for " + docID + "\n"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return rel
}

// TestResolver_UsesContextWorkingDir is the LAYER 2 contract test:
// when the request ctx carries a WorkingDir via WithWorkingDir, every
// resolver method reads from THAT directory rather than the resolver's
// fallback r.WorkingDir. We exercise DocumentByPath against project B's
// path while the resolver is constructed with project A as the default
// — the result must come from B.
func TestResolver_UsesContextWorkingDir(t *testing.T) {
	root := t.TempDir()
	projA := filepath.Join(root, "proj-a")
	projB := filepath.Join(root, "proj-b")
	relA := seedDocsRoot(t, projA, "alpha", "Alpha A")
	relB := seedDocsRoot(t, projB, "beta", "Beta B")

	// Resolver constructed with A as the FALLBACK working dir.
	r := &Resolver{WorkingDir: projA}
	q := &queryResolver{r}

	// 1. With ctx carrying B's WorkingDir, asking for B's doc returns B's content.
	ctxB := WithWorkingDir(context.Background(), projB)
	docB, err := q.DocumentByPath(ctxB, filepath.ToSlash(relB))
	if err != nil {
		t.Fatalf("DocumentByPath(B) error: %v", err)
	}
	if docB == nil || docB.Content == nil {
		t.Fatalf("DocumentByPath(B) returned nil/no content: %+v", docB)
	}
	if got := *docB.Content; !contains(got, "Beta B") {
		t.Fatalf("expected B's content, got %q", got)
	}

	// 2. With ctx carrying B's WorkingDir, asking for A's doc must NOT find it
	//    (A's relA happens to differ; importantly, the resolver must NOT silently
	//    fall back to projA via the struct field).
	docCross, err := q.DocumentByPath(ctxB, filepath.ToSlash(relA))
	if err != nil {
		t.Fatalf("DocumentByPath(A under B-ctx) error: %v", err)
	}
	if docCross != nil && docCross.Content != nil && contains(*docCross.Content, "Alpha A") {
		t.Fatalf("CROSS-PROJECT LEAK: ctx WorkingDir=B returned A's content: %+v", docCross)
	}

	// 3. Without WithWorkingDir, the resolver falls back to r.WorkingDir (projA).
	docFallback, err := q.DocumentByPath(context.Background(), filepath.ToSlash(relA))
	if err != nil {
		t.Fatalf("DocumentByPath(A fallback) error: %v", err)
	}
	if docFallback == nil || docFallback.Content == nil {
		t.Fatalf("fallback DocumentByPath(A) returned nil: %+v", docFallback)
	}
	if got := *docFallback.Content; !contains(got, "Alpha A") {
		t.Fatalf("expected A's content via fallback, got %q", got)
	}

	// 4. WorkingDirFromContext reflects what was injected.
	if got := WorkingDirFromContext(ctxB); got != projB {
		t.Fatalf("WorkingDirFromContext: got %q want %q", got, projB)
	}
	if got := WorkingDirFromContext(context.Background()); got != "" {
		t.Fatalf("WorkingDirFromContext on bare ctx: got %q want empty", got)
	}

	// 5. r.workingDir(ctx) prefers ctx, then falls back to r.WorkingDir.
	if got := r.workingDir(ctxB); got != projB {
		t.Fatalf("r.workingDir(ctxB): got %q want %q", got, projB)
	}
	if got := r.workingDir(context.Background()); got != projA {
		t.Fatalf("r.workingDir(bare): got %q want %q", got, projA)
	}
}

// contains is a tiny stdlib-free substring check to avoid importing
// strings just for one assertion.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
