package workerprobe_test

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// TestWorker_RealAttemptEvents_FlowToServer is the wired-in end-to-end
// proof for ADR-022 step 2: it stands up the production server HTTP path
// (httptest.Server wrapping a real serverpkg.Server), points the worker at
// it via the production server.addr discovery (XDG_DATA_HOME), spawns a
// real `ddx work --local --once` subprocess against a fixture project, and
// asserts that the worker's bead-attempt loop events appear in the
// server's derived view (the worker-events.jsonl log) within 5s.
//
// This exercises the entire wire: writeLoopEvent → loopSink → TeeJSONL →
// workerprobe.Probe → POST /api/workers/register + /api/workers/{id}/event
// (or /backfill on the immediate first probe before any events fire).
func TestWorker_RealAttemptEvents_FlowToServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: spawns ddx work subprocess")
	}

	// Use a 'standard' fixture (5 sample beads) so the loop has at least
	// one bead to claim → at least one bead-scoped event flows. We don't
	// require the bead to succeed; ddx work will emit loop.start +
	// bead.claimed before any harness invocation.
	// Build a fresh ddx binary from the current tree so the subprocess
	// includes the workerprobe wiring under test (cannot rely on $PATH ddx
	// being current). Build before NewFixtureRepo so the same binary seeds
	// the fixture, avoiding two builds.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ddx")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = repoCLIDir(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ddx: %v\n%s", err, out)
	}
	t.Setenv("DDX_BIN", binPath)

	proj := testutils.NewFixtureRepo(t, "standard")
	bin := binPath

	// Real server — same constructor, same Handler, same requireTrusted gate.
	srv := serverpkg.New(":0", proj)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Production discovery path: write server.addr under XDG_DATA_HOME so
	// the worker subprocess (running with the same XDG_DATA_HOME) finds it
	// via serverpkg.ReadServerAddr.
	xdg := t.TempDir()
	addrDir := filepath.Join(xdg, "ddx")
	if err := os.MkdirAll(addrDir, 0o700); err != nil {
		t.Fatalf("mkdir addr dir: %v", err)
	}
	addrPath := filepath.Join(addrDir, "server.addr")
	addrPayload, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"pid": os.Getpid(), // alive so ReadServerAddr accepts it
	})
	if err := os.WriteFile(addrPath, addrPayload, 0o600); err != nil {
		t.Fatalf("write server.addr: %v", err)
	}

	// Spawn `ddx work --local --once`. We deliberately pass --no-review and
	// a bogus harness so the worker exits quickly after emitting loop.start
	// + bead.claimed; the harness failure is irrelevant to this test, which
	// asserts only that events flowed through the probe.
	cmd := exec.Command(bin, "work", "--local", "--once", "--no-review",
		"--harness", "noop", "--poll-interval", "0", "--project", proj)
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+xdg,
		"HOME="+t.TempDir(),
	)
	cmd.Dir = proj
	out, err := cmd.CombinedOutput()
	t.Logf("ddx work exit: err=%v\noutput:\n%s", err, out)

	// Wait up to 5s for the server's worker-events.jsonl log to receive at
	// least one event mirrored from the worker subprocess.
	logPath := filepath.Join(proj, ".ddx", "server", "worker-events.jsonl")
	deadline := time.Now().Add(5 * time.Second)
	var lines []string
	for time.Now().Before(deadline) {
		lines = readLogLines(t, logPath)
		if len(lines) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(lines) == 0 {
		t.Fatalf("no worker events appeared in %s within 5s; subprocess output:\n%s", logPath, out)
	}

	// Sanity-check: at least one mirrored event should be a loop.* or
	// bead.* envelope identifiable by the writeLoopEvent shape (kind field
	// in the loggedEvent wrapper).
	var kinds []string
	for _, line := range lines {
		var le struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal([]byte(line), &le)
		kinds = append(kinds, le.Kind)
	}
	t.Logf("mirrored event kinds: %v", kinds)
	hasExpected := false
	for _, k := range kinds {
		if strings.HasPrefix(k, "loop.") || strings.HasPrefix(k, "bead.") {
			hasExpected = true
			break
		}
	}
	if !hasExpected {
		t.Errorf("expected at least one loop.* or bead.* event mirrored, got kinds=%v", kinds)
	}
}

// repoCLIDir walks up from this file to find the cli/ directory (which
// holds go.mod). Used to build a fresh ddx binary for the integration test.
func repoCLIDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod from %s", wd)
		}
		dir = parent
	}
}

// readLogLines returns each newline-separated line of the file at path,
// or nil if the file does not yet exist.
func readLogLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}
