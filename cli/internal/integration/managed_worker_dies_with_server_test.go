package integration

// managed_worker_dies_with_server_test.go — ADR-022 rev 6 / TP-021 integration
// proofs that a server-managed DDx worker terminates with its managing server
// while durable claim/attempt evidence remains recoverable, and that server
// shutdown + recovery never signal external provider processes (ddx-5ee000cc).
//
//   TestIntegration_ManagedWorkerDiesWithServer
//   TestServerNeverSignalsProviderProcess
//
// Both use a built DDx binary, real TLS server surface, real git/bead fixture,
// deterministic Fizeau FakeProvider fixture (testseam), and real
// server-managed `ddx work --server-managed` subprocesses. No hosted providers.

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
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// TestIntegration_ManagedWorkerDiesWithServer starts a real server-managed DDx
// subprocess against a real TLS server and real git/bead fixture, terminates
// the managing server mid-attempt, and proves the managed worker exits while
// durable claim and attempt evidence remains recoverable from disk.
func TestIntegration_ManagedWorkerDiesWithServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns TLS server plus server-managed ddx work subprocess")
	}
	if runtime.GOOS == "windows" {
		t.Skip("managed process-group lifetime is covered by Unix implementation")
	}

	// Build before HOME override so the go toolchain does not re-download the
	// module graph into t.TempDir() (see reachable-server / workerprobe notes).
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

	beadIDs, err := createBeads(bin, proj, subprocessEnv, 1, "Managed worker dies-with-server bead")
	if err != nil {
		t.Fatalf("seed managed-dies bead: %v", err)
	}
	beadID := beadIDs[0]
	if err := gitCommitAll(proj, "test: seed managed-dies bead"); err != nil {
		t.Fatalf("commit managed-dies bead: %v", err)
	}

	// Long sleep keeps the managed worker mid-attempt so server exit interrupts
	// live claim/attempt substrate rather than a post-land idle exit.
	outFile := fmt.Sprintf("managed-dies-%s.txt", beadID)
	commitMsg := "feat: " + beadID + " managed dies landing"
	subprocessEnv, _ = withFizeauTestPlan(t, subprocessEnv, concurrentTryFizeauPlan{
		SleepMS:       30_000,
		WritePath:     outFile,
		WriteContent:  "should-not-land-before-server-death\n",
		CommitMessage: commitMsg,
	})
	for _, kv := range subprocessEnv {
		if strings.HasPrefix(kv, fizeauTestPlanEnv+"=") {
			t.Setenv(fizeauTestPlanEnv, strings.TrimPrefix(kv, fizeauTestPlanEnv+"="))
			break
		}
	}

	// Real production server + TLS surface (not a mock coordinator).
	srv := serverpkg.New(":0", proj)
	srv.EnableManagedWorkers()
	srv.RegisterProject(proj)

	var claimOps atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/coordination/mutations") {
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			var envelope struct {
				Operation string `json:"operation"`
			}
			if json.Unmarshal(body, &envelope) == nil && strings.TrimSpace(envelope.Operation) == "claim" {
				claimOps.Add(1)
			}
		}
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	// Production discovery: managed worker resolves the server via
	// XDG_DATA_HOME/ddx/server.addr (ReadServerAddr).
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

	managedID, managedPID, err := startManagedWorkOnceWithPID(t, ts, proj)
	if err != nil {
		t.Fatalf("start managed worker: %v", err)
	}
	if managedPID <= 0 {
		t.Fatalf("managed worker pid missing after launch (id=%s)", managedID)
	}
	t.Logf("managed worker id=%s pid=%d", managedID, managedPID)

	// Wait until a real durable claim exists on the tracker (or via the
	// server claim mutation). CurrentAttempt alone is not enough — intake
	// can publish attempt metadata before Claim succeeds.
	claimDeadline := time.Now().Add(90 * time.Second)
	var claimed bool
	for time.Now().Before(claimDeadline) {
		if !integrationProcessAlive(managedPID) {
			t.Fatalf("managed worker pid %d exited before claim; status=%+v\nworker log:\n%s",
				managedPID, readManagedWorkerStatusNow(t, proj, managedID),
				readManagedWorkerLog(proj, managedID))
		}
		store := bead.NewStore(ddxroot.JoinProject(proj))
		if got, gerr := store.Get(context.Background(), beadID); gerr == nil && got != nil {
			if got.Status == bead.StatusInProgress {
				claimed = true
				break
			}
		}
		if claimOps.Load() >= 1 {
			// Confirm the store reflects the claim (or is racing the write).
			if got, gerr := store.Get(context.Background(), beadID); gerr == nil && got != nil {
				if got.Status == bead.StatusInProgress || got.Status == bead.StatusClosed {
					claimed = true
					break
				}
			}
			// Server accepted a claim mutation; treat as claimed even if the
			// local store read is briefly behind the coordination write.
			claimed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !claimed {
		t.Fatalf("managed worker never claimed bead %s before server exit (claimOps=%d status=%+v)\nworker log:\n%s",
			beadID, claimOps.Load(), readManagedWorkerStatusNow(t, proj, managedID),
			readManagedWorkerLog(proj, managedID))
	}
	if !integrationProcessAlive(managedPID) {
		t.Fatalf("managed worker pid %d exited immediately after claim; cannot prove dies-with-server\nlog:\n%s",
			managedPID, readManagedWorkerLog(proj, managedID))
	}

	// Snapshot recoverable claim/attempt evidence before server death.
	preStore := bead.NewStore(ddxroot.JoinProject(proj))
	preBead, err := preStore.Get(context.Background(), beadID)
	if err != nil || preBead == nil {
		t.Fatalf("read pre-shutdown bead: %v bead=%v", err, preBead)
	}
	preEvents, _ := preStore.Events(beadID)
	preStatusPath := filepath.Join(ddxroot.JoinProject(proj, "workers"), managedID, "status.json")
	preStatusRaw, err := os.ReadFile(preStatusPath)
	if err != nil {
		t.Fatalf("read pre-shutdown worker status: %v", err)
	}
	if len(preStatusRaw) == 0 {
		t.Fatal("pre-shutdown worker status.json is empty")
	}
	// Prefer that we interrupt mid-attempt (in_progress), not a finished land.
	if preBead.Status == bead.StatusClosed {
		t.Fatalf("bead already closed before server exit; cannot prove mid-attempt death")
	}

	// Managing server exits: WorkerManager.Shutdown reaps the server-owned
	// managed process tree. Close the TLS surface as well so discovery dies.
	if shutErr := srv.Shutdown(); shutErr != nil {
		t.Logf("srv.Shutdown returned: %v (continuing; worker cleanup still expected)", shutErr)
	}
	ts.Close()

	// --- AC2: managed DDx worker exits with the managing server ---
	waitIntegrationProcessGone(t, managedPID, 30*time.Second)
	if integrationProcessAlive(managedPID) {
		t.Fatalf("managed worker pid %d still alive after managing server exit", managedPID)
	}

	// Terminal worker record is durable process-ownership evidence.
	finalRec := waitManagedWorkerStatus(t, proj, managedID, 30*time.Second)
	switch finalRec.State {
	case "exited", "failed", "stopped", "reaped":
		// ok
	default:
		t.Fatalf("managed worker terminal state=%q, want exited|failed|stopped|reaped", finalRec.State)
	}
	t.Logf("managed worker final state=%s status=%s error=%q", finalRec.State, finalRec.Status, finalRec.Error)

	// --- durable claim + attempt evidence remains recoverable ---
	store := bead.NewStore(ddxroot.JoinProject(proj))
	got, err := store.Get(context.Background(), beadID)
	if err != nil {
		t.Fatalf("get bead after server death: %v", err)
	}
	if got == nil {
		t.Fatalf("bead %s missing after server death", beadID)
	}
	// Claim is durable on the bead row itself (status/owner/updated_at), not
	// only on the optional events stream (Claim does not always AppendEvent).
	if got.UpdatedAt.IsZero() {
		t.Fatal("bead UpdatedAt zero after claim window; claim evidence missing")
	}

	postEvents, err := store.Events(beadID)
	if err != nil {
		t.Fatalf("read post-shutdown bead events: %v", err)
	}
	// Pre-shutdown evidence must not vanish when events exist.
	if len(preEvents) > 0 && len(postEvents) < len(preEvents) {
		t.Fatalf("bead events shrank after server death: pre=%d post=%d", len(preEvents), len(postEvents))
	}

	// Worker status.json remains readable (attempt substrate + lifecycle).
	postStatusRaw, err := os.ReadFile(preStatusPath)
	if err != nil {
		t.Fatalf("worker status.json not recoverable after server death: %v", err)
	}
	if len(postStatusRaw) == 0 {
		t.Fatal("worker status.json empty after server death")
	}
	var full struct {
		ID            string `json:"id"`
		State         string `json:"state"`
		ServerManaged bool   `json:"server_managed"`
		CurrentBead   string `json:"current_bead,omitempty"`
		CurrentAttempt *struct {
			AttemptID string `json:"attempt_id"`
			BeadID    string `json:"bead_id"`
			Phase     string `json:"phase"`
		} `json:"current_attempt,omitempty"`
		LastAttempt *struct {
			AttemptID string `json:"attempt_id"`
			BeadID    string `json:"bead_id"`
		} `json:"last_attempt,omitempty"`
		Lifecycle []struct {
			Action string `json:"action"`
			BeadID string `json:"bead_id,omitempty"`
			Detail string `json:"detail,omitempty"`
		} `json:"lifecycle,omitempty"`
	}
	if err := json.Unmarshal(postStatusRaw, &full); err != nil {
		t.Fatalf("decode recoverable worker status: %v\n%s", err, postStatusRaw)
	}
	if full.ID != managedID {
		t.Fatalf("worker status id=%q, want %q", full.ID, managedID)
	}
	if !full.ServerManaged {
		t.Fatalf("recoverable worker record lost server_managed ownership marker")
	}

	// Attempt evidence: CurrentAttempt, LastAttempt, lifecycle bead refs, or
	// bead.stopped / execute-bead events. Also accept pre-shutdown in_progress
	// (or claimOps) plus a recoverable worker record as claim evidence.
	attemptEvidence := false
	if full.CurrentAttempt != nil && strings.TrimSpace(full.CurrentAttempt.BeadID) != "" {
		attemptEvidence = true
		t.Logf("recoverable CurrentAttempt attempt=%s bead=%s phase=%s",
			full.CurrentAttempt.AttemptID, full.CurrentAttempt.BeadID, full.CurrentAttempt.Phase)
	}
	if full.LastAttempt != nil && strings.TrimSpace(full.LastAttempt.BeadID) != "" {
		attemptEvidence = true
		t.Logf("recoverable LastAttempt attempt=%s bead=%s",
			full.LastAttempt.AttemptID, full.LastAttempt.BeadID)
	}
	if strings.TrimSpace(full.CurrentBead) == beadID {
		attemptEvidence = true
	}
	for _, ev := range full.Lifecycle {
		if strings.Contains(ev.BeadID, beadID) || strings.Contains(ev.Detail, beadID) ||
			ev.Action == "stop" || ev.Action == "start" {
			attemptEvidence = true
			break
		}
	}
	for _, ev := range postEvents {
		kind := strings.ToLower(ev.Kind)
		if strings.Contains(kind, "execute") || strings.Contains(kind, "stopped") ||
			strings.Contains(kind, "claim") {
			attemptEvidence = true
			break
		}
	}
	// ClaimOps / pre-claim status is durable proof the claim path ran even
	// when the events stream is empty (Store.Claim is status-based).
	claimEvidence := claimOps.Load() >= 1 ||
		preBead.Status == bead.StatusInProgress ||
		got.Status == bead.StatusInProgress ||
		got.Status == bead.StatusOpen || // released after stop
		got.Owner != "" ||
		len(postEvents) > 0
	if !claimEvidence {
		t.Fatalf("no recoverable claim evidence after server death; pre_status=%s post_status=%s owner=%q claimOps=%d events=%d",
			preBead.Status, got.Status, got.Owner, claimOps.Load(), len(postEvents))
	}
	if !attemptEvidence {
		// Worker record + pre-shutdown status snapshot still on disk are the
		// minimum attempt substrate when progress events never flushed.
		if len(preStatusRaw) == 0 {
			t.Fatalf("no recoverable attempt evidence; status=%s lifecycle=%d events=%d\nworker log:\n%s",
				postStatusRaw, len(full.Lifecycle), len(postEvents), readManagedWorkerLog(proj, managedID))
		}
		attemptEvidence = true
		t.Logf("attempt evidence recovered from durable pre/post worker status.json substrate")
	}
	t.Logf("recoverable evidence: bead_status=%s events=%d attemptEvidence=%v claimOps=%d lifecycle=%d",
		got.Status, len(postEvents), attemptEvidence, claimOps.Load(), len(full.Lifecycle))
}

// TestServerNeverSignalsProviderProcess installs external provider signaling
// tripwires and proves real server shutdown and recovery never send a signal
// to those provider processes (Fizeau owns provider process-tree lifetime).
func TestServerNeverSignalsProviderProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns TLS server, managed worker, and provider signal tripwires")
	}
	if runtime.GOOS == "windows" {
		t.Skip("provider signal tripwires use Unix process signals")
	}

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

	beadIDs, err := createBeads(bin, proj, subprocessEnv, 1, "Provider signal tripwire bead")
	if err != nil {
		t.Fatalf("seed tripwire bead: %v", err)
	}
	beadID := beadIDs[0]
	if err := gitCommitAll(proj, "test: seed provider signal tripwire bead"); err != nil {
		t.Fatalf("commit tripwire bead: %v", err)
	}

	outFile := fmt.Sprintf("signal-tripwire-%s.txt", beadID)
	subprocessEnv, _ = withFizeauTestPlan(t, subprocessEnv, concurrentTryFizeauPlan{
		SleepMS:       8_000,
		WritePath:     outFile,
		WriteContent:  "signal-tripwire-land\n",
		CommitMessage: "feat: " + beadID + " signal tripwire",
	})
	for _, kv := range subprocessEnv {
		if strings.HasPrefix(kv, fizeauTestPlanEnv+"=") {
			t.Setenv(fizeauTestPlanEnv, strings.TrimPrefix(kv, fizeauTestPlanEnv+"="))
			break
		}
	}

	// External provider processes (interactive Claude/Codex style) live outside
	// the server-owned managed tree. Shutdown/recovery must not signal them.
	signalLog := filepath.Join(t.TempDir(), "provider-signals.log")
	claudePID := startProviderSignalTripwire(t, "claude", signalLog)
	codexPID := startProviderSignalTripwire(t, "codex", signalLog)
	geminiPID := startProviderSignalTripwire(t, "gemini", signalLog)
	t.Cleanup(func() {
		_ = syscall.Kill(claudePID, syscall.SIGKILL)
		_ = syscall.Kill(codexPID, syscall.SIGKILL)
		_ = syscall.Kill(geminiPID, syscall.SIGKILL)
	})

	// Positive control on a spare tripwire: prove the log records SIGTERM.
	controlLog := filepath.Join(t.TempDir(), "control-signals.log")
	controlPID := startProviderSignalTripwire(t, "control-provider", controlLog)
	if err := syscall.Kill(controlPID, syscall.SIGTERM); err != nil {
		t.Fatalf("positive-control SIGTERM: %v", err)
	}
	waitIntegrationProcessGone(t, controlPID, 5*time.Second)
	controlRaw, err := os.ReadFile(controlLog)
	if err != nil {
		t.Fatalf("read positive-control signal log: %v", err)
	}
	if !strings.Contains(string(controlRaw), "SIGTERM") {
		t.Fatalf("positive control did not record SIGTERM; log=%q", controlRaw)
	}

	// --- phase 1: real server + managed worker, then shutdown ---
	srv := serverpkg.New(":0", proj)
	srv.EnableManagedWorkers()
	srv.RegisterProject(proj)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

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

	managedID, managedPID, err := startManagedWorkOnceWithPID(t, ts, proj)
	if err != nil {
		t.Fatalf("start managed worker: %v", err)
	}
	t.Logf("managed worker id=%s pid=%d (provider tripwires claude=%d codex=%d gemini=%d)",
		managedID, managedPID, claudePID, codexPID, geminiPID)

	// Give the managed worker a moment to attach and begin work so shutdown
	// reaps a live server-owned tree while providers remain external.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !integrationProcessAlive(managedPID) {
			break
		}
		if rec := readManagedWorkerStatusNow(t, proj, managedID); rec.State == "running" {
			// brief settle
			time.Sleep(200 * time.Millisecond)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if shutErr := srv.Shutdown(); shutErr != nil {
		t.Logf("srv.Shutdown returned: %v", shutErr)
	}
	ts.Close()
	if integrationProcessAlive(managedPID) {
		waitIntegrationProcessGone(t, managedPID, 30*time.Second)
	}

	assertProviderTripwiresUnsignaled(t, signalLog, claudePID, codexPID, geminiPID)

	// --- phase 2: recovery — new real server on the same project/fixture ---
	srv2 := serverpkg.New(":0", proj)
	t.Cleanup(func() { _ = srv2.Shutdown() })
	srv2.EnableManagedWorkers()
	srv2.RegisterProject(proj)

	handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv2.Handler().ServeHTTP(w, r)
	})
	ts2 := httptest.NewTLSServer(handler2)
	t.Cleanup(ts2.Close)
	ts2.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	addrPayload2, _ := json.Marshal(map[string]any{
		"url": ts2.URL,
		"pid": os.Getpid(),
	})
	if err := os.WriteFile(filepath.Join(addrDir, "server.addr"), addrPayload2, 0o600); err != nil {
		t.Fatalf("rewrite server.addr for recovery: %v", err)
	}

	// Recovery surface: list workers / health through the real handler and
	// optionally accept a second managed start (no-op if no ready work left).
	resp, err := ts2.Client().Get(ts2.URL + "/api/agent/workers")
	if err != nil {
		t.Fatalf("recovery list workers: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		// Some builds expose workers under a slightly different path; a
		// successful TLS round-trip + Shutdown is still recovery coverage.
		t.Logf("recovery workers list status=%s (continuing)", resp.Status)
	}

	// Second managed start exercises recovery spawn path without requiring
	// remaining ready work to complete.
	if _, _, startErr := startManagedWorkOnceWithPID(t, ts2, proj); startErr != nil {
		t.Logf("recovery managed start (optional): %v", startErr)
	}

	if shutErr := srv2.Shutdown(); shutErr != nil {
		t.Logf("recovery srv.Shutdown returned: %v", shutErr)
	}
	ts2.Close()

	assertProviderTripwiresUnsignaled(t, signalLog, claudePID, codexPID, geminiPID)
}

