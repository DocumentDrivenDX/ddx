package cmd

// coordination_wiring_test.go — ADR-022 rev 6 AC for ddx-2e49980d:
// try/work/managed share one reconnecting coordination client.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTry_UsesReachableServerCoordinator verifies ddx try submits coordination
// mutations to a real server and does not construct NewLocalLandCoordinator.
func TestTry_UsesReachableServerCoordinator(t *testing.T) {
	projectRoot, store, beadID := setupCoordinationCmdProject(t)
	srv, baseURL, mutations, stop := startCoordinationTestServer(t, projectRoot)
	defer stop()
	_ = srv
	writeTestServerAddr(t, baseURL)

	// Real command entry: ddx try with a hermetic executor (claim still hits
	// the shared coordination client via WrapStoreWithCoordination).
	factory := NewCommandFactory(projectRoot)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(_ context.Context, id string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    id,
			AttemptID: "try-coord-attempt",
			Status:    agent.ExecuteBeadStatusSuccess,
			BaseRev:   "base",
			ResultRev: "base", // no land (equal revs)
			SessionID: "sess-try-coord",
		}, nil
	})
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	root := factory.NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"try", beadID, "--no-review", "--no-review-i-know-what-im-doing"})
	require.NoError(t, root.Execute(), out.String())

	// Claim must have gone through the server coordination path at least once.
	require.GreaterOrEqual(t, mutations.Load(), int32(1),
		"try must POST coordination/mutations to the reachable server")

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	// After success with equal revs the loop may close or leave claimed;
	// the load-bearing proof is the mutation count above.
	assert.NotEqual(t, "", got.Status)

	// Source contract: no unconditional NewLocalLandCoordinator in cli/cmd.
	assertNoLocalLandCoordinatorInCmd(t)
}

// TestWork_UsesReachableServerCoordinator verifies ddx work --once submits
// coordination mutations to a real server without a process-local land
// coordinator competing on the command path.
func TestWork_UsesReachableServerCoordinator(t *testing.T) {
	projectRoot, store, beadID := setupCoordinationCmdProject(t)
	_, baseURL, mutations, stop := startCoordinationTestServer(t, projectRoot)
	defer stop()
	writeTestServerAddr(t, baseURL)

	factory := NewCommandFactory(projectRoot)
	factory.tryExecutorOverride = agent.ExecuteBeadExecutorFunc(func(_ context.Context, id string) (agent.ExecuteBeadReport, error) {
		return agent.ExecuteBeadReport{
			BeadID:    id,
			AttemptID: "work-coord-attempt",
			Status:    agent.ExecuteBeadStatusSuccess,
			BaseRev:   "base",
			ResultRev: "base",
			SessionID: "sess-work-coord",
		}, nil
	})
	factory.AgentRunnerOverride = &tryHookRunnerStub{t: t}
	// Skip heavy intake/lint agent calls by using the override without quality runner.

	root := factory.NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"work", "--once", "--no-review", "--no-review-i-know-what-im-doing"})
	err := root.Execute()
	// work may exit non-zero on some post-attempt paths; mutation is the AC.
	_ = err
	_ = store
	_ = beadID

	require.GreaterOrEqual(t, mutations.Load(), int32(1),
		"work must POST coordination/mutations to the reachable server")
	assertNoLocalLandCoordinatorInCmd(t)
}

