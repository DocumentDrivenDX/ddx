package spechonesty

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis/analysistest"
)

type fixtureState struct {
	sum     string
	mode    fs.FileMode
	size    int64
	modTime time.Time
}

func snapshotFixtures(t *testing.T, root string) map[string]fixtureState {
	t.Helper()

	snapshot := make(map[string]fixtureState)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		snapshot[rel] = fixtureState{
			sum:     hex.EncodeToString(sum[:]),
			mode:    info.Mode(),
			size:    info.Size(),
			modTime: info.ModTime().UTC().Round(0),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot fixtures under %s: %v", root, err)
	}

	return snapshot
}

func diffFixtures(before, after map[string]fixtureState) []string {
	var diffs []string
	seen := make(map[string]bool, len(before))
	for path, prev := range before {
		seen[path] = true
		next, ok := after[path]
		if !ok {
			diffs = append(diffs, "removed: "+path)
			continue
		}
		if prev != next {
			diffs = append(diffs, fmt.Sprintf(
				"modified: %s (before=%s mode=%s size=%d modtime=%s after=%s mode=%s size=%d modtime=%s)",
				path,
				prev.sum, prev.mode, prev.size, prev.modTime.Format(time.RFC3339Nano),
				next.sum, next.mode, next.size, next.modTime.Format(time.RFC3339Nano),
			))
		}
	}
	for path := range after {
		if !seen[path] {
			diffs = append(diffs, "added: "+path)
		}
	}
	sort.Strings(diffs)
	return diffs
}

func copyTree(t *testing.T, srcRoot, dstRoot string) {
	t.Helper()

	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcRoot, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			linkTarget, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			return os.Symlink(linkTarget, target)
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if writeErr := os.WriteFile(target, data, info.Mode().Perm()); writeErr != nil {
			return writeErr
		}
		return os.Chmod(target, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy %s to %s: %v", srcRoot, dstRoot, err)
	}
}

func runCoverageCardinalityReadOnlyInvariant(t *testing.T, root string, mutate func(string) error) []string {
	t.Helper()

	before := snapshotFixtures(t, root)
	if len(before) == 0 {
		t.Fatalf("expected fixture snapshot under %s to contain files", root)
	}

	analysistest.Run(t, root, Analyzer, "clean")

	if mutate != nil {
		if err := mutate(root); err != nil {
			t.Fatalf("mutate fixture tree: %v", err)
		}
	}

	after := snapshotFixtures(t, root)
	return diffFixtures(before, after)
}

func TestCompleteVerificationCoverageCardinality_ReadOnly(t *testing.T) {
	diffs := runCoverageCardinalityReadOnlyInvariant(t, analysistest.TestData(), nil)
	if len(diffs) > 0 {
		t.Fatalf("coverage-cardinality analyzer mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}
}

func TestCompleteVerificationCoverageCardinality_ReadOnlyReportsMutatedFixtureNames(t *testing.T) {
	srcRoot := analysistest.TestData()
	tempRoot := t.TempDir()
	copyTree(t, srcRoot, tempRoot)

	diffs := runCoverageCardinalityReadOnlyInvariant(t, tempRoot, func(root string) error {
		path := filepath.Join(root, "src", "clean", "clean.go")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.WriteString(f, "\n// mutated by TestCompleteVerificationCoverageCardinality_ReadOnlyReportsMutatedFixtureNames\n"); err != nil {
			return err
		}
		return nil
	})

	if len(diffs) == 0 {
		t.Fatal("expected mutation to be detected")
	}
	if got := strings.Join(diffs, "\n"); !strings.Contains(got, filepath.Join("src", "clean", "clean.go")) {
		t.Fatalf("expected mutated file name in diff, got:\n%s", got)
	}
}
