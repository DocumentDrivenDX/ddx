package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestDocAuditCommand_CleanRepoExitsZero verifies the no-issues path:
// success, exit code 0 (no error returned), and a short "clean" message.
func TestDocAuditCommand_CleanRepoExitsZero(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: doc.a\n---\n# A\n")
	writeTestFile(t, dir, "docs/b.md",
		"---\nddx:\n  id: doc.b\n  depends_on:\n    - doc.a\n---\n# B\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "audit"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("doc audit on clean repo should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "clean") {
		t.Errorf("expected clean message, got: %q", out.String())
	}
}

// TestDocAuditCommand_IssuesExitOne verifies the failure path: a duplicate ID
// fixture must produce a grouped report and a non-zero exit via ExitError.
func TestDocAuditCommand_IssuesExitOne(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: dup.id\n---\n# A\n")
	writeTestFile(t, dir, "docs/b.md", "---\nddx:\n  id: dup.id\n---\n# B\n")
	writeTestFile(t, dir, "docs/c.md",
		"---\nddx:\n  id: doc.c\n  depends_on:\n    - ghost.id\n---\n# C\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "audit"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	err := root.Execute()
	if err == nil {
		t.Fatal("doc audit on broken repo must return error for exit 1")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitCodeGeneralError {
		t.Errorf("expected exit code %d, got %d", ExitCodeGeneralError, exitErr.Code)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "duplicate_id") {
		t.Errorf("expected duplicate_id group in output, got: %q", outStr)
	}
	if !strings.Contains(outStr, "missing_dep") {
		t.Errorf("expected missing_dep group in output, got: %q", outStr)
	}
	if !strings.Contains(errOut.String(), "integrity issue") {
		t.Errorf("expected stderr summary, got: %q", errOut.String())
	}
}

// TestDocAuditCommand_JSONOutput verifies the --json flag emits an array of
// issues consumable by CI tooling.
func TestDocAuditCommand_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: shared\n---\n# A\n")
	writeTestFile(t, dir, "docs/b.md", "---\nddx:\n  id: shared\n---\n# B\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "audit", "--json"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	// --json always succeeds (exit 0) so piping into jq works.
	if err := root.Execute(); err != nil {
		t.Fatalf("doc audit --json should not return error, got: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, `"kind"`) {
		t.Errorf("expected JSON output with kind field, got: %q", outStr)
	}
	if !strings.Contains(outStr, `"duplicate_id"`) {
		t.Errorf("expected duplicate_id in JSON output, got: %q", outStr)
	}
}