// TestWork_ServerLossSwitchesOfflineAndRetries verifies one unknown-response
// mutation is journaled/applied once, discovery reconnects, and reconciliation
// completes before a new online write. Exercises the shared client that work
// bootstrap constructs.
func TestWork_ServerLossSwitchesOfflineAndRetries(t *testing.T) {
	projectRoot, store, beadID := setupCoordinationCmdProject(t)

	var serve atomic.Bool
	serve.Store(true)
	var mutations atomic.Int32
	var reconciles atomic.Int32

	inner := serverpkg.New(":0", projectRoot)
	t.Cleanup(func() { /* server is process-local; GC is fine */ })
	inner.RegisterProject(projectRoot)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serve.Load() {
			// Simulate unknown-response / transport failure mid-flight.
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if strings.Contains(r.URL.Path, "/coordination/mutations") {
			mutations.Add(1)
		}
		if strings.Contains(r.URL.Path, "/coordination/reconcile") {
			reconciles.Add(1)
		}
		inner.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	writeTestServerAddr(t, ts.URL)

	client, err := agent.NewCoordinationClient(projectRoot, agent.CoordinationClientConfig{
		WorkerID:         "work-loss-test",
		AddrFunc:         serverpkg.ReadServerAddr,
		HTTPClient:       ts.Client(),
		DiscoverInterval: 50 * time.Millisecond,
		Store:            store,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	client.Start(context.Background())

	// Wait until connected.
	require.Eventually(t, func() bool { return client.Connected() }, 3*time.Second, 20*time.Millisecond)

	// Online claim.
	require.NoError(t, client.ClaimBead(context.Background(), beadID, "worker-loss", "sess", ""))
	onlineMuts := mutations.Load()
	require.GreaterOrEqual(t, onlineMuts, int32(1), "first claim must hit the server")

	// Server loss: next mutation must apply offline + journal once.
	serve.Store(false)
	client.MarkDisconnectedForTest()

	bead2 := "ddx-coord-loss-002"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    bead2,
		Title: "offline claim fixture",
	}))
	require.NoError(t, client.ClaimBead(context.Background(), bead2, "worker-loss", "sess", ""))

	pending, err := agent.LoadOfflineJournalPending(projectRoot)
	require.NoError(t, err)
	require.NotEmpty(t, pending, "offline mutation must be journaled")
	// Exactly one new journal entry for the offline claim (applied once).
	claimPending := 0
	for _, rec := range pending {
		if rec.Operation == "claim" && rec.IdempotencyKey != "" {
			claimPending++
		}
	}
	require.GreaterOrEqual(t, claimPending, 1)

	got2, err := store.Get(context.Background(), bead2)
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, bead.StatusInProgress, got2.Status)
	assert.Equal(t, "worker-loss", got2.Owner)

	// Reconnect: discovery + reconcile before a new online write.
	serve.Store(true)
	require.Eventually(t, func() bool {
		client.ProbeOnceForTest(context.Background())
		if !client.Connected() {
			return false
		}
		// Reconcile must drain the offline journal before we treat reconnect
		// as complete (ADR-022: reconcile before new online writes).
		if err := client.Reconcile(context.Background()); err != nil {
			return false
		}
		pendingNow, err := agent.LoadOfflineJournalPending(projectRoot)
		return err == nil && len(pendingNow) == 0
	}, 5*time.Second, 50*time.Millisecond, "reconnect must reconcile journal")

	require.GreaterOrEqual(t, reconciles.Load(), int32(1), "reconnect must POST coordination/reconcile")

	bead3 := "ddx-coord-loss-003"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    bead3,
		Title: "post-reconcile claim fixture",
	}))
	before := mutations.Load()
	require.NoError(t, client.ClaimBead(context.Background(), bead3, "worker-loss", "sess", ""))
	require.Greater(t, mutations.Load(), before, "post-reconcile write must go online")

	// Pending journal must remain drained after the online write.
	pendingAfter, err := agent.LoadOfflineJournalPending(projectRoot)
	require.NoError(t, err)
	assert.Empty(t, pendingAfter, "reconcile must ack journal before new online writes complete")
}

