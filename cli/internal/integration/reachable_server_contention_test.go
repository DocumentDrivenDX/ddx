package integration

// reachable_server_contention_test.go — ADR-022 / TP-021 integration proof that
// independently started manual and server-managed DDx worker subprocesses
// serialize claims and landings through a reachable TLS server (ddx-d2d4c9d1).
//
// TestIntegration_WorkersCoordinateThroughReachableServer is the wired-in
// end-to-end guard: real TLS server, real git/bead fixture, deterministic
// Fizeau FakeProvider fixture (testseam), real manual `ddx work` and
// server-managed subprocesses, contested ready work → exactly one claimant
// and one landing.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// TestIntegration_WorkersCoordinateThroughReachableServer starts a real TLS
// DDx server, a server-managed worker subprocess, and a manual `ddx work`
// subprocess against one contested ready bead. Coordination mutations flow
// through the reachable server so claim + land serialize to a single durable
// claimant and a single landing commit/effect.
func TestIntegration_WorkersCoordinateThroughReachableServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns TLS server plus manual and managed ddx work subprocesses")
	}

	// Build before HOME override so the go toolchain does not re-download the
	// module graph into t.TempDir() (see workerprobe integration note).
	// Fizeau testseam binary supplies the deterministic FakeProvider fixture
	// (practices: no hosted providers).
	bin := testutils.BuildDDxFizeauTestSeamBinary(t)
	t.Setenv("DDX_BIN", bin)

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("locate git: %v", err)
	}
	minimalPATH := strings.Join([]string{
		filepath.Dir(bin),
		filepath.Dir(gitPath),
		"/usr/bin", "/bin", "/usr/sbin", "/sbin",
	}, string(os.PathListSeparator))

	home := t.TempDir()
	xdg := t.TempDir()
	// Restrict env for this process and every managed-worker child it launches
	// (launchManagedExecuteLoop inherits the parent environment).
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", minimalPATH)
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	proj := testutils.NewFixtureRepo(t, "minimal")
	if err := appendGitignore(proj, ".agents/", ".claude/", ".ddx/lifecycle-schema.json"); err != nil {
		t.Fatalf("append gitignore: %v", err)
	}
	if err := gitCommitAll(proj, "test: ignore auto-materialized paths"); err != nil {
		t.Fatalf("commit gitignore: %v", err)
	}

	subprocessEnv := []string{
		"HOME=" + home,
		"XDG_DATA_HOME=" + xdg,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"PATH=" + minimalPATH,
		"DDX_BIN=" + bin,
		"DDX_DISABLE_UPDATE_CHECK=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	}

	beadIDs, err := createBeads(bin, proj, subprocessEnv, 1, "Reachable-server contention bead")
	if err != nil {
		t.Fatalf("seed contested bead: %v", err)
	}
	beadID := beadIDs[0]
	if err := gitCommitAll(proj, "test: seed contested bead"); err != nil {
		t.Fatalf("commit contested bead: %v", err)
	}

	initialTip := gitRunIn(t, proj, "rev-parse", "refs/heads/main")

	// Deterministic Fizeau fixture: sleep widens the contention window after
	// claim so the loser still races the claim path; write + commit are the
	// sole landing effect under test.
	outFile := fmt.Sprintf("contention-%s.txt", beadID)
	commitMsg := "feat: " + beadID + " contention landing"
	subprocessEnv, seamLog := withFizeauTestPlan(t, subprocessEnv, concurrentTryFizeauPlan{
		SleepMS:       800,
		WritePath:     outFile,
		WriteContent:  "landed-once\n",
		CommitMessage: commitMsg,
	})
	// Managed workers inherit process env; pin the same Fizeau plan there.
	for _, kv := range subprocessEnv {
		if strings.HasPrefix(kv, fizeauTestPlanEnv+"=") {
			t.Setenv(fizeauTestPlanEnv, strings.TrimPrefix(kv, fizeauTestPlanEnv+"="))
			break
		}
	}

	// Real production server + TLS surface (not a mock coordinator).
	srv := serverpkg.New(":0", proj)
	t.Cleanup(func() { _ = srv.Shutdown() })
	srv.EnableManagedWorkers()
	srv.RegisterProject(proj)

	var (
		mutationPosts atomic.Int32
		landOps       atomic.Int32
		claimOps      atomic.Int32
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/coordination/mutations") {
			mutationPosts.Add(1)
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			var envelope struct {
				Operation string `json:"operation"`
			}
			if json.Unmarshal(body, &envelope) == nil {
				switch strings.TrimSpace(envelope.Operation) {
				case "claim":
					claimOps.Add(1)
				case "land":
					landOps.Add(1)
				}
			}
		}
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	// Self-signed cert: production clients already skip verify for local server.
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	// Production discovery: both manual and managed workers resolve the server
	// via XDG_DATA_HOME/ddx/server.addr (ReadServerAddr).
	addrDir := filepath.Join(xdg, "ddx")
	if err := os.MkdirAll(addrDir, 0o700); err != nil {
		t.Fatalf("mkdir server.addr dir: %v", err)
	}
	addrPayload, _ := json.Marshal(map[string]any{
		"url": ts.URL,
		"pid": os.Getpid(),
	})
	if err := os.WriteFile(filepath.Join(addrDir, "server.addr"), addrPayload, 0o600); err != nil {
		t.Fatalf("write server.addr: %v", err)
	}

	// Server-managed worker: real DDx subprocess launched by WorkerManager
	// (EnableManagedWorkers → launchManagedExecuteLoop → ddx work --server-managed).
	managedID, err := startManagedWorkOnce(t, ts, proj)
	if err != nil {
		t.Fatalf("start managed worker: %v", err)
	}

	// Manual worker: independent `ddx work --once` subprocess against the same
	// fixture + server.addr, racing the managed worker on the single ready bead.
	manualBuf := &bytes.Buffer{}
	manualCmd := exec.Command(bin, "work", "--once",
		"--harness", "fiz",
		"--attempt-backend", "local-clone",
		"--no-review", "--no-review-i-know-what-im-doing",
		"--project", proj,
	)
	manualCmd.Dir = proj
	manualCmd.Env = subprocessEnv
	manualCmd.Stdout = manualBuf
	manualCmd.Stderr = manualBuf
	if err := manualCmd.Start(); err != nil {
		t.Fatalf("start manual ddx work: %v", err)
	}

	// Wait for both workers to finish (managed via disk status; manual via Wait).
	manualDone := make(chan error, 1)
	go func() { manualDone <- manualCmd.Wait() }()

	managedRec := waitManagedWorkerStatus(t, proj, managedID, 120*time.Second)
	manualErr := <-manualDone
	manualOut := manualBuf.String()
	t.Logf("managed worker id=%s final state=%s status=%s error=%q",
		managedID, managedRec.State, managedRec.Status, managedRec.Error)
	t.Logf("manual ddx work exit: err=%v\noutput:\n%s", manualErr, manualOut)

	// --- AC2: exactly one claimant, one landing, no duplicate tracker/landing ---

	if mutationPosts.Load() < 1 {
		t.Fatalf("expected at least one POST /coordination/mutations through the reachable server, got %d",
			mutationPosts.Load())
	}
	if claimOps.Load() < 1 {
		t.Fatalf("expected at least one server-side claim mutation, got %d", claimOps.Load())
	}

	store := bead.NewStore(ddxroot.JoinProject(proj))
	got, err := store.Get(context.Background(), beadID)
	if err != nil {
		t.Fatalf("get contested bead: %v", err)
	}
	if got == nil {
		t.Fatalf("contested bead %s missing from store", beadID)
	}
	if got.Status != bead.StatusClosed {
		t.Fatalf("contested bead status=%q, want closed (exactly one worker must finish the work); notes=%q\nmanual:\n%s",
			got.Status, got.Notes, manualOut)
	}

	// Exactly one successful execute-bead event for this bead (duplicate would
	// mean both workers believed they completed the contested work).
	events, err := store.Events(beadID)
	if err != nil {
		t.Fatalf("read bead events: %v", err)
	}
	successEvents := 0
	for _, ev := range events {
		if ev.Kind != "execute-bead" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ev.Summary), string(agent.ExecuteBeadStatusSuccess)) {
			successEvents++
		}
	}
	if successEvents != 1 {
		var kinds []string
		for _, ev := range events {
			kinds = append(kinds, fmt.Sprintf("%s/%s", ev.Kind, ev.Summary))
		}
		t.Fatalf("expected exactly 1 execute-bead success event for contested bead, got %d; events=%v",
			successEvents, kinds)
	}

	// Exactly one landing effect on main: the work product file is present once
	// and the feature commit message appears once since the pre-contention tip.
	content, showErr := exec.Command("git", "-C", proj, "show", "HEAD:"+outFile).CombinedOutput()
	if showErr != nil {
		t.Fatalf("landing effect missing on main (HEAD:%s): %v\n%s", outFile, showErr, content)
	}
	if !strings.Contains(string(content), "landed-once") {
		t.Fatalf("landing file content unexpected: %q", content)
	}

	logOut, err := exec.Command("git", "-C", proj, "log", "--oneline",
		initialTip+"..HEAD", "--grep", commitMsg).CombinedOutput()
	if err != nil {
		t.Fatalf("git log landing commits: %v\n%s", err, logOut)
	}
	landingCommits := 0
	for _, line := range strings.Split(strings.TrimSpace(string(logOut)), "\n") {
		if strings.TrimSpace(line) != "" {
			landingCommits++
		}
	}
	if landingCommits != 1 {
		fullLog, _ := exec.Command("git", "-C", proj, "log", "--oneline", initialTip+"..HEAD").CombinedOutput()
		t.Fatalf("expected exactly 1 landing commit for contested work, got %d\nlanding-grep:\n%s\nfull range:\n%s",
			landingCommits, logOut, fullLog)
	}

	// Land mutation must have gone through the reachable server at least once.
	if landOps.Load() < 1 {
		t.Fatalf("expected at least one land mutation through the reachable server, got %d", landOps.Load())
	}

	finalTip := gitRunIn(t, proj, "rev-parse", "refs/heads/main")
	if finalTip == initialTip {
		t.Fatal("main tip did not advance; expected one landing to move the branch")
	}

	// Contested work must execute exactly once through the Fizeau fixture
	// (loser exits without a second bead_execution).
	requireFizeauTestSeamExecutions(t, seamLog, 1)
}

