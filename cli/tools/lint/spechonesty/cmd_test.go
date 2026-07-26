package spechonesty_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// moduleRoot resolves the cli/ module root from this test package location.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	return root
}

// runSpechonesty executes the spechonesty CLI against docsDir via go run.
func runSpechonesty(t *testing.T, docsDir string) (exitCode int, stdout, stderr string) {
	t.Helper()
	root := moduleRoot(t)
	cmd := exec.Command("go", "run", "-buildvcs=false", "./tools/lint/spechonesty/cmd/spechonesty", docsDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), outBuf.String(), errBuf.String()
	}
	t.Fatalf("go run spechonesty: %v\nstdout:\n%s\nstderr:\n%s", err, outBuf.String(), errBuf.String())
	return -1, "", ""
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
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
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("hash tree %s: %v", root, err)
	}
	return out
}

// TestSpechonestyCommandExitsNonZeroOnMissingStatus runs the command against
// a fixture docs tree containing an unstamped SD document and expects a
// non-zero exit with a missing-status diagnostic.
func TestSpechonestyCommandExitsNonZeroOnMissingStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-999-unstamped.md"),
		"---\n"+
			"ddx:\n"+
			"  id: SD-999\n"+
			"---\n"+
			"# Solution Design: Unstamped Fixture\n\n"+
			"No status stamp is present on this design document.\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for unstamped SD; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "missing_status") && !strings.Contains(strings.ToLower(combined), "missing status") {
		t.Fatalf("expected missing-status diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandExitsZeroForStampedDocs runs the command against
// fixture docs with body and frontmatter statuses and expects exit zero.
func TestSpechonestyCommandExitsZeroForStampedDocs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-998-body-status.md"),
		"# Solution Design: Body Status\n\n"+
			"**Status:** Proposed — free-text qualifier should not fail\n\n"+
			"Body stamp only.\n")
	writeFile(t, filepath.Join(dir, "technical-designs", "TD-998-frontmatter-status.md"),
		"---\n"+
			"ddx:\n"+
			"  id: TD-998\n"+
			"status: Complete\n"+
			"---\n"+
			"# Technical Design: Frontmatter Status\n\n"+
			"Frontmatter stamp only.\n")
	writeFile(t, filepath.Join(dir, "adr", "ADR-998-both.md"),
		"---\n"+
			"status: \"In Progress\"\n"+
			"---\n"+
			"# ADR-998: Both Encodings\n\n"+
			"**Status:** Implemented (body wins when both present)\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	if code != 0 {
		t.Fatalf("expected exit 0 for stamped docs; exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandExitsNonZeroOnDuplicateUserStoryID ensures the
// docs-directory CLI surfaces duplicate US-id collisions across feature
// documents and names both offending files in the diagnostic.
func TestSpechonestyCommandExitsNonZeroOnDuplicateUserStoryID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-087-alpha.md"),
		"---\n"+
			"ddx:\n"+
			"  id: FEAT-087\n"+
			"---\n"+
			"# FEAT-087 Alpha\n\n"+
			"**Status:** Proposed\n\n"+
			"## User Stories\n\n"+
			"### US-087: Alpha user story\n\n"+
			"Alpha acceptance criteria.\n")
	writeFile(t, filepath.Join(dir, "01-frame", "features", "FEAT-088-beta.md"),
		"---\n"+
			"ddx:\n"+
			"  id: FEAT-088\n"+
			"---\n"+
			"# FEAT-088 Beta\n\n"+
			"**Status:** Proposed\n\n"+
			"## User Stories\n\n"+
			"### US-087: Beta user story\n\n"+
			"Beta acceptance criteria.\n")

	code, stdout, stderr := runSpechonesty(t, dir)
	combined := stdout + stderr
	if code == 0 {
		t.Fatalf("expected non-zero exit for duplicate US-id; stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(combined, "duplicate_us_id") && !strings.Contains(strings.ToLower(combined), "duplicate user-story id") {
		t.Fatalf("expected duplicate US-id diagnostic, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, filepath.Join("01-frame", "features", "FEAT-087-alpha.md")) {
		t.Fatalf("expected diagnostic to name first feature file, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(combined, filepath.Join("01-frame", "features", "FEAT-088-beta.md")) {
		t.Fatalf("expected diagnostic to name second feature file, got exit=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// TestSpechonestyCommandIsReadOnly asserts fixture file contents are
// byte-identical before and after command execution.
func TestSpechonestyCommandIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "solution-designs", "SD-997-readonly.md"),
		"# Solution Design: Read-Only Check\n\n"+
			"**Status:** Deferred\n\n"+
			"Contents must not change.\n")
	writeFile(t, filepath.Join(dir, "notes", "README.md"),
		"# Non-design note without status\n\n"+
			"Should not be mutated either.\n")

	before := hashTree(t, dir)
	_, _, _ = runSpechonesty(t, dir)
	after := hashTree(t, dir)

	if len(before) != len(after) {
		t.Fatalf("file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, sum := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file removed: %s", path)
		}
		if got != sum {
			t.Fatalf("file content changed: %s", path)
		}
	}
}
