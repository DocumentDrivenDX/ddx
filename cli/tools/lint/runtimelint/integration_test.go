package runtimelint_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRuntimelintCleanTree runs the runtimelint binary against the
// post-cleanup project tree and asserts zero violations. This is the
// SD-024 Stage 4 final-sweep gate: any drift that reintroduces a
// forbidden durable-knob field on a *Runtime struct or revives a
// retired *Options type will fail this test.
func TestRuntimelintCleanTree(t *testing.T) {
	// Resolve the cli/ module root from this test file's package
	// directory (cli/tools/lint/runtimelint).
	moduleRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cmd := exec.Command("go", "run", "./tools/lint/runtimelint/cmd/runtimelint", "./...")
	cmd.Dir = moduleRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		t.Fatalf("runtimelint exited non-zero: %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("runtimelint produced diagnostics on a clean tree:\n%s", stdout.String())
	}
}
