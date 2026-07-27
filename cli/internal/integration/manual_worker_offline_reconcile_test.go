package integration

// manual_worker_offline_reconcile_test.go — ADR-022 rev 6 / TP-021 integration
// proof that a manually started DDx worker survives a real server outage,
// lands offline work under the project cross-process coordination lock, and
// reconciles its offline journal exactly once after reconnect (ddx-205aa254).
//
// TestIntegration_ManualWorkerContinuesOfflineAndReconciles is the wired-in
// end-to-end guard: real TLS server, real git/bead fixture, deterministic
// Fizeau FakeProvider fixture (testseam), real manual `ddx work --watch`
// subprocess, server drop during active work → offline land + single reconcile.

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

// TestIntegration_ManualWorkerContinuesOfflineAndReconciles starts a real
// manual DDx work subprocess against a real TLS server and real git/bead
// fixture, drops the server during active work, proves the manual worker PID
// survives and lands offline under the cross-process lock, then restarts the
// server and proves the offline journal is acknowledged exactly once with no
// duplicate tracker event or landing commit.
func TestIntegration_ManualWorkerContinuesOfflineAndReconciles(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: spawns TLS server plus long-lived manual ddx work subprocess")
	}

	// Build before HOME override so the go toolchain does not re-download the
	// module graph into t.TempDir() (see workerprobe / reachable-server notes).
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

	beadIDs, err := createBeads(bin, proj, subprocessEnv, 1, "Manual offline reconcile bead")
	if err != nil {
		t.Fatalf("seed offline-reconcile bead: %v", err)
	}
	beadID := beadIDs[0]
	if err := gitCommitAll(proj, "test: seed offline-reconcile bead"); err != nil {
		t.Fatalf("commit offline-reconcile bead: %v", err)
	}

	initialTip := gitRunIn(t, proj, "rev-parse", "refs/heads/main")

	// Deterministic Fizeau fixture: long sleep keeps the worker "in active work"
	// so the test can drop the server between claim and land.
	outFile := fmt.Sprintf("offline-%s.txt", beadID)
	commitMsg := "feat: " + beadID + " offline landing"
	subprocessEnv, seamLog := withFizeauTestPlan(t, subprocessEnv, concurrentTryFizeauPlan{
		SleepMS:       5000,
		WritePath:     outFile,
		WriteContent:  "landed-offline-once\n",
		CommitMessage: commitMsg,
	})

	// Real production server + TLS surface with a hard outage switch.
	srv := serverpkg.New(":0", proj)
	t.Cleanup(func() { _ = srv.Shutdown() })
	srv.RegisterProject(proj)

	var (
		serve          atomic.Bool
		mutationPosts  atomic.Int32
		claimOps       atomic.Int32
		landOps        atomic.Int32
		reconcilePosts atomic.Int32
	)
	serve.Store(true)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serve.Load() {
			// Hard transport failure: close the connection so clients treat the
			// server as unreachable (health + mutations + reconcile).
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
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
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/coordination/reconcile") {
			reconcilePosts.Add(1)
		}
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	// Production discovery: manual worker resolves the server via
	// XDG_DATA_HOME/ddx/server.addr (ReadServerAddr). PID is this test process
	// so the address stays live across the TLS outage toggle.
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

	// Manual worker: continuous --watch so the process stays alive past the
	// drain of a single bead and can reconnect after the 30s discover interval.
	manualBuf := &bytes.Buffer{}
	manualCmd := exec.Command(bin, "work", "--watch",
		"--idle-interval", "2s",
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
	manualPID := 0
	if manualCmd.Process != nil {
		manualPID = manualCmd.Process.Pid
	}
	if manualPID <= 0 {
		t.Fatal("manual ddx work started without a PID")
	}
	t.Logf("manual worker pid=%d", manualPID)

	manualDone := make(chan error, 1)
	var manualExited atomic.Bool
	go func() {
		manualDone <- manualCmd.Wait()
		manualExited.Store(true)
	}()
	// Always stop the long-lived --watch worker. Wait is only called once (in
	// the goroutine above); cleanup must not re-Wait or re-receive after the
	// test body has already drained manualDone.
	t.Cleanup(func() {
		if manualExited.Load() {
			return
		}
		if manualCmd.Process != nil {
			_ = manualCmd.Process.Signal(os.Interrupt)
		}
		select {
		case <-manualDone:
			manualExited.Store(true)
		case <-time.After(10 * time.Second):
			if manualCmd.Process != nil {
				_ = manualCmd.Process.Kill()
			}
			select {
			case <-manualDone:
				manualExited.Store(true)
			case <-time.After(5 * time.Second):
				// Process may already be reaped; do not hang the suite.
			}
		}
	})

	// Wait until the worker has claimed (online mutation) so the subsequent
	// server drop falls during active harness work rather than before start.
	claimDeadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(claimDeadline) {
		select {
		case err := <-manualDone:
			manualExited.Store(true)
			t.Fatalf("manual worker exited before claim: err=%v\noutput:\n%s", err, manualBuf.String())
		default:
		}
		if claimOps.Load() >= 1 {
			break
		}
		// Also accept local claim progress if the first mutation was already offline.
		store := bead.NewStore(ddxroot.JoinProject(proj))
		if got, gerr := store.Get(context.Background(), beadID); gerr == nil && got != nil {
			if got.Status == bead.StatusInProgress || got.Status == bead.StatusClosed {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if claimOps.Load() < 1 {
		// Claim may have been fully offline if discovery lagged; still require
		// the worker to be alive and the bead claimed or closed later.
		t.Logf("no online claim mutation before outage (claimOps=%d); continuing with mid-work drop", claimOps.Load())
	}

	// --- AC1: drop the server during active work ---
	serve.Store(false)
	t.Logf("server outage engaged (serve=false) while manual pid=%d is alive", manualPID)

	// --- AC2: PID survives outage; offline land under cross-process lock ---
	select {
	case err := <-manualDone:
		manualExited.Store(true)
		t.Fatalf("manual worker PID did not survive server outage: err=%v\noutput:\n%s", err, manualBuf.String())
	default:
		// still running
	}

	lockPath := agent.OfflineCoordinationLockPath(proj)
	if !strings.Contains(lockPath, offlineCoordinationLockDirLeaf) {
		t.Fatalf("expected offline coordination lock path to use %q, got %q", offlineCoordinationLockDirLeaf, lockPath)
	}

	// Wait for offline landing: journal records (only appended under the
	// cross-process offline coordination lock) and/or the durable work product.
	landDeadline := time.Now().Add(90 * time.Second)
	var (
		sawJournal   bool
		sawLockHeld  bool
		journalCount int
	)
	for time.Now().Before(landDeadline) {
		select {
		case err := <-manualDone:
			manualExited.Store(true)
			// Worker may exit only after drain in non-watch; --watch must stay up.
			t.Fatalf("manual worker exited during offline land wait: err=%v\noutput:\n%s", err, manualBuf.String())
		default:
		}

		// Cross-process lock observation: while offlineMutate runs, the lock dir
		// holds the owner PID. Catching it is best-effort; journal append is
		// the durable proof that WithLock ran.
		if pidData, rerr := os.ReadFile(filepath.Join(lockPath, "pid")); rerr == nil {
			if strings.TrimSpace(string(pidData)) == fmt.Sprintf("%d", manualPID) {
				sawLockHeld = true
			}
		}

		recs, rerr := agent.LoadOfflineJournalRecords(proj)
		if rerr == nil && len(recs) > 0 {
			sawJournal = true
			journalCount = len(recs)
		}

		// Landing effect on main (offline local land path).
		if content, showErr := exec.Command("git", "-C", proj, "show", "HEAD:"+outFile).CombinedOutput(); showErr == nil {
			if strings.Contains(string(content), "landed-offline-once") && sawJournal {
				break
			}
		}

		store := bead.NewStore(ddxroot.JoinProject(proj))
		if got, gerr := store.Get(context.Background(), beadID); gerr == nil && got != nil && got.Status == bead.StatusClosed && sawJournal {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// PID still alive after outage + offline land.
	select {
	case err := <-manualDone:
		manualExited.Store(true)
		t.Fatalf("manual worker died after offline land: err=%v\noutput:\n%s", err, manualBuf.String())
	default:
	}

	if !sawJournal {
		t.Fatalf("expected offline journal records after server outage (cross-process lock + journal path); journal empty\nworker output:\n%s\nlockPath=%s",
			manualBuf.String(), lockPath)
	}
	t.Logf("offline journal has %d record(s); lockObservedWithWorkerPID=%v", journalCount, sawLockHeld)

	// Journal append only happens inside OfflineCoordinator.WithLock; require
	// at least one land (or claim/transition) record for this offline window.
	recs, err := agent.LoadOfflineJournalRecords(proj)
	if err != nil {
		t.Fatalf("load offline journal: %v", err)
	}
	ops := make([]string, 0, len(recs))
	for _, rec := range recs {
		ops = append(ops, fmt.Sprintf("%d:%s", rec.Sequence, rec.Operation))
	}
	t.Logf("offline journal ops: %v", ops)

	// Landing must have completed offline: no online land mutation after outage
	// (landOps may be 0 entirely if claim was online and land was offline).
	if landOps.Load() > 0 {
		// An online land means the outage window missed the land path — fail
		// because this test requires offline landing under the journal lock.
		t.Fatalf("expected land to occur offline (no server land mutations), got landOps=%d", landOps.Load())
	}

	// Work product on main from offline local land.
	content, showErr := exec.Command("git", "-C", proj, "show", "HEAD:"+outFile).CombinedOutput()
	if showErr != nil {
		// Allow a brief settle if close raced ahead of the branch tip update.
		settle := time.Now().Add(15 * time.Second)
		for time.Now().Before(settle) && showErr != nil {
			time.Sleep(200 * time.Millisecond)
			content, showErr = exec.Command("git", "-C", proj, "show", "HEAD:"+outFile).CombinedOutput()
		}
	}
	if showErr != nil {
		t.Fatalf("offline landing effect missing on main (HEAD:%s): %v\n%s\nworker:\n%s",
			outFile, showErr, content, manualBuf.String())
	}
	if !strings.Contains(string(content), "landed-offline-once") {
		t.Fatalf("landing file content unexpected: %q", content)
	}

	// Lock must not remain held after the offline mutation window.
	if _, statErr := os.Stat(lockPath); statErr == nil {
		// Brief grace: a concurrent mutation might still hold it.
		time.Sleep(500 * time.Millisecond)
		if _, statErr = os.Stat(lockPath); statErr == nil {
			t.Fatalf("offline coordination lock still held after land: %s", lockPath)
		}
	}

	pendingBeforeReconnect, err := agent.LoadOfflineJournalPending(proj)
	if err != nil {
		t.Fatalf("load pending journal before reconnect: %v", err)
	}
	if len(pendingBeforeReconnect) == 0 {
		t.Fatal("expected unacknowledged offline journal entries before reconnect")
	}
	pendingSeqs := make([]uint64, 0, len(pendingBeforeReconnect))
	var highestPending uint64
	for _, rec := range pendingBeforeReconnect {
		pendingSeqs = append(pendingSeqs, rec.Sequence)
		if rec.Sequence > highestPending {
			highestPending = rec.Sequence
		}
	}
	t.Logf("pending journal sequences before reconnect: %v", pendingSeqs)

	// --- AC3: reconnect acknowledges journal exactly once; no duplicates ---
	serve.Store(true)
	t.Logf("server restored; waiting for discover+reconcile (default interval 30s)")

	// DefaultCoordinationDiscoverInterval is 30s; allow one full tick + slack.
	reconcileDeadline := time.Now().Add(90 * time.Second)
	var ackedThrough uint64
	for time.Now().Before(reconcileDeadline) {
		select {
		case err := <-manualDone:
			manualExited.Store(true)
			t.Fatalf("manual worker exited before journal reconcile: err=%v\noutput:\n%s", err, manualBuf.String())
		default:
		}
		if reconcilePosts.Load() >= 1 {
			pending, perr := agent.LoadOfflineJournalPending(proj)
			if perr == nil && len(pending) == 0 {
				acked, aerr := agent.LoadOfflineJournalAcknowledgedThrough(proj)
				if aerr == nil && acked > 0 {
					ackedThrough = acked
					break
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	if reconcilePosts.Load() < 1 {
		t.Fatalf("expected exactly one POST /coordination/reconcile after reconnect, got %d\nworker:\n%s",
			reconcilePosts.Load(), manualBuf.String())
	}
	// Discover may re-probe, but empty-pending Reconcile does not re-POST.
	// Allow at most a small number of reconcile POSTs from concurrent probes
	// racing non-empty pending; each must be idempotent. Require the journal
	// to drain and treat more than a few POSTs as a bug.
	if n := reconcilePosts.Load(); n > 3 {
		t.Fatalf("expected reconcile to complete with few POSTs (idempotent once), got %d", n)
	}

	pendingAfter, err := agent.LoadOfflineJournalPending(proj)
	if err != nil {
		t.Fatalf("load pending journal after reconnect: %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Fatalf("expected offline journal fully acknowledged after reconnect, pending=%d ackedThrough=%d",
			len(pendingAfter), ackedThrough)
	}
	if ackedThrough < highestPending {
		// Re-read in case the loop broke on empty pending before capturing ack.
		ackedThrough, err = agent.LoadOfflineJournalAcknowledgedThrough(proj)
		if err != nil {
			t.Fatalf("load ack cursor: %v", err)
		}
	}
	if ackedThrough < highestPending {
		t.Fatalf("acknowledged_through=%d, want >= highest pending sequence %d", ackedThrough, highestPending)
	}
	t.Logf("journal acknowledged_through=%d reconcile_posts=%d", ackedThrough, reconcilePosts.Load())

	// Stop the watch worker now that reconcile is proven.
	if !manualExited.Load() && manualCmd.Process != nil {
		_ = manualCmd.Process.Signal(os.Interrupt)
		select {
		case err := <-manualDone:
			manualExited.Store(true)
			t.Logf("manual worker exit after interrupt: err=%v\noutput:\n%s", err, manualBuf.String())
		case <-time.After(20 * time.Second):
			_ = manualCmd.Process.Kill()
			select {
			case err := <-manualDone:
				manualExited.Store(true)
				t.Logf("manual worker killed after timeout: err=%v\noutput:\n%s", err, manualBuf.String())
			case <-time.After(5 * time.Second):
				t.Logf("manual worker did not report Wait after kill; output:\n%s", manualBuf.String())
			}
		}
	}

	// Bead closed exactly once; single execute-bead success event.
	store := bead.NewStore(ddxroot.JoinProject(proj))
	got, err := store.Get(context.Background(), beadID)
	if err != nil {
		t.Fatalf("get offline-reconcile bead: %v", err)
	}
	if got == nil {
		t.Fatalf("offline-reconcile bead %s missing from store", beadID)
	}
	if got.Status != bead.StatusClosed {
		t.Fatalf("bead status=%q, want closed; notes=%q\nworker:\n%s",
			got.Status, got.Notes, manualBuf.String())
	}

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
		t.Fatalf("expected exactly 1 execute-bead success event, got %d; events=%v",
			successEvents, kinds)
	}

	// Exactly one landing commit for this work product since the pre-outage tip.
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
		t.Fatalf("expected exactly 1 landing commit, got %d\nlanding-grep:\n%s\nfull range:\n%s",
			landingCommits, logOut, fullLog)
	}

	finalTip := gitRunIn(t, proj, "rev-parse", "refs/heads/main")
	if finalTip == initialTip {
		t.Fatal("main tip did not advance; expected offline land to move the branch")
	}

	// Offline work executed exactly once through the Fizeau fixture.
	requireFizeauTestSeamExecutions(t, seamLog, 1)

	// Online land must remain zero (offline path was the sole landing path).
	if landOps.Load() != 0 {
		t.Fatalf("server land mutations must stay 0 after offline land+reconcile, got %d", landOps.Load())
	}
}

// offlineCoordinationLockDirLeaf matches agent.offlineCoordinationLockDirName
// (unexported). Duplicated here so the integration package can assert the
// production lock path without exporting the constant.
const offlineCoordinationLockDirLeaf = "offline-coordination.lock"
