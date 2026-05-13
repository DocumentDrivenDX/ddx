package graphql

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDocStale_ReturnsDocumentGraphStaleness(t *testing.T) {
	root := t.TempDir()

	mustWrite := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("docs/prd.md", "---\nddx:\n  id: helix.prd\n---\n# PRD\n")
	mustWrite("docs/arch.md", "---\nddx:\n  id: helix.arch\n  depends_on:\n    - helix.prd\n  review:\n    self_hash: stale\n    deps:\n      helix.prd: wrong_hash\n---\n# Arch\n")

	resolver := &queryResolver{&Resolver{WorkingDir: root}}
	stale, err := resolver.DocStale(context.Background())
	if err != nil {
		t.Fatalf("DocStale returned error: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected one stale document, got %d: %+v", len(stale), stale)
	}
	if stale[0].ID != "helix.arch" {
		t.Fatalf("expected helix.arch to be stale, got %+v", stale[0])
	}
	if stale[0].Path != "docs/arch.md" {
		t.Fatalf("expected docs/arch.md path, got %+v", stale[0])
	}
	if len(stale[0].Reasons) == 0 {
		t.Fatalf("expected stale reasons, got %+v", stale[0])
	}
}
