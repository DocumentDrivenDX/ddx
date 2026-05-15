package ddxroot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDDxRoot_InTreeMode(t *testing.T) {
	projectRoot := t.TempDir()
	inTree := filepath.Join(projectRoot, ".ddx")
	if err := os.MkdirAll(inTree, 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}

	got := Path(context.Background(), projectRoot)
	if got != inTree {
		t.Fatalf("Path() = %q, want %q", got, inTree)
	}
}
