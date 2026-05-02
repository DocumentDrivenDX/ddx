package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// TestRunBundleFile_PathTraversalRejected covers AC: bundleFile rejects path
// traversal (`..`), absolute paths outside the bundle root, and symlink
// targets that escape the root.
func TestRunBundleFile_PathTraversalRejected(t *testing.T) {
	workDir, bundleID := setupRunBundleFixture(t)
	state := stateWithProject(workDir)

	cases := []struct {
		name string
		path string
	}{
		{"dotdot prefix", "../../etc/passwd"},
		{"dotdot mid", "subdir/../../escape.txt"},
		{"absolute path", "/etc/passwd"},
		{"absolute under workdir", filepath.Join(workDir, ".ddx", "config.yaml")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ok := state.GetRunBundleFileGraphQL("exec-"+bundleID, tc.path)
			if ok || out != nil {
				t.Fatalf("expected reject for %q, got ok=%v out=%+v", tc.path, ok, out)
			}
		})
	}
}

// TestRunBundleFile_SymlinkEscapeRejected creates a symlink inside the
// bundle that targets a file outside the root and confirms the resolver
// refuses to read it.
func TestRunBundleFile_SymlinkEscapeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not portable on windows test runners")
	}
	workDir, bundleID := setupRunBundleFixture(t)
	bundleRoot := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)
	outsideFile := filepath.Join(workDir, "outside-secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(bundleRoot, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	state := stateWithProject(workDir)
	out, ok := state.GetRunBundleFileGraphQL("exec-"+bundleID, "escape.txt")
	if ok || out != nil {
		t.Fatalf("expected symlink-escape rejection, got ok=%v out=%+v", ok, out)
	}
}

// TestRunBundleFile_SizeCap covers AC: files >64KB return truncated=true with
// sizeBytes populated and no inline content.
func TestRunBundleFile_SizeCap(t *testing.T) {
	workDir, bundleID := setupRunBundleFixture(t)
	bundleRoot := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)
	bigPath := filepath.Join(bundleRoot, "big.md")
	body := strings.Repeat("a", runBundleInlineMaxBytes+1)
	if err := os.WriteFile(bigPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	state := stateWithProject(workDir)
	out, ok := state.GetRunBundleFileGraphQL("exec-"+bundleID, "big.md")
	if !ok || out == nil {
		t.Fatal("expected file metadata, got nil")
	}
	if !out.Truncated {
		t.Fatal("expected truncated=true for >64KB file")
	}
	if out.Content != nil {
		t.Fatalf("expected no inline content for >64KB file, got %d bytes", len(*out.Content))
	}
	if out.SizeBytes != runBundleInlineMaxBytes+1 {
		t.Fatalf("expected sizeBytes=%d, got %d", runBundleInlineMaxBytes+1, out.SizeBytes)
	}
}

