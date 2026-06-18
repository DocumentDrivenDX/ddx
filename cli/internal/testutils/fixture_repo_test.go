package testutils

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func TestNewFixtureRepoCachesPerProfileAndCopiesIsolation(t *testing.T) {
	cacheRoot := t.TempDir()

	prevCacheRoot := fixtureRepoCacheRootOverride
	prevBuilder := buildFixtureRepoTemplateFn
	fixtureRepoCacheRootOverride = cacheRoot
	t.Cleanup(func() {
		fixtureRepoCacheRootOverride = prevCacheRoot
		buildFixtureRepoTemplateFn = prevBuilder
	})

	var mu sync.Mutex
	buildCounts := map[string]int{}
	buildFixtureRepoTemplateFn = func(t *testing.T, dest, profile string) error {
		mu.Lock()
		buildCounts[profile]++
		mu.Unlock()

		if err := os.MkdirAll(filepath.Join(dest, ddxroot.DirName), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(ddxroot.InTree(dest, "config.yaml"), []byte("version: \"1.0\"\n"), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(ddxroot.InTree(dest, "beads.jsonl"), []byte(profile+"\n"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, "profile.txt"), []byte(profile), 0o644)
	}

	minimal1 := NewFixtureRepo(t, "minimal")
	minimal2 := NewFixtureRepo(t, "minimal")
	standard1 := NewFixtureRepo(t, "standard")
	standard2 := NewFixtureRepo(t, "standard")

	mustWriteFile(t, filepath.Join(minimal1, "marker.txt"), "one")
	if _, err := os.Stat(filepath.Join(minimal2, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("second minimal fixture saw first fixture marker: %v", err)
	}

	mustWriteFile(t, filepath.Join(standard1, "marker.txt"), "one")
	if _, err := os.Stat(filepath.Join(standard2, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("second standard fixture saw first fixture marker: %v", err)
	}

	assertFileContents(t, filepath.Join(minimal2, "profile.txt"), "minimal")
	assertFileContents(t, filepath.Join(standard2, "profile.txt"), "standard")

	mu.Lock()
	defer mu.Unlock()
	if got := buildCounts["minimal"]; got != 1 {
		t.Fatalf("minimal build count = %d, want 1", got)
	}
	if got := buildCounts["standard"]; got != 1 {
		t.Fatalf("standard build count = %d, want 1", got)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
