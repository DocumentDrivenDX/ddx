package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// MakeInitializedDDxRoot creates an in-tree .ddx root with the minimal
// config.yaml required for ddxroot.Path to treat dir as an initialized project.
func MakeInitializedDDxRoot(t testing.TB, dir string) string {
	t.Helper()
	root := filepath.Join(dir, ddxroot.DirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir initialized ddx root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("version: \"1.0\"\n"), 0o644); err != nil {
		t.Fatalf("write initialized ddx config: %v", err)
	}
	return root
}
