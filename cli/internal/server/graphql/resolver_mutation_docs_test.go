package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func newDocumentWriteTestResolver(t *testing.T) (string, *mutationResolver) {
	t.Helper()

	root := t.TempDir()
	ddxDir := filepath.Join(root, ddxroot.DirName)
	if err := os.MkdirAll(filepath.Join(ddxDir, "plugins", "ddx"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: "1.0"
library:
  path: ".ddx/plugins/ddx"
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	return root, &mutationResolver{&Resolver{WorkingDir: root}}
}

func writeDocumentFixture(t *testing.T, root, rel, body string) string {
	t.Helper()

	full := filepath.Join(root, ddxroot.DirName, "plugins", "ddx", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func fileContentHash(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestDocumentWrite_RejectsStaleExpectedHash(t *testing.T) {
	root, resolver := newDocumentWriteTestResolver(t)
	fullPath := writeDocumentFixture(t, root, "docs/alpha.md", "# Alpha\n\nOriginal content.\n")
	expectedHash := fileContentHash(t, fullPath)

	updatedBody := "# Alpha\n\nFresh content.\n"
	if err := os.WriteFile(fullPath, []byte(updatedBody), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolver.DocumentWrite(
		context.Background(),
		"docs/alpha.md",
		"# Alpha\n\nStale content.\n",
		&expectedHash,
	)
	if err == nil {
		t.Fatalf("expected stale-write error, got document: %+v", got)
	}
	gqlErr, ok := err.(*gqlerror.Error)
	if !ok {
		t.Fatalf("expected gqlerror.Error, got %T: %v", err, err)
	}
	if gqlErr.Extensions["code"] != "DOCUMENT_WRITE_CONFLICT" {
		t.Fatalf("expected conflict code, got %+v", gqlErr.Extensions)
	}
	if got := strings.TrimSpace(gqlErr.Message); !strings.Contains(got, "conflict") {
		t.Fatalf("expected conflict message, got %q", gqlErr.Message)
	}

	body, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != updatedBody {
		t.Fatalf("stale write overwrote current content:\nwant %q\n got %q", updatedBody, string(body))
	}
}

func TestDocumentWrite_PathConfinementStillRejectsEscape(t *testing.T) {
	_, resolver := newDocumentWriteTestResolver(t)

	tests := []struct {
		name string
		path string
	}{
		{name: "absolute", path: "/etc/passwd"},
		{name: "traversal", path: "../etc/passwd"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolver.DocumentWrite(context.Background(), tc.path, "pwned", nil)
			if err == nil {
				t.Fatalf("expected rejection, got document: %+v", got)
			}
			if err.Error() != "invalid path" {
				t.Fatalf("expected invalid path error, got %v", err)
			}
		})
	}
}
