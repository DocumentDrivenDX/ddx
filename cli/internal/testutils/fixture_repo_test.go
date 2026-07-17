package testutils

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

func TestFixtureBinaryScratchUsesConfiguredExecutionRoot(t *testing.T) {
	configuredRoot := filepath.Join(t.TempDir(), "scratch", "ddx-exec-wt")
	t.Setenv(config.ExecutionWorktreeRootEnv, configuredRoot)
	wantScratchRoot := filepath.Dir(configuredRoot)
	resolvedScratchRoot := config.ExecutionScratchRoot("")
	if resolvedScratchRoot != wantScratchRoot {
		t.Fatalf("ExecutionScratchRoot = %q, want %q from DDX_EXEC_WT_DIR", resolvedScratchRoot, wantScratchRoot)
	}

	patterns := []string{
		"ddx-fixture-bin-*",
		"ddx-fizeau-testseam-bin-*",
	}
	for _, pattern := range patterns {
		t.Run(strings.TrimSuffix(pattern, "-*"), func(t *testing.T) {
			dir, err := fixtureBinaryScratchDir(pattern)
			if err != nil {
				t.Fatalf("fixtureBinaryScratchDir(%q): %v", pattern, err)
			}
			if filepath.Dir(dir) != resolvedScratchRoot {
				t.Fatalf("scratch dir = %q, want direct child of configured scratch root %q", dir, resolvedScratchRoot)
			}
			if !strings.HasPrefix(filepath.Base(dir), strings.TrimSuffix(pattern, "*")) {
				t.Fatalf("scratch base = %q, want pattern %q", filepath.Base(dir), pattern)
			}
		})
	}
}