// startManagedWorkOnce POSTs a mode=once fiz-harness worker to the real
// server API so WorkerManager launches a --server-managed DDx subprocess.
// Execution behavior comes from DDX_FIZEAU_TEST_PLAN in the process env.
func startManagedWorkOnce(t *testing.T, ts *httptest.Server, proj string) (string, error) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"spec_version":       1,
		"mode":               "once",
		"harness":            "fiz",
		"attempt_backend":    "local-clone",
		"no_review":          true,
		"project_root":       proj,
		"opaque_passthrough": true,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/agent/workers/work", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// httptest serves on loopback; RemoteAddr is localhost so isTrusted passes.
	resp, err := ts.Client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("start managed worker: status %s body=%s", resp.Status, raw)
	}
	var rec struct {
		ID            string `json:"id"`
		ServerManaged bool   `json:"server_managed"`
		PID           int    `json:"pid"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return "", fmt.Errorf("decode worker record: %w body=%s", err, raw)
	}
	if rec.ID == "" {
		return "", fmt.Errorf("managed worker record missing id: %s", raw)
	}
	if !rec.ServerManaged {
		return "", fmt.Errorf("expected server_managed=true worker, got %s", raw)
	}
	if rec.PID <= 0 {
		return "", fmt.Errorf("expected managed worker PID > 0, got %s", raw)
	}
	return rec.ID, nil
}

// waitManagedWorkerStatus polls .ddx/workers/<id>/status.json until the
// managed subprocess reaches a terminal state or the timeout elapses.
func waitManagedWorkerStatus(t *testing.T, proj, workerID string, timeout time.Duration) managedWorkerStatus {
	t.Helper()
	path := filepath.Join(ddxroot.JoinProject(proj, "workers"), workerID, "status.json")
	deadline := time.Now().Add(timeout)
	var last managedWorkerStatus
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			if json.Unmarshal(data, &last) == nil {
				switch last.State {
				case "exited", "failed", "stopped", "reaped":
					return last
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("managed worker %s did not reach a terminal state within %s (last state=%q status=%q err=%q)",
		workerID, timeout, last.State, last.Status, last.Error)
	return last
}

type managedWorkerStatus struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	PID    int    `json:"pid,omitempty"`
}
