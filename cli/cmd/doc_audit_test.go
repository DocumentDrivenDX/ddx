package cmd

import (
	"bytes"
	"encoding/json"
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

// TestDocAuditCommand_JSONOutputExitsOne verifies the --json flag emits an
// array of issues while preserving the audit command's non-zero exit contract.
func TestDocAuditCommand_JSONOutputExitsOne(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: shared\n---\n# A\n")
	writeTestFile(t, dir, "docs/b.md", "---\nddx:\n  id: shared\n---\n# B\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "audit", "--json"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("doc audit --json on broken repo must return error for exit 1")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitCodeGeneralError {
		t.Errorf("expected exit code %d, got %d", ExitCodeGeneralError, exitErr.Code)
	}

	outStr := out.String()
	if !strings.Contains(outStr, `"kind"`) {
		t.Errorf("expected JSON output with kind field, got: %q", outStr)
	}
	if !strings.Contains(outStr, `"duplicate_id"`) {
		t.Errorf("expected duplicate_id in JSON output, got: %q", outStr)
	}
}

func TestDocAuditCommand_JSONExitZeroOverride(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: shared\n---\n# A\n")
	writeTestFile(t, dir, "docs/b.md", "---\nddx:\n  id: shared\n---\n# B\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "audit", "--json", "--exit-zero"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("doc audit --json --exit-zero should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), `"duplicate_id"`) {
		t.Errorf("expected duplicate_id in JSON output, got: %q", out.String())
	}
}

func TestDocsAuditAlias(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/a.md", "---\nddx:\n  id: doc.a\n---\n# A\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"docs", "audit"})

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("docs audit alias should succeed on clean repo, got: %v", err)
	}
	if !strings.Contains(out.String(), "clean") {
		t.Errorf("expected clean message, got: %q", out.String())
	}
}

func TestDocStaleCommand_BucketsActiveHistoricalAndNoise(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/parent.md", "---\nddx:\n  id: doc.parent\n---\n# Parent\n")
	writeTestFile(t, dir, "docs/active.md", "---\nddx:\n  id: doc.active\n  status: draft\n  depends_on:\n    - doc.parent\n  review:\n    deps:\n      doc.parent: wrong\n---\n# Active\n")
	writeTestFile(t, dir, "docs/historical.md", "---\nddx:\n  id: doc.historical\n  depends_on:\n    - doc.parent\n  review:\n    deps:\n      doc.parent: wrong\n---\n> **Historical** — archived planning note.\n# Historical\n")
	writeTestFile(t, dir, "docs/reference.md", "---\nddx:\n  id: doc.reference\n  status: published\n  depends_on:\n    - doc.parent\n  review:\n    deps:\n      doc.parent: wrong\n---\n# Reference\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "stale", "--json"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("doc stale --json should succeed, got: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got: %q", errOut.String())
	}

	var report struct {
		ActiveActionable []struct {
			ID string `json:"id"`
		} `json:"active_actionable"`
		HistoricalSuperseded []struct {
			ID string `json:"id"`
		} `json:"historical_superseded"`
		Noise []struct {
			ID string `json:"id"`
		} `json:"noise"`
		Summary struct {
			Total                int `json:"total"`
			ActiveActionable     int `json:"active_actionable"`
			HistoricalSuperseded int `json:"historical_superseded"`
			Noise                int `json:"noise"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("expected bucketed JSON, got %q: %v", out.String(), err)
	}
	if report.Summary.Total != 3 {
		t.Fatalf("expected total 3 stale docs, got %+v", report.Summary)
	}
	if report.Summary.ActiveActionable != 1 || report.Summary.HistoricalSuperseded != 1 || report.Summary.Noise != 1 {
		t.Fatalf("unexpected summary buckets: %+v", report.Summary)
	}
	if len(report.ActiveActionable) != 1 || report.ActiveActionable[0].ID != "doc.active" {
		t.Fatalf("expected active doc in actionable bucket, got %+v", report.ActiveActionable)
	}
	if len(report.HistoricalSuperseded) != 1 || report.HistoricalSuperseded[0].ID != "doc.historical" {
		t.Fatalf("expected historical doc in historical bucket, got %+v", report.HistoricalSuperseded)
	}
	if len(report.Noise) != 1 || report.Noise[0].ID != "doc.reference" {
		t.Fatalf("expected reference doc in noise bucket, got %+v", report.Noise)
	}
}
