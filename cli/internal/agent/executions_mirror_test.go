package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// TestRenderMirrorPath verifies the four supported placeholders are
// substituted correctly and an unrelated placeholder is left in place.
func TestRenderMirrorPath(t *testing.T) {
	tpl := "/var/lib/ddx-mirror/{project}/{date}/{bead_id}/{attempt_id}"
	got := RenderMirrorPath(tpl, "axon", "20260418T061717-1993d293", "ddx-5930ed71")
	want := "/var/lib/ddx-mirror/axon/2026-04-18/ddx-5930ed71/20260418T061717-1993d293"
	if got != want {
		t.Fatalf("RenderMirrorPath: got %q want %q", got, want)
	}

	// Unrelated placeholders pass through unchanged.
	got = RenderMirrorPath("/x/{unknown}/{project}", "p", "20260418T010101-aa", "")
	want = "/x/{unknown}/p"
	if got != want {
		t.Fatalf("RenderMirrorPath unknown: got %q want %q", got, want)
	}
}

// TestIncludeFilter verifies that the default filter (empty include list)
// matches every part, and that a restricted list excludes embedded.
func TestIncludeFilter(t *testing.T) {
	all := IncludeFilter(nil)
	for _, name := range []string{"manifest.json", "prompt.md", "result.json", "usage.json", "checks.json", "embedded/agent-1.jsonl"} {
		if !all(name) {
			t.Errorf("default filter must accept %q", name)
		}
	}
	noEmbedded := IncludeFilter([]string{"manifest", "result"})
	cases := []struct {
		path    string
		want    bool
		comment string
	}{
		{"manifest.json", true, "manifest in list"},
		{"result.json", true, "result in list"},
		{"prompt.md", false, "prompt not in list"},
		{"embedded/agent-1.jsonl", false, "embedded not in list"},
	}
	for _, c := range cases {
		if got := noEmbedded(c.path); got != c.want {
			t.Errorf("filter(%q) = %v, want %v (%s)", c.path, got, c.want, c.comment)
		}
	}
}

// TestMirrorBundle_LocalDirRoundtrip verifies the local-dir backend can
// upload a bundle, append an index row, and fetch the bundle back so the
// bytes match the original.
func TestMirrorBundle_LocalDirRoundtrip(t *testing.T) {
	projectRoot := t.TempDir()
	const attemptID = "20260418T061717-1993d293"
	const beadID = "ddx-mirror-roundtrip"

	// Build the bundle on disk.
	bundleDir := filepath.Join(projectRoot, ".ddx", "executions", attemptID)
	if err := os.MkdirAll(filepath.Join(bundleDir, "embedded"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"manifest.json":            `{"attempt_id":"` + attemptID + `"}`,
		"prompt.md":                "task: do the thing",
		"result.json":              `{"status":"success"}`,
		"usage.json":               `{"tokens":42}`,
		"embedded/agent-001.jsonl": "trace line 1\ntrace line 2\n",
	}
	for name, body := range files {
		path := filepath.Join(bundleDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}

	// Mirror destination — outside the project root.
	mirrorRoot := t.TempDir()
	tpl := filepath.Join(mirrorRoot, "{project}/{attempt_id}")

	cfg := &config.ExecutionsMirrorConfig{
		Kind: "local",
		Path: tpl,
	}
	entry, err := MirrorBundle(MirrorRequest{
		ProjectRoot: projectRoot,
		AttemptID:   attemptID,
		BeadID:      beadID,
		BundleDir:   bundleDir,
		Cfg:         cfg,
	})
	if err != nil {
		t.Fatalf("MirrorBundle: %v", err)
	}

	expectedDest := filepath.Join(mirrorRoot, filepath.Base(projectRoot), attemptID)
	if entry.MirrorURI != expectedDest {
		t.Errorf("MirrorURI = %q, want %q", entry.MirrorURI, expectedDest)
	}
	if entry.AttemptID != attemptID || entry.BeadID != beadID {
		t.Errorf("entry ids: got attempt=%q bead=%q", entry.AttemptID, entry.BeadID)
	}
	if entry.ByteSize == 0 {
		t.Error("ByteSize must be non-zero after upload")
	}
	if entry.Kind != "local" {
		t.Errorf("Kind = %q, want local", entry.Kind)
	}

	// Index file appended.
	indexPath := filepath.Join(projectRoot, ExecutionsMirrorIndexFile)
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	if !strings.Contains(string(indexData), attemptID) {
		t.Errorf("index missing attempt id, got: %s", indexData)
	}

	// LookupMirrorEntry round-trip.
	got, err := LookupMirrorEntry(projectRoot, attemptID)
	if err != nil {
		t.Fatalf("LookupMirrorEntry: %v", err)
	}
	if got == nil || got.MirrorURI != expectedDest {
		t.Fatalf("LookupMirrorEntry: %#v", got)
	}

	// Fetch back into a different destination and verify byte-equal.
	restored := t.TempDir()
	backend, err := NewMirrorBackend("local")
	if err != nil {
		t.Fatalf("NewMirrorBackend: %v", err)
	}
	if err := backend.Fetch(entry.MirrorURI, restored); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(restored, name))
		if err != nil {
			t.Errorf("restored %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("restored %s mismatch:\n got: %q\nwant: %q", name, got, want)
		}
	}

	// Belt-and-suspenders: directory tree hashes match.
	if h1, h2 := dirHash(t, bundleDir), dirHash(t, restored); h1 != h2 {
		t.Errorf("dir hash mismatch after roundtrip: original=%s restored=%s", h1, h2)
	}
}

// TestMirrorOrLog_FailureDoesNotPanic exercises the failure path: an
// unsupported backend kind must be logged but never panic or be returned.
func TestMirrorOrLog_FailureDoesNotPanic(t *testing.T) {
	projectRoot := t.TempDir()
	bundleDir := filepath.Join(projectRoot, ".ddx", "executions", "x")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "result.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	async := false
	cfg := &config.ExecutionsMirrorConfig{
		Kind:  "s3", // unsupported in this implementation
		Path:  "s3://bucket/{attempt_id}",
		Async: &async,
	}
	MirrorOrLog(MirrorRequest{
		ProjectRoot: projectRoot,
		AttemptID:   "x",
		BeadID:      "ddx-x",
		BundleDir:   bundleDir,
		Cfg:         cfg,
	})

	logPath := filepath.Join(projectRoot, ExecutionsMirrorLogFile)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected mirror.log to exist after failure: %v", err)
	}
	if !strings.Contains(string(data), "ERR") {
		t.Errorf("mirror.log missing failure marker: %s", data)
	}
}

