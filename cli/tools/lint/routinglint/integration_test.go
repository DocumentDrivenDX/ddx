package routinglint_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRoutingLint_RunPathNeverSetsProfileDirectly runs the routinglint
// binary against the live tree and asserts zero violations. This is the
// integration gate for the FEAT-006 routing-cleanup invariant: the
// run/try/work dispatch paths must not re-introduce compensating-routing
// identifiers or retired flag/config-key literals. runtimelint enforces
// that *Runtime structs in cli/internal/agent/ never declare Profile,
// Harness, or Model as durable-knob fields; this test enforces that the
// broader cli/ tree stays free of the retired routing symbols those rules
// depend on being absent.
func TestRoutingLint_RunPathNeverSetsProfileDirectly(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cmd := exec.Command("go", "run", "-buildvcs=false", "./tools/lint/routinglint/cmd/routinglint", "./...")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		t.Fatalf("routinglint exited non-zero: %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("routinglint produced diagnostics on a clean tree:\n%s", stdout.String())
	}
}
