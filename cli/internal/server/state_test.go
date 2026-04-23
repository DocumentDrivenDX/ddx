package server

// Tests for ServerState migrate/sweep test-dir filtering (ddx-15f7ee0b Fix B)
// and the associated test helpers.

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// redirectStdLogger pipes the default stdlib logger to w for the caller's
// lifetime. Used by tests that need to assert on log.Printf output.
func redirectStdLogger(w io.Writer) func() {
	prevW := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(w)
	return func() {
		log.SetOutput(prevW)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	}
}

// withTestDirFilterDisabled is an internal helper used by the handful of
// tests that must exercise migration or sweep semantics using paths inside
// Go's test temp root (e.g. t.TempDir()). It flips the package-level
// override off for the duration of the test.
func withTestDirFilterDisabled(t *testing.T) {
	t.Helper()
	testDirFilterOverrideMu.Lock()
	prev := testDirFilterOverride
	testDirFilterOverride = func(string) bool { return false }
	testDirFilterOverrideMu.Unlock()
	t.Cleanup(func() {
		testDirFilterOverrideMu.Lock()
		testDirFilterOverride = prev
		testDirFilterOverrideMu.Unlock()
	})
}

// TestIsTestDirPathMatches exercises the allowed-pattern cases — paths that
// must be recognised as test-dir pollution.
func TestIsTestDirPathMatches(t *testing.T) {
	cases := []string{
		"/tmp/foo",
		"/tmp/TestFoo1234567890/001",
		"/tmp/TestAgentCheckSuccess3666443068/001",
		"/private/tmp/TestMac789/001",
		"/private/tmp/something",
		"/var/folders/xy/abc/T/TestFoo123/001",
		"/home/user/Projects/TestFooBar123/inner", // Test-named segment anywhere
	}
	for _, c := range cases {
		if !IsTestDirPath(c) {
			t.Errorf("expected IsTestDirPath(%q) = true", c)
		}
	}
}

// TestIsTestDirPathDoesNotMatch ensures real project paths are untouched.
func TestIsTestDirPathDoesNotMatch(t *testing.T) {
	cases := []string{
		"",
		"/home/user/Projects/ddx",
		"/home/user/Projects/helix",
		"/Users/erik/Projects/ddx",
		"/opt/ddx",
		"/srv/ddx",
		"/home/user/TestProjects/ddx", // no trailing digit — not a test name
		"/home/user/tmp/ddx",          // not prefix /tmp/
	}
	for _, c := range cases {
		if IsTestDirPath(c) {
			t.Errorf("expected IsTestDirPath(%q) = false", c)
		}
	}
}

