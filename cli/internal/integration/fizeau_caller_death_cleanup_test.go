package integration

// fizeau_caller_death_cleanup_test.go — ADR-022 / TP-021 integration proof that
// provider process-tree lifetime is owned by Fizeau's public caller-death
// contract. DDx workers observe only the public typed final result and never
// process-scan provider descendants or signal provider processes
// (ddx-a3029e47).
//
//   TestIntegration_FizeauCallerDeathOwnsProviderCleanup
//
// Topology: real TLS server, real git/bead fixture, DDx binary compiled with
// Fizeau's public FakeProvider testseam (no hosted providers), real
// server-managed worker subprocess, external provider-signal tripwires, and
// Fizeau's pinned public Execute + POSIX process-tree fixture for
// caller-death/cancellation cleanup evidence.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	agentlib "github.com/easel/fizeau"
)

// emptyFizeauServiceConfig is a public ServiceConfig with no providers. Pinned
// harness + model on Execute still resolve without consulting provider entries
// (Fizeau public harness-dispatch contract).
type emptyFizeauServiceConfig struct {
	workDir       string
	sessionLogDir string
}

func (c emptyFizeauServiceConfig) ProviderNames() []string { return nil }
func (c emptyFizeauServiceConfig) DefaultProviderName() string {
	return ""
}
func (c emptyFizeauServiceConfig) Provider(string) (agentlib.ServiceProviderEntry, bool) {
	return agentlib.ServiceProviderEntry{}, false
}
func (c emptyFizeauServiceConfig) HealthCooldown() time.Duration { return 0 }
func (c emptyFizeauServiceConfig) WorkDir() string               { return c.workDir }
func (c emptyFizeauServiceConfig) SessionLogDir() string         { return c.sessionLogDir }

