package server

import (
	"slices"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

func TestExternalWorkerArgsKeepSelfRefreshEnabledForManagedWatchWorkers(t *testing.T) {
	args := externalWorkerArgs("worker-test", "/tmp/project", ExecuteLoopWorkerSpec{
		Mode: executeloop.ModeWatch,
	})

	for _, want := range []string{"work", "--project", "/tmp/project", "--server-managed-worker-id", "worker-test", "--watch"} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing %q in managed worker args: %v", want, args)
		}
	}
	for _, forbidden := range []string{"--no-self-refresh", "--self-refresh=false"} {
		if slices.Contains(args, forbidden) {
			t.Fatalf("server-managed watch workers must not opt out of self-refresh: %v", args)
		}
	}
}