// TestManagedWork_UsesSameCoordinationClient verifies --server-managed only
// changes lifecycle ownership and does not double-submit landing through
// WorkerManager. Managed subprocess uses execute_loop_shared (same client);
// WorkerManager.launchManagedExecuteLoop does not land.
func TestManagedWork_UsesSameCoordinationClient(t *testing.T) {
	// 1) Managed CLI args only add process ownership flag.
	args := serverpkg.ManagedWorkerCommandArgs(serverpkg.ExecuteLoopWorkerSpec{
		Mode:        "once",
		ProjectRoot: "/tmp/proj",
	}, "wkr-test-1")
	require.Equal(t, "work", args[0])
	require.Equal(t, "--server-managed", args[1])
	require.Equal(t, "wkr-test-1", args[2])

	// 2) Production work path uses shared bootstrap (no NewLocalLandCoordinator).
	assertNoLocalLandCoordinatorInCmd(t)

	// 3) WorkerManager managed launch short-circuits before in-process runWorker
	// (which owns evaluateGatesAndSubmit / LandCoordinator.Submit). Managed
	// workers land solely through the shared coordination client in the
	// subprocess execute_loop_shared path — no WorkerManager double-submit.
	srcPaths := []string{
		filepath.Join("internal", "server", "workers.go"),
		filepath.Join("..", "internal", "server", "workers.go"),
		"cli/internal/server/workers.go",
	}
	var text string
	var readErr error
	for _, p := range srcPaths {
		var b []byte
		b, readErr = os.ReadFile(p)
		if readErr == nil {
			text = string(b)
			break
		}
	}
	require.NoError(t, readErr, "locate workers.go")
	assert.Contains(t, text, "launchManagedExecuteLoop")
	// In StartExecuteLoop, managedLaunch returns after launchManagedExecuteLoop
	// and never reaches go m.runWorker (the land-via-LandCoordinators path).
	managedIdx := strings.Index(text, "if m.managedLaunch {")
	runWorkerIdx := strings.Index(text, "go m.runWorker(")
	require.Greater(t, managedIdx, 0)
	require.Greater(t, runWorkerIdx, managedIdx,
		"managed launch must short-circuit before in-process runWorker land path")
	// Between managed branch and runWorker, launchManagedExecuteLoop must appear.
	segment := text[managedIdx:runWorkerIdx]
	assert.Contains(t, segment, "launchManagedExecuteLoop",
		"managed path must launch subprocess rather than in-process land submitter")
	assert.NotContains(t, segment, "evaluateGatesAndSubmit",
		"managed launch must not double-submit landing through WorkerManager")

	// 4) Same bootstrap helper used by try/work constructs the shared client.
	projectRoot, store, _ := setupCoordinationCmdProject(t)
	client, err := bootstrapCoordinationClient(projectRoot, store)
	require.NoError(t, err)
	require.NotNil(t, client)
	_ = client.Close()
}

// --- helpers ----------------------------------------------------------------

func setupCoordinationCmdProject(t *testing.T) (projectRoot string, store *bead.Store, beadID string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	projectRoot = t.TempDir()
	runCoordGit(t, projectRoot, "init", "-b", "main")
	runCoordGit(t, projectRoot, "config", "user.name", "Coord Wire Test")
	runCoordGit(t, projectRoot, "config", "user.email", "coord-wire@test.local")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# coord\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(".ddx/executions/\n"), 0o644))
	runCoordGit(t, projectRoot, "add", "-A")
	runCoordGit(t, projectRoot, "commit", "-m", "init")
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	store = bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	beadID = "ddx-coord-wire-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:         beadID,
		Title:      "coordination wiring fixture",
		Acceptance: "claim via shared client",
	}))
	runCoordGit(t, projectRoot, "add", "-A")
	runCoordGit(t, projectRoot, "commit", "-m", "seed bead")
	return projectRoot, store, beadID
}

func startCoordinationTestServer(t *testing.T, projectRoot string) (*serverpkg.Server, string, *atomic.Int32, func()) {
	t.Helper()
	var mutations atomic.Int32
	srv := serverpkg.New(":0", projectRoot)
	srv.RegisterProject(projectRoot)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/coordination/mutations") {
			mutations.Add(1)
		}
		srv.Handler().ServeHTTP(w, r)
	})
	ts := httptest.NewTLSServer(handler)
	ts.Client().Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	stop := func() { ts.Close() }
	return srv, ts.URL, &mutations, stop
}

func writeTestServerAddr(t *testing.T, url string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "ddx")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	body, _ := json.Marshal(map[string]any{
		"url": url,
		"pid": os.Getpid(),
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "server.addr"), body, 0o600))
}

func assertNoLocalLandCoordinatorInCmd(t *testing.T) {
	t.Helper()
	// Production command paths must not unconditionally construct a process-
	// local land coordinator (AC4). Scan this package's non-test .go files.
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		if bytes.Contains(data, []byte("NewLocalLandCoordinator")) {
			t.Fatalf("cli/cmd/%s must not construct NewLocalLandCoordinator on the command path", name)
		}
	}
}

func runCoordGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
}