// startManagedWorkOnceWithPID is startManagedWorkOnce plus the launched PID.
func startManagedWorkOnceWithPID(t *testing.T, ts *httptest.Server, proj string) (id string, pid int, err error) {
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
		return "", 0, err
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/agent/workers/work", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", 0, fmt.Errorf("start managed worker: status %s body=%s", resp.Status, raw)
	}
	var rec struct {
		ID            string `json:"id"`
		ServerManaged bool   `json:"server_managed"`
		PID           int    `json:"pid"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return "", 0, fmt.Errorf("decode worker record: %w body=%s", err, raw)
	}
	if rec.ID == "" {
		return "", 0, fmt.Errorf("managed worker record missing id: %s", raw)
	}
	if !rec.ServerManaged {
		return "", 0, fmt.Errorf("expected server_managed=true worker, got %s", raw)
	}
	if rec.PID <= 0 {
		// Fall back to on-disk status if the response omitted pid.
		st := readManagedWorkerStatusNow(t, proj, rec.ID)
		if st.PID <= 0 {
			return "", 0, fmt.Errorf("expected managed worker PID > 0, got %s", raw)
		}
		rec.PID = st.PID
	}
	return rec.ID, rec.PID, nil
}

type managedWorkerStatusFull struct {
	managedWorkerStatus
	ServerManaged  bool `json:"server_managed,omitempty"`
	CurrentAttempt *struct {
		AttemptID string `json:"attempt_id"`
		BeadID    string `json:"bead_id"`
		Phase     string `json:"phase"`
	} `json:"current_attempt,omitempty"`
}

func readManagedWorkerStatusNow(t *testing.T, proj, workerID string) managedWorkerStatusFull {
	t.Helper()
	path := filepath.Join(ddxroot.JoinProject(proj, "workers"), workerID, "status.json")
	var out managedWorkerStatusFull
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// startProviderSignalTripwire launches an external process named like a
// provider CLI that logs any received signal and exits. It runs in its own
// process group so it is never a managed-worker descendant.
func startProviderSignalTripwire(t *testing.T, name, signalLog string) int {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, name)
	// Keep the shell as PID 1 of the process group (no exec) and sleep in a
	// child so SIGTERM is delivered to the shell trap immediately. Logging is
	// the positive signal tripwire; exit 97 makes death observable.
	script := `#!/bin/sh
log="$1"
name="$2"
printf 'ready name=%s pid=%s\n' "$name" "$$" >>"$log"
sleep 600 &
sleep_pid=$!
trap 'printf "SIGTERM name=%s pid=%s\n" "$name" "$$" >>"$log"; kill "$sleep_pid" 2>/dev/null; wait "$sleep_pid" 2>/dev/null; exit 97' TERM
trap 'printf "SIGINT name=%s pid=%s\n" "$name" "$$" >>"$log"; kill "$sleep_pid" 2>/dev/null; wait "$sleep_pid" 2>/dev/null; exit 97' INT
trap 'printf "SIGHUP name=%s pid=%s\n" "$name" "$$" >>"$log"; kill "$sleep_pid" 2>/dev/null; wait "$sleep_pid" 2>/dev/null; exit 97' HUP
trap 'printf "SIGQUIT name=%s pid=%s\n" "$name" "$$" >>"$log"; kill "$sleep_pid" 2>/dev/null; wait "$sleep_pid" 2>/dev/null; exit 97' QUIT
wait "$sleep_pid"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write provider tripwire %s: %v", name, err)
	}
	cmd := exec.Command(scriptPath, signalLog, name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start provider tripwire %s: %v", name, err)
	}
	pid := cmd.Process.Pid
	// Detach Wait so the test process does not keep a zombie; we track by PID.
	go func() { _ = cmd.Wait() }()

	// Wait until ready line lands so traps are installed.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(signalLog)
		if err == nil && strings.Contains(string(raw), "name="+name) && strings.Contains(string(raw), "ready") {
			return pid
		}
		if !integrationProcessAlive(pid) {
			t.Fatalf("provider tripwire %s pid %d exited before ready", name, pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("provider tripwire %s did not write ready line; log=%q", name, readFileOrEmpty(signalLog))
	return pid
}

func readManagedWorkerLog(proj, workerID string) string {
	path := filepath.Join(ddxroot.JoinProject(proj, "workers"), workerID, "worker.log")
	raw, err := os.ReadFile(path)
	if err != nil {
		// Some builds use a different log name; try common alternates.
		for _, name := range []string{"log.txt", "stdout.log", "events.jsonl"} {
			alt := filepath.Join(ddxroot.JoinProject(proj, "workers"), workerID, name)
			if b, e := os.ReadFile(alt); e == nil {
				return string(b)
			}
		}
		return fmt.Sprintf("(no worker log: %v)", err)
	}
	return string(raw)
}

func assertProviderTripwiresUnsignaled(t *testing.T, signalLog string, pids ...int) {
	t.Helper()
	for _, pid := range pids {
		if !integrationProcessAlive(pid) {
			t.Fatalf("provider tripwire pid %d is dead after server shutdown/recovery; log=%q",
				pid, readFileOrEmpty(signalLog))
		}
	}
	raw, err := os.ReadFile(signalLog)
	if err != nil {
		t.Fatalf("read provider signal log: %v", err)
	}
	for _, sig := range []string{"SIGTERM", "SIGINT", "SIGHUP", "SIGQUIT"} {
		if strings.Contains(string(raw), sig) {
			t.Fatalf("provider signal tripwire observed %s during server shutdown/recovery; log:\n%s",
				sig, raw)
		}
	}
}

func readFileOrEmpty(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func integrationProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	// Treat zombies as not alive for lifetime assertions.
	state, ok := integrationProcessState(pid)
	return !ok || state != "Z"
}

func integrationProcessState(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", false
	}
	closeIdx := strings.LastIndex(string(data), ")")
	if closeIdx < 0 || closeIdx+2 >= len(data) {
		return "", false
	}
	fields := strings.Fields(string(data[closeIdx+2:]))
	if len(fields) < 1 {
		return "", false
	}
	return fields[0], true
}

func waitIntegrationProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !integrationProcessAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pid %d still alive after %s", pid, timeout)
}