// TestMigrateDropsTestDirPollution is the unit test required by ddx-15f7ee0b
// AC §2: seed a fixture with one real project, one missing /tmp path, one
// existing /tmp path, and one /private/tmp path — assert that only the real
// project survives migration.
func TestMigrateDropsTestDirPollution(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "test-node")
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	stateDir := filepath.Join(xdgDir, "ddx")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Make the "existing test-dir path" actually exist on disk — the sweep
	// must still drop it because the test-dir pattern wins over reachability.
	existingTestDir := filepath.Join(t.TempDir(), "TestBar456", "002")
	if err := os.MkdirAll(existingTestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The "real" project is a fake non-/tmp path that does not exist on
	// disk. It will be marked Unreachable + kept (recent LastSeen) — it
	// survives migration, which is all we care about for this AC.
	realPath := "/home/example/real-project"

	now := time.Now().UTC()
	state := map[string]any{
		"schema_version": "1",
		"node": map[string]any{
			"name":       "test-node",
			"id":         "node-seed",
			"started_at": now.Format(time.RFC3339),
			"last_seen":  now.Format(time.RFC3339),
		},
		"projects": []map[string]any{
			{
				"id":            "proj-real",
				"name":          "real-project",
				"path":          realPath,
				"registered_at": now.Format(time.RFC3339),
				"last_seen":     now.Format(time.RFC3339),
			},
			{
				"id":            "proj-missing",
				"name":          "missing",
				"path":          "/tmp/TestFoo123/001",
				"registered_at": now.Format(time.RFC3339),
				"last_seen":     now.Format(time.RFC3339),
			},
			{
				"id":            "proj-existing",
				"name":          "existing",
				"path":          existingTestDir,
				"registered_at": now.Format(time.RFC3339),
				"last_seen":     now.Format(time.RFC3339),
			},
			{
				"id":            "proj-mac",
				"name":          "mac",
				"path":          "/private/tmp/TestMac789/001",
				"registered_at": now.Format(time.RFC3339),
				"last_seen":     now.Format(time.RFC3339),
			},
		},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "server-state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// loadServerState invokes migrate. The default filter is enabled here,
	// so test-dir paths must all be dropped.
	s := loadServerState(stateDir, "test-node")

	if len(s.Projects) != 1 {
		for _, p := range s.Projects {
			t.Logf("  survivor: %s (%s)", p.ID, p.Path)
		}
		t.Fatalf("expected 1 project after migration, got %d", len(s.Projects))
	}
	if s.Projects[0].Path != realPath {
		t.Errorf("expected only %q to survive, got %q", realPath, s.Projects[0].Path)
	}
}

// TestSweepProjectsDropsTestDirPollution asserts that SweepProjects also
// drops test-dir paths unconditionally.
func TestSweepProjectsDropsTestDirPollution(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "test-node")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	s := &ServerState{dir: filepath.Join(os.Getenv("XDG_DATA_HOME"), "ddx")}
	now := time.Now().UTC()
	s.Projects = []ProjectEntry{
		{ID: "real", Path: "/home/example/proj", RegisteredAt: now, LastSeen: now},
		{ID: "tmp1", Path: "/tmp/TestFoo123/001", RegisteredAt: now, LastSeen: now},
		{ID: "tmp2", Path: "/private/tmp/TestBar456/002", RegisteredAt: now, LastSeen: now},
		{ID: "vf", Path: "/var/folders/ab/cd/T/TestBaz789/001", RegisteredAt: now, LastSeen: now},
	}

	result := s.SweepProjects()

	// Expect only the real non-/tmp path remains.
	if len(result) != 1 {
		for _, p := range result {
			t.Logf("  survivor: %s (%s)", p.ID, p.Path)
		}
		t.Fatalf("expected 1 project after sweep, got %d", len(result))
	}
	if !strings.HasPrefix(result[0].Path, "/home/") {
		t.Errorf("expected /home/... survivor, got %q", result[0].Path)
	}
}

// TestLoadServerStateLogsPhantomCleanup verifies the upgrade-log message is
// emitted when migrate drops at least one phantom entry.
func TestLoadServerStateLogsPhantomCleanup(t *testing.T) {
	t.Setenv("DDX_NODE_NAME", "test-node")
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	stateDir := filepath.Join(xdgDir, "ddx")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	state := map[string]any{
		"schema_version": "1",
		"node":           map[string]any{"name": "test-node", "id": "node-seed", "started_at": now.Format(time.RFC3339), "last_seen": now.Format(time.RFC3339)},
		"projects": []map[string]any{
			{"id": "p1", "path": "/tmp/TestFoo123/001", "registered_at": now.Format(time.RFC3339), "last_seen": now.Format(time.RFC3339)},
			{"id": "p2", "path": "/tmp/TestBar456/002", "registered_at": now.Format(time.RFC3339), "last_seen": now.Format(time.RFC3339)},
		},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(filepath.Join(stateDir, "server-state.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Redirect the default logger to capture output.
	var buf strings.Builder
	restoreLog := redirectStdLogger(&buf)
	defer restoreLog()

	_ = loadServerState(stateDir, "test-node")

	got := buf.String()
	if !strings.Contains(got, "pruned 2 phantom test-dir projects") {
		t.Errorf("expected cleanup log line, got %q", got)
	}
}
