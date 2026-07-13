//go:build !windows

package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

// ptyProbeStubService is a FizeauService stub whose ListModels delegates to a
// caller-supplied closure so tests can simulate a PTY-driven subprocess model
// probe (a `codex --no-alt-screen` style child) that outlives the fizeau
// ListModels call itself.
type ptyProbeStubService struct {
	stubAgentService
	listModelsFn func(ctx context.Context) ([]agentlib.ModelInfo, error)
}

func (s ptyProbeStubService) ListModels(ctx context.Context, _ agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return s.listModelsFn(ctx)
}

// startFakeProviderProcess launches a real child process (a symlink to
// `sleep` named after a provider CLI, e.g. "codex") in its own process group
// so tests can exercise real process-tree scanning and process-group
// termination, mirroring internal/agent's provider-child reap tests.
func startFakeProviderProcess(t *testing.T, dir, provider string) int {
	t.Helper()
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	bin := filepath.Join(dir, provider)
	if err := os.Symlink(sleepPath, bin); err != nil {
		t.Fatalf("symlink fake %s: %v", provider, err)
	}
	cmd := exec.Command(bin, "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake %s: %v", provider, err)
	}
	pid := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	})
	waitForProcessObservable(t, pid)
	return pid
}

// waitForProcessObservable blocks until pid is visible to syscall-level
// liveness checks, guarding against the (rare) window between cmd.Start()
// returning and the child's PID becoming queryable.
func waitForProcessObservable(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if processAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake provider process pid %d never became observable", pid)
}

func assertProcessReaped(t *testing.T, pid int) {
	t.Helper()
	if processAlive(pid) {
		t.Fatalf("provider process pid %d still alive after ListModelsWithProbeContainment returned", pid)
	}
}

// TestLifecycleModelDiscovery_CancellationReapsPTYProbe proves that when the
// caller's context is cancelled mid-call, ListModelsWithProbeContainment does
// not return until the spawned PTY probe (and its process group) has actually
// exited (cli/cmd/execute_loop_shared.go's 2s model-discovery preflight call
// site, ddx-93d8b7c8).
func TestLifecycleModelDiscovery_CancellationReapsPTYProbe(t *testing.T) {
	dir := t.TempDir()
	var probePID int

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := ptyProbeStubService{
		listModelsFn: func(ctx context.Context) ([]agentlib.ModelInfo, error) {
			probePID = startFakeProviderProcess(t, dir, "codex")
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := agent.ListModelsWithProbeContainment(ctx, svc, agentlib.ModelFilter{})
	if err == nil {
		t.Fatalf("expected context cancellation error from ListModelsWithProbeContainment")
	}
	if probePID == 0 {
		t.Fatalf("stub never recorded a probe pid")
	}
	assertProcessReaped(t, probePID)
}

// TestLifecycleModelDiscovery_ReturnDoesNotLeaveNoAltScreenChild covers
// success, upstream error, caller timeout, and worker cancellation outcomes
// for the model-discovery preflight call site, proving that in every case the
// PTY probe spawned during the call is reaped by the time
// ListModelsWithProbeContainment returns while a provider process that
// predates the call (the pre-call baseline) is left untouched.
func TestLifecycleModelDiscovery_ReturnDoesNotLeaveNoAltScreenChild(t *testing.T) {
	scenarios := []struct {
		name       string
		buildCtx   func() (context.Context, context.CancelFunc)
		listModels func(ctx context.Context, dir string, probePID *int) ([]agentlib.ModelInfo, error)
	}{
		{
			name: "success",
			buildCtx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			listModels: func(ctx context.Context, dir string, probePID *int) ([]agentlib.ModelInfo, error) {
				*probePID = startFakeProviderProcess(t, dir, "codex")
				return []agentlib.ModelInfo{{ID: "gpt-test"}}, nil
			},
		},
		{
			name: "upstream_error",
			buildCtx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			listModels: func(ctx context.Context, dir string, probePID *int) ([]agentlib.ModelInfo, error) {
				*probePID = startFakeProviderProcess(t, dir, "codex")
				return nil, errors.New("upstream discovery failure")
			},
		},
		{
			name: "caller_timeout",
			buildCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 30*time.Millisecond)
			},
			listModels: func(ctx context.Context, dir string, probePID *int) ([]agentlib.ModelInfo, error) {
				*probePID = startFakeProviderProcess(t, dir, "codex")
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			name: "worker_cancellation",
			buildCtx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			listModels: func(ctx context.Context, dir string, probePID *int) ([]agentlib.ModelInfo, error) {
				*probePID = startFakeProviderProcess(t, dir, "codex")
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			dir := t.TempDir()
			// A provider process that predates this call — e.g. another
			// attempt's live route, or a manual session — must survive
			// containment untouched (NON-SCOPE: never kill processes that
			// predate the call).
			baselinePID := startFakeProviderProcess(t, dir, "claude")

			var probePID int
			ctx, cancel := sc.buildCtx()
			defer cancel()

			svc := ptyProbeStubService{
				listModelsFn: func(ctx context.Context) ([]agentlib.ModelInfo, error) {
					return sc.listModels(ctx, dir, &probePID)
				},
			}

			if sc.name == "worker_cancellation" {
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()
			}

			_, _ = agent.ListModelsWithProbeContainment(ctx, svc, agentlib.ModelFilter{})

			if probePID == 0 {
				t.Fatalf("scenario %s: probe pid never recorded", sc.name)
			}
			assertProcessReaped(t, probePID)
			if !processAlive(baselinePID) {
				t.Fatalf("scenario %s: pre-call baseline pid %d was reaped", sc.name, baselinePID)
			}
		})
	}
}