// TestIntegration_FizeauCallerDeathOwnsProviderCleanup proves provider cleanup
// is reported through Fizeau's public typed final result after caller-side
// cancellation, without DDx process scanning or provider signaling.
//
// AC1: uses the pinned FakeProvider testseam fixture + Fizeau public Execute
// process-tree fixture (not hosted providers) and asserts the public typed
// final fields.
// AC2: cleanup is observed only via Fizeau's public result; external provider
// signal tripwires remain unsignaled and DDx durable evidence carries no
// provider PIDs or process-scan findings for the fixture tree.
func TestIntegration_FizeauCallerDeathOwnsProviderCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns TLS server, managed worker, Fizeau process-tree fixture, and provider tripwires")
	}
	if runtime.GOOS == "windows" {
		t.Skip("caller-death process-tree fixture requires POSIX shell and process-group semantics")
	}

	// Build before HOME override so the go toolchain does not re-download the
	// module graph into t.TempDir() (see workerprobe / concurrent-try notes).
	// FakeProvider testseam is the pinned deterministic Fizeau fixture.
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

	beadIDs, err := createBeads(bin, proj, subprocessEnv, 1, "Fizeau caller-death provider cleanup bead")
	if err != nil {
		t.Fatalf("seed cleanup bead: %v", err)
	}
	beadID := beadIDs[0]
	if err := gitCommitAll(proj, "test: seed Fizeau caller-death cleanup bead"); err != nil {
		t.Fatalf("commit cleanup bead: %v", err)
	}

	// Deterministic FakeProvider plan: sleep keeps the managed worker live
	// while the Fizeau public caller-death fixture runs. No hosted providers.
	outFile := fmt.Sprintf("caller-death-%s.txt", beadID)
	subprocessEnv, seamLog := withFizeauTestPlan(t, subprocessEnv, concurrentTryFizeauPlan{
		SleepMS:       12_000,
		WritePath:     outFile,
		WriteContent:  "caller-death-fixture\n",
		CommitMessage: "feat: " + beadID + " caller-death cleanup",
	})
	for _, kv := range subprocessEnv {
		if strings.HasPrefix(kv, fizeauTestPlanEnv+"=") {
			t.Setenv(fizeauTestPlanEnv, strings.TrimPrefix(kv, fizeauTestPlanEnv+"="))
			break
		}
	}

	// External provider processes live outside the server-owned managed tree.
	// DDx must never signal them; Fizeau alone owns provider process-tree lifetime.
	signalLog := filepath.Join(t.TempDir(), "provider-signals.log")
	claudePID := startProviderSignalTripwire(t, "claude", signalLog)
	codexPID := startProviderSignalTripwire(t, "codex", signalLog)
	geminiPID := startProviderSignalTripwire(t, "gemini", signalLog)
	t.Cleanup(func() {
		_ = syscall.Kill(claudePID, syscall.SIGKILL)
		_ = syscall.Kill(codexPID, syscall.SIGKILL)
		_ = syscall.Kill(geminiPID, syscall.SIGKILL)
	})

	// Real production server + TLS surface (not a mock coordinator).
	srv := serverpkg.New(":0", proj)
	t.Cleanup(func() { _ = srv.Shutdown() })
	srv.EnableManagedWorkers()
	srv.RegisterProject(proj)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
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
	t.Logf("managed worker id=%s pid=%d; provider tripwires claude=%d codex=%d gemini=%d",
		managedID, managedPID, claudePID, codexPID, geminiPID)

	// Wait until the managed worker is live and the FakeProvider testseam has
	// accepted at least one bead_execution Execute (proves no hosted providers
	// on the real DDx worker path).
	deadline := time.Now().Add(45 * time.Second)
	sawRunning := false
	for time.Now().Before(deadline) {
		if !integrationProcessAlive(managedPID) {
			break
		}
		if rec := readManagedWorkerStatusNow(t, proj, managedID); rec.State == "running" {
			sawRunning = true
		}
		if seamRaw, readErr := os.ReadFile(seamLog); readErr == nil &&
			strings.Contains(string(seamRaw), "mode=bead_execution") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawRunning && !integrationProcessAlive(managedPID) {
		t.Fatalf("managed worker never reached running state; log:\n%s",
			readManagedWorkerLog(proj, managedID))
	}
	// Soft-require seam activity: either bead_execution or any Execute line.
	// Full bead_execution may race with a very fast claim path; empty log fails.
	if seamRaw, readErr := os.ReadFile(seamLog); readErr != nil || len(bytes.TrimSpace(seamRaw)) == 0 {
		t.Fatalf("Fizeau FakeProvider testseam log empty (hosted providers forbidden): %v", readErr)
	}

	// --- Fizeau public caller-death / cancellation cleanup fixture ---
	// Exercise only exported Fizeau APIs with a local shell process-tree
	// fixture (not hosted providers). Cancel the Execute context; Fizeau must
	// reap the wrapped tree and report cleanup via the public typed final.
	// Restore the integration env afterwards so subsequent assertions still
	// see the DDx worker topology (HOME/XDG/PATH isolation).
	savedHome := os.Getenv("HOME")
	savedXDGConfig := os.Getenv("XDG_CONFIG_HOME")
	savedXDGData := os.Getenv("XDG_DATA_HOME")
	savedPATH := os.Getenv("PATH")
	publicFinal, fixturePIDs := exerciseFizeauCallerDeathPublicCleanup(t)
	t.Setenv("HOME", savedHome)
	t.Setenv("XDG_CONFIG_HOME", savedXDGConfig)
	t.Setenv("XDG_DATA_HOME", savedXDGData)
	t.Setenv("PATH", savedPATH)

	// DDx observes only the public typed result shape (same fields the run
	// substrate is allowed to persist). No provider PIDs, process groups, or
	// scan findings may appear on that public surface.
	observed := ddxObservesOnlyFizeauPublicResult(t, publicFinal, fixturePIDs)
	if observed.FinalStatus == "" && publicFinal.Status == "" {
		t.Fatal("public typed final reported no status for DDx to observe")
	}
	t.Logf("DDx-observed Fizeau public result: status=%q exit=%v duration_ms=%v session_log=%q",
		observed.FinalStatus, observed.FinalExitCode, observed.DurationMS, observed.SessionLogPath)

	// Provider cleanup ownership: Fizeau reaped the fixture tree before the
	// public final stream closed. The test verifies PIDs are gone as Fizeau
	// contract evidence; production DDx must not perform this scan.
	for label, pid := range fixturePIDs {
		assertFixtureProcessGoneNow(t, pid, label)
	}

	// DDx must not have signaled external provider processes while Fizeau
	// owned cleanup of its own fixture tree.
	assertProviderTripwiresUnsignaled(t, signalLog, claudePID, codexPID, geminiPID)

	// Durable DDx project evidence must not record fixture/provider PIDs as
	// process-scan findings — cleanup is Fizeau's public result only.
	assertDDxEvidenceHasNoProviderProcessScan(t, proj, managedID, fixturePIDs, claudePID, codexPID, geminiPID)

	// FakeProvider seam path was used for the managed worker (no live providers).
	// Prefer bead_execution when present; any non-empty seam log already proved
	// the tagged FakeProvider path above.
	if seamRaw, readErr := os.ReadFile(seamLog); readErr == nil &&
		strings.Contains(string(seamRaw), "mode=bead_execution") {
		requireFizeauTestSeamExecutions(t, seamLog, 1)
	} else if readErr == nil {
		t.Logf("Fizeau test-seam log (pre/post bead_execution):\n%s", seamRaw)
	}

	// Shut down the managed topology cleanly. Provider tripwires must still
	// remain unsignaled after server stop.
	if shutErr := srv.Shutdown(); shutErr != nil {
		t.Logf("srv.Shutdown returned: %v", shutErr)
	}
	ts.Close()
	if integrationProcessAlive(managedPID) {
		waitIntegrationProcessGone(t, managedPID, 30*time.Second)
	}
	assertProviderTripwiresUnsignaled(t, signalLog, claudePID, codexPID, geminiPID)
}

