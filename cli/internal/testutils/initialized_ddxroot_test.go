package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func TestMakeInitializedDDxRoot_WritesConfigYaml(t *testing.T) {
	dir := t.TempDir()

	root := MakeInitializedDDxRoot(t, dir)

	if root != filepath.Join(dir, ddxroot.DirName) {
		t.Fatalf("root = %q, want %q", root, filepath.Join(dir, ddxroot.DirName))
	}
	got, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if string(got) != "version: \"1.0\"\n" {
		t.Fatalf("config.yaml = %q", got)
	}
}