// TestRunBundleFile_WhitelistEnforcement covers AC: files outside the
// allowed-extension whitelist return truncated=true with no inline content,
// while small whitelisted files inline.
func TestRunBundleFile_WhitelistEnforcement(t *testing.T) {
	workDir, bundleID := setupRunBundleFixture(t)
	bundleRoot := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)

	// Whitelisted small file: should inline.
	allowed := filepath.Join(bundleRoot, "notes.md")
	if err := os.WriteFile(allowed, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-whitelisted small file: should be truncated=true.
	denied := filepath.Join(bundleRoot, "trace.bin")
	if err := os.WriteFile(denied, []byte("\x00\x01\x02"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := stateWithProject(workDir)

	out, ok := state.GetRunBundleFileGraphQL("exec-"+bundleID, "notes.md")
	if !ok || out == nil {
		t.Fatal("expected whitelisted file to resolve")
	}
	if out.Truncated || out.Content == nil || *out.Content != "hello" {
		t.Fatalf("expected inline content for notes.md, got truncated=%v content=%v", out.Truncated, out.Content)
	}

	out, ok = state.GetRunBundleFileGraphQL("exec-"+bundleID, "trace.bin")
	if !ok || out == nil {
		t.Fatal("expected non-whitelisted file to resolve metadata")
	}
	if !out.Truncated || out.Content != nil {
		t.Fatalf("expected truncated=true with no content for trace.bin, got %+v", out)
	}
	if out.SizeBytes != 3 {
		t.Fatalf("expected sizeBytes=3, got %d", out.SizeBytes)
	}
}

// TestRunBundleFile_MissingPathReturns404 covers AC: missing files are 404.
func TestRunBundleFile_MissingPathReturns404(t *testing.T) {
	workDir, bundleID := setupRunBundleFixture(t)
	state := stateWithProject(workDir)
	out, ok := state.GetRunBundleFileGraphQL("exec-"+bundleID, "does-not-exist.md")
	if ok || out != nil {
		t.Fatalf("expected nil for missing file, got ok=%v out=%+v", ok, out)
	}
}

// TestRunBundleFiles_Listing confirms bundleFiles returns one entry per file
// with size + mimeType populated.
func TestRunBundleFiles_Listing(t *testing.T) {
	workDir, bundleID := setupRunBundleFixture(t)
	bundleRoot := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)
	if err := os.WriteFile(filepath.Join(bundleRoot, "extra.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := listRunBundleFiles(bundleRoot)
	got := map[string]int{}
	for _, f := range files {
		got[f.Path] = f.Size
	}
	if got["manifest.json"] == 0 {
		t.Errorf("expected manifest.json in bundleFiles, got %v", got)
	}
	if got["result.json"] == 0 {
		t.Errorf("expected result.json in bundleFiles, got %v", got)
	}
	if got["extra.txt"] != 1 {
		t.Errorf("expected extra.txt size=1, got %v", got)
	}
	for _, f := range files {
		if f.MimeType == "" {
			t.Errorf("file %q has empty mimeType", f.Path)
		}
	}
}

// TestRunToolCalls_PersistedAtDrain covers AC: tool calls are read from the
// persisted SessionIndexEntry.ToolCalls field, NOT parsed from log files at
// resolver time. We write a session row directly and confirm the resolver
// returns its ToolCalls without scanning any log file.
func TestRunToolCalls_PersistedAtDrain(t *testing.T) {
	workDir := t.TempDir()
	writeConfig(t, workDir, `version: "1.0"`+"\n")
	state := stateWithProject(workDir)

	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	entry := agent.SessionIndexEntry{
		ID:          "sess-tools-001",
		Harness:     "claude",
		BillingMode: "paid",
		StartedAt:   now,
		ToolCalls: []agent.ToolCallEntry{
			{Tool: "Read", Input: `{"path":"a"}`, Output: "ok", Duration: 12},
			{Tool: "Bash", Input: `{"cmd":"ls"}`, Error: "exit 1"},
		},
	}
	if err := agent.AppendSessionIndex(agent.SessionLogDirForWorkDir(workDir), entry, now); err != nil {
		t.Fatal(err)
	}

	calls := state.GetRunToolCallsGraphQL("sess-tools-001")
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Tool != "Read" || calls[0].Seq != 0 {
		t.Errorf("call[0]=%+v want Tool=Read Seq=0", calls[0])
	}
	if calls[0].DurationMs == nil || *calls[0].DurationMs != 12 {
		t.Errorf("call[0] DurationMs=%v want 12", calls[0].DurationMs)
	}
	if calls[1].Error == nil || *calls[1].Error != "exit 1" {
		t.Errorf("call[1] Error=%v want 'exit 1'", calls[1].Error)
	}
}

// TestSessionIndexEntryFromResult_PersistsToolCalls confirms the drain-time
// pipeline (Result.ToolCalls → SessionIndexEntry.ToolCalls) is wired up.
func TestSessionIndexEntryFromResult_PersistsToolCalls(t *testing.T) {
	calls := []agent.ToolCallEntry{
		{Tool: "Read", Input: `{}`},
		{Tool: "Edit", Input: `{}`, Output: "ok"},
	}
	result := &agent.Result{Harness: "claude", ToolCalls: calls}
	now := time.Now().UTC()
	entry := agent.SessionIndexEntryFromResult("/tmp/x", agent.SessionIndexInputs{
		Harness: "claude",
	}, result, now, now)
	if len(entry.ToolCalls) != 2 {
		t.Fatalf("expected 2 persisted tool calls, got %d", len(entry.ToolCalls))
	}
	if entry.ToolCalls[0].Tool != "Read" || entry.ToolCalls[1].Tool != "Edit" {
		t.Fatalf("persisted tool calls do not match input: %+v", entry.ToolCalls)
	}
}

// TestResolveRunBundlePath_DotDotEscape covers the canonical-path check on
// the resolver helper directly (white-box, complements the tests above).
func TestResolveRunBundlePath_DotDotEscape(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"../escape",
		"a/../../escape",
		"./../../etc/passwd",
		"",
	}
	for _, p := range cases {
		if _, err := resolveRunBundlePath(root, p); err == nil {
			t.Errorf("expected error rejecting %q, got nil", p)
		}
	}
	// Sanity: a normal relative path resolves without error.
	if _, err := resolveRunBundlePath(root, "a/b.txt"); err != nil {
		t.Errorf("unexpected error for normal path: %v", err)
	}
}

// setupRunBundleFixture writes a minimal bundle directory under workDir and
// returns (workDir, bundleID). The bundle contains manifest.json and
// result.json so try-layer Run synthesis works.
func setupRunBundleFixture(t *testing.T) (string, string) {
	t.Helper()
	workDir := t.TempDir()
	writeConfig(t, workDir, `version: "1.0"`+"\n")

	bundleID := "20260502T120000-deadbeef"
	bundleDir := filepath.Join(workDir, agent.ExecuteBeadArtifactDir, bundleID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), []byte(`{"attempt_id":"`+bundleID+`","verdict":"success"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), []byte(`{"verdict":"success","exit_code":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return workDir, bundleID
}