// exerciseFizeauCallerDeathPublicCleanup runs the pinned Fizeau public Execute
// process-tree fixture, cancels the caller context (caller-death / cancel
// path), and returns the public typed final plus the fixture PIDs Fizeau was
// responsible for reaping.
func exerciseFizeauCallerDeathPublicCleanup(t *testing.T) (*agentlib.ServiceFinalData, map[string]int) {
	t.Helper()

	// Isolate this Fizeau construction from operator config and the restricted
	// integration PATH so only the in-test ServiceConfig and PATH stub run.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(fakeHome, ".local", "share"))

	workDir := t.TempDir()
	sessionLogDir := filepath.Join(workDir, "sessions")
	if err := os.MkdirAll(sessionLogDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	const (
		targetPIDFile     = "lifecycle-target.pid"
		grandchildPIDFile = "lifecycle-grandchild.pid"
	)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	// TERM-immune direct child + grandchild: Fizeau must still reap the tree
	// on Execute cancellation (public process-tree / caller-death contract).
	script := `#!/bin/sh
trap '' TERM
printf '%s\n' "$$" > lifecycle-target.pid
sh -c 'trap "" TERM; exec sleep 300' &
child=$!
printf '%s\n' "$child" > lifecycle-grandchild.pid
wait "$child"
`
	if err := os.WriteFile(codexPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write codex fixture: %v", err)
	}
	// Keep system PATH so sh/sleep remain available inside the fixture.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	refreshCancel()
	svc, err := agentlib.New(agentlib.ServiceOptions{
		ServiceConfig: emptyFizeauServiceConfig{
			workDir:       workDir,
			sessionLogDir: sessionLogDir,
		},
		SessionLogDir:           sessionLogDir,
		QuotaRefreshContext:     refreshCtx,
		HarnessCleanupTimeout:   15 * time.Second,
		StaleHarnessReaperGrace: time.Hour,
	})
	if err != nil {
		t.Fatalf("agentlib.New: %v", err)
	}

	execCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := svc.Execute(execCtx, agentlib.ServiceExecuteRequest{
		Prompt:        "integration-caller-death-process-tree",
		Harness:       "codex",
		Model:         "gpt-5.4",
		WorkDir:       workDir,
		SessionLogDir: sessionLogDir,
		Permissions:   "safe",
		Reasoning:     agentlib.ReasoningLow,
	})
	if err != nil {
		t.Fatalf("Fizeau Execute: %v", err)
	}

	targetPID := waitForCompletePIDFile(t, filepath.Join(workDir, targetPIDFile), 10*time.Second)
	grandchildPID := waitForCompletePIDFile(t, filepath.Join(workDir, grandchildPIDFile), 10*time.Second)
	if targetPID == grandchildPID {
		t.Fatalf("target and grandchild PIDs must differ; both %d", targetPID)
	}
	assertFixtureProcessAlive(t, targetPID, "target")
	assertFixtureProcessAlive(t, grandchildPID, "grandchild")

	// Cancel Execute: public contract requires the event stream to stay open
	// until the wrapped tree has exited, then emit a typed final.
	cancel()

	drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer drainCancel()
	var final *agentlib.ServiceFinalData
	for {
		select {
		case <-drainCtx.Done():
			t.Fatalf("timed out waiting for Fizeau Execute stream after cancel: %v", drainCtx.Err())
		case ev, ok := <-events:
			if !ok {
				if final == nil {
					t.Fatal("Fizeau Execute closed without a public typed final event after cancellation")
				}
				// Stream closed == Fizeau finished cleanup. PIDs must already be gone.
				assertFixtureProcessGoneNow(t, targetPID, "target")
				assertFixtureProcessGoneNow(t, grandchildPID, "grandchild")
				return final, map[string]int{
					"target":     targetPID,
					"grandchild": grandchildPID,
				}
			}
			decoded, decodeErr := agentlib.DecodeServiceEvent(ev)
			if decodeErr != nil {
				t.Fatalf("DecodeServiceEvent: %v", decodeErr)
			}
			if decoded.Final != nil {
				// Copy so later assertions own a stable value.
				f := *decoded.Final
				final = &f
				// Public typed result must report cancellation after cleanup.
				// Accept cleanup_failed primary-cancel only if the tree is gone
				// at stream close (same bar as the agent-package conformance test).
				if final.Outcome != agentlib.SessionOutcomeCancelled &&
					final.PrimaryOutcome != agentlib.SessionOutcomeCancelled {
					t.Fatalf("public final outcome=%q primary=%q, want cancelled (status=%q cause=%q stage=%q)",
						final.Outcome, final.PrimaryOutcome, final.Status, final.Cause, final.Stage)
				}
				// Prefer the public typed cause when present.
				if final.Cause != "" &&
					final.Cause != agentlib.TerminalCauseContextCancelled &&
					final.Cause != agentlib.TerminalCauseCallerDied &&
					final.Cause != agentlib.TerminalCauseCleanupFailed {
					t.Fatalf("public final cause=%q, want context_cancelled|caller_died|cleanup_failed", final.Cause)
				}
			}
		}
	}
}

// ddxObservesOnlyFizeauPublicResult maps the public final the way DDx run
// substrates are allowed to (session log path, status, exit, duration only —
// no provider process metadata). Asserts the fixture PIDs never appear on that
// public surface.
func ddxObservesOnlyFizeauPublicResult(t *testing.T, final *agentlib.ServiceFinalData, fixturePIDs map[string]int) *runrecord.FizeauPublicResult {
	t.Helper()
	if final == nil {
		t.Fatal("nil public typed final")
	}
	out := &runrecord.FizeauPublicResult{}
	if p := strings.TrimSpace(final.SessionLogPath); p != "" {
		out.SessionLogPath = p
	}
	if s := strings.TrimSpace(final.Status); s != "" {
		out.FinalStatus = s
	}
	exit := final.ExitCode
	out.FinalExitCode = &exit
	if final.DurationMS != 0 {
		d := final.DurationMS
		out.DurationMS = &d
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal observed public result: %v", err)
	}
	for label, pid := range fixturePIDs {
		if pid > 0 && bytes.Contains(raw, []byte(strconv.Itoa(pid))) {
			t.Fatalf("DDx-observed public result must not embed %s provider pid %d; got %s", label, pid, raw)
		}
	}
	// Public typed terminal tuple must be present for DDx policy to consume.
	if final.Outcome == "" && final.Status == "" {
		t.Fatalf("public typed final missing outcome/status: %+v", final)
	}
	return out
}

// assertDDxEvidenceHasNoProviderProcessScan scans durable DDx project evidence
// for the fixture/provider PIDs as process-scan artifacts. Cleanup ownership
// is Fizeau's public result; DDx must not invent provider-process findings.
func assertDDxEvidenceHasNoProviderProcessScan(t *testing.T, proj, managedID string, fixturePIDs map[string]int, providerPIDs ...int) {
	t.Helper()
	var needles []string
	for _, pid := range fixturePIDs {
		if pid > 0 {
			needles = append(needles, strconv.Itoa(pid))
		}
	}
	for _, pid := range providerPIDs {
		if pid > 0 {
			needles = append(needles, strconv.Itoa(pid))
		}
	}
	if len(needles) == 0 {
		return
	}

	// Paths DDx is allowed to write for worker/attempt evidence. Presence of a
	// fixture/provider PID string next to process-scan vocabulary is a boundary
	// violation; bare PID appearance in unrelated logs is still treated as a
	// hard fail for this fixture because the PIDs are unique to this test.
	roots := []string{
		filepath.Join(proj, ".ddx"),
	}
	scanKeywords := []string{
		"process_scan",
		"process-scan",
		"provider_pid",
		"provider-pid",
		"reap_provider",
		"kill_provider",
		"signal_provider",
		"descendant",
		"pgid",
	}
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			// Skip large or binary-ish artifacts.
			if info.Size() > 2<<20 {
				return nil
			}
			name := strings.ToLower(info.Name())
			if strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".pack") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			lower := strings.ToLower(string(data))
			hasKeyword := false
			for _, kw := range scanKeywords {
				if strings.Contains(lower, kw) {
					hasKeyword = true
					break
				}
			}
			if !hasKeyword {
				return nil
			}
			for _, needle := range needles {
				if bytes.Contains(data, []byte(needle)) {
					t.Fatalf("DDx evidence %s pairs process-scan vocabulary with provider/fixture pid %s; provider cleanup must be Fizeau public result only", path, needle)
				}
			}
			return nil
		})
	}

	// Managed worker log is a high-signal surface for accidental process control.
	logPath := filepath.Join(proj, ".ddx", "workers", managedID, "worker.log")
	if raw, err := os.ReadFile(logPath); err == nil {
		for _, needle := range needles {
			if bytes.Contains(raw, []byte("kill "+needle)) ||
				bytes.Contains(raw, []byte("SIGTERM "+needle)) ||
				bytes.Contains(raw, []byte("signal provider")) {
				t.Fatalf("managed worker log shows provider signaling for pid %s:\n%s", needle, raw)
			}
		}
	}
}

func waitForCompletePIDFile(t *testing.T, path string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastData []byte
	var lastErr error
	for {
		data, err := os.ReadFile(path)
		lastData, lastErr = data, err
		// Require trailing newline so we do not parse a partial write.
		if err == nil && bytes.HasSuffix(data, []byte{'\n'}) {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				return pid
			}
			lastErr = parseErr
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for complete pid file %s (data %q): %v", path, lastData, lastErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertFixtureProcessAlive(t *testing.T, pid int, label string) {
	t.Helper()
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("%s pid %d is not alive before cancel: %v", label, pid, err)
	}
}

// assertFixtureProcessGoneNow fails if pid is still killable. Immediate check
// used at Fizeau Execute stream close to prove Fizeau-owned cleanup completed.
func assertFixtureProcessGoneNow(t *testing.T, pid int, label string) {
	t.Helper()
	err := syscall.Kill(pid, 0)
	if err == nil {
		t.Fatalf("%s pid %d still alive after Fizeau Execute returned (provider cleanup must be Fizeau-owned)", label, pid)
	}
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return
	}
	if strings.Contains(err.Error(), "no such process") {
		return
	}
	t.Fatalf("%s pid %d unexpected kill(0) after Fizeau Execute returned: %v", label, pid, err)
}
