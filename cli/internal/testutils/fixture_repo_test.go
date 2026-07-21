package testutils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// TestFixtureBinaryScratchEscapesTestScopedRoots pins the invariant that
// replaced the old "uses configured execution root" contract. Binaries built
// behind a sync.Once outlive the test that happens to build them first, so the
// scratch dir must not sit under a caller's DDX_EXEC_WT_DIR — that env var is
// routinely pointed at a t.TempDir() which Go removes at that test's cleanup.
func TestFixtureBinaryScratchEscapesTestScopedRoots(t *testing.T) {
	testScopedRoot := filepath.Join(t.TempDir(), "scratch", "ddx-exec-wt")
	t.Setenv(config.ExecutionWorktreeRootEnv, testScopedRoot)

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
			if strings.HasPrefix(dir, filepath.Dir(testScopedRoot)) {
				t.Fatalf("scratch dir %q must not live under test-scoped root %q", dir, filepath.Dir(testScopedRoot))
			}
			if !strings.HasPrefix(filepath.Base(dir), strings.TrimSuffix(pattern, "*")) {
				t.Fatalf("scratch base = %q, want pattern %q", filepath.Base(dir), pattern)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
		})
	}
}
