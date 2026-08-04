package testutils

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/scratchowner"
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

func TestFixtureBinaryScratchWritesLiveOwnerMarker(t *testing.T) {
	originalScratchDirFn := fixtureBinaryScratchDirFn
	originalBuilder := fixtureBinaryBuilder
	originalMarkerWriter := fixtureBinaryMarkerWrite
	t.Cleanup(func() {
		fixtureBinaryScratchDirFn = originalScratchDirFn
		fixtureBinaryBuilder = originalBuilder
		fixtureBinaryMarkerWrite = originalMarkerWriter
		resetFixtureBinaryBuildState()
	})

	runSuccessCase := func(t *testing.T, build func() (string, error), wantKind string) {
		t.Helper()
		scratchRoot := t.TempDir()
		scratchDir := filepath.Join(scratchRoot, wantKind+"-scratch")

		resetFixtureBinaryBuildState()
		fixtureBinaryScratchDirFn = func(pattern string) (string, error) {
			return scratchDir, nil
		}
		fixtureBinaryMarkerWrite = scratchowner.WriteForCurrentProcess
		fixtureBinaryBuilder = func(dir, out string, args []string) error {
			t.Helper()
			if dir != scratchDir {
				t.Fatalf("build dir: got %q want %q", dir, scratchDir)
			}
			status, marker, err := scratchowner.Evaluate(dir)
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", dir, err)
			}
			if status != scratchowner.StatusLive {
				t.Fatalf("marker status before build: got %q want %q", status, scratchowner.StatusLive)
			}
			if marker.Kind != wantKind {
				t.Fatalf("marker kind before build: got %q want %q", marker.Kind, wantKind)
			}
			if marker.OwnerPID != os.Getpid() {
				t.Fatalf("marker owner_pid before build: got %d want %d", marker.OwnerPID, os.Getpid())
			}
			if _, err := os.Stat(scratchowner.Path(dir)); err != nil {
				t.Fatalf("marker file missing before build: %v", err)
			}
			if len(args) == 0 {
				t.Fatal("build args must not be empty")
			}
			return os.WriteFile(out, []byte("#!/bin/sh\nexit 0\n"), 0o755)
		}

		path, err := build()
		if err != nil {
			t.Fatalf("build helper failed: %v", err)
		}
		if path == "" {
			t.Fatal("build helper returned empty path")
		}
		if filepath.Dir(path) != scratchDir {
			t.Fatalf("returned path dir: got %q want %q", filepath.Dir(path), scratchDir)
		}
		status, marker, err := scratchowner.Evaluate(scratchDir)
		if err != nil {
			t.Fatalf("Evaluate(%q) after build: %v", scratchDir, err)
		}
		if status != scratchowner.StatusLive {
			t.Fatalf("marker status after build: got %q want %q", status, scratchowner.StatusLive)
		}
		if marker.Kind != wantKind {
			t.Fatalf("marker kind after build: got %q want %q", marker.Kind, wantKind)
		}
	}

	t.Run("ddx", func(t *testing.T) {
		runSuccessCase(t, buildDDxBinary, scratchowner.KindFixtureBinary)
	})

	t.Run("fizeau", func(t *testing.T) {
		runSuccessCase(t, buildDDxFizeauTestSeamBinary, scratchowner.KindFizeauTestSeamBinary)
	})

	t.Run("marker failure removes scratch dir", func(t *testing.T) {
		scratchDir := filepath.Join(t.TempDir(), "ddx-fixture-bin-failure")

		resetFixtureBinaryBuildState()
		fixtureBinaryScratchDirFn = func(pattern string) (string, error) {
			return scratchDir, nil
		}
		fixtureBinaryMarkerWrite = func(dir, kind string) (scratchowner.Marker, error) {
			return scratchowner.Marker{}, errors.New("marker write failed")
		}
		fixtureBinaryBuilder = func(string, string, []string) error {
			t.Fatal("build should not run when marker creation fails")
			return nil
		}

		if _, err := buildDDxBinary(); err == nil {
			t.Fatal("buildDDxBinary should fail when marker creation fails")
		}
		if _, err := os.Stat(scratchDir); !os.IsNotExist(err) {
			t.Fatalf("scratch dir should be removed after marker failure, got err=%v", err)
		}
	})
}

func resetFixtureBinaryBuildState() {
	builtBinaryOnce = sync.Once{}
	builtBinaryPath = ""
	builtBinaryErr = nil
	builtFizeauTestSeamBinaryOnce = sync.Once{}
	builtFizeauTestSeamBinaryPath = ""
	builtFizeauTestSeamBinaryErr = nil
}