// TestExecuteBead_TriggersMirror exercises the wiring inside ExecuteBead so
// the bundle is mirrored once result.json is on disk. This is the happy-path
// integration of the worker → mirror handoff (sync mode for determinism).
func TestExecuteBead_TriggersMirror(t *testing.T) {
	const beadID = "ddx-mirror-int-01"

	projectRoot := setupArtifactTestProjectRoot(t)
	mirrorRoot := t.TempDir()
	async := false
	cfg := &config.ExecutionsMirrorConfig{
		Kind:  "local",
		Path:  filepath.Join(mirrorRoot, "{attempt_id}"),
		Async: &async,
	}
	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaaa000000000001",
		resultRev:   "aaaa000000000001",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}
	res, err := ExecuteBead(context.Background(), projectRoot, beadID,
		ExecuteBeadOptions{MirrorCfg: cfg, AgentRunner: &artifactTestAgentRunner{}}, gitOps)
	if err != nil {
		t.Fatalf("ExecuteBead: %v", err)
	}
	if res == nil || res.AttemptID == "" {
		t.Fatalf("expected non-nil result with attempt id")
	}

	mirroredManifest := filepath.Join(mirrorRoot, res.AttemptID, "manifest.json")
	if _, err := os.Stat(mirroredManifest); err != nil {
		t.Errorf("expected mirrored manifest at %s: %v", mirroredManifest, err)
	}
	mirroredResult := filepath.Join(mirrorRoot, res.AttemptID, "result.json")
	if _, err := os.Stat(mirroredResult); err != nil {
		t.Errorf("expected mirrored result at %s: %v", mirroredResult, err)
	}

	// Index file written with this attempt.
	entries, err := ReadMirrorIndex(projectRoot)
	if err != nil {
		t.Fatalf("ReadMirrorIndex: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.AttemptID == res.AttemptID && e.BeadID == beadID {
			found = true
		}
	}
	if !found {
		raw, _ := os.ReadFile(filepath.Join(projectRoot, ExecutionsMirrorIndexFile))
		t.Errorf("attempt %s not in mirror index. raw=%s", res.AttemptID, raw)
	}
}

// TestMirrorIndexEntry_JSONRoundtrip guarantees the on-disk shape is stable.
func TestMirrorIndexEntry_JSONRoundtrip(t *testing.T) {
	in := MirrorIndexEntry{
		AttemptID: "x",
		BeadID:    "ddx-x",
		MirrorURI: "/tmp/mirror/x",
		ByteSize:  123,
		Kind:      "local",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out MirrorIndexEntry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.AttemptID != in.AttemptID || out.BeadID != in.BeadID || out.MirrorURI != in.MirrorURI || out.ByteSize != in.ByteSize || out.Kind != in.Kind {
		t.Errorf("roundtrip mismatch: %+v vs %+v", in, out)
	}
}

func dirHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	walked := []string{}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		walked = append(walked, rel)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Stable order.
	for _, rel := range walked {
		f, err := os.Open(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(h, rel+"\x00")
		_, _ = io.Copy(h, f)
		_ = f.Close()
	}
	return hex.EncodeToString(h.Sum(nil))
}
