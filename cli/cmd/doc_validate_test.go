package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestDocValidateCommand_RejectsDanglingMetricID(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/gate.md", "---\nddx:\n  id: gate.doc\n  execution:\n    kind: command\n    required: true\n    command:\n      - sh\n      - -c\n      - echo ok\n    thresholds:\n      ratchet: 10\n    metric:\n      metric_id: MET-999\n---\n# Gate\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "validate"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	err := root.Execute()
	if err == nil {
		t.Fatal("doc validate must fail on dangling metric_id")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitCodeGeneralError {
		t.Fatalf("expected exit code %d, got %d", ExitCodeGeneralError, exitErr.Code)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "error:") {
		t.Fatalf("expected error output, got %q", outStr)
	}
	if !strings.Contains(outStr, "MET-999") {
		t.Fatalf("expected missing metric id in output, got %q", outStr)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestDocValidateCommand_WarnsOnBudgetRatchetConflict(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "docs/gate.md", "---\nddx:\n  id: gate.doc\n  execution:\n    kind: command\n    required: true\n    command:\n      - sh\n      - -c\n      - echo ok\n    thresholds:\n      ratchet: 250\n    metric:\n      metric_id: MET-001\n---\n# Gate\n")
	writeTestFile(t, dir, "docs/metrics/MET-001.md", "---\nddx:\n  id: MET-001\nmetric:\n  schema_version: 1\n  unit: ms\n  direction: lower-is-better\n  budget: 400\n  source: exec\n  scope: per-attempt\n---\n# Metric\n")

	root := NewCommandFactory(dir).NewRootCommand()
	root.SetArgs([]string{"doc", "validate"})

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)

	if err := root.Execute(); err != nil {
		t.Fatalf("doc validate should succeed on budget/ratchet drift warning, got: %v", err)
	}
	outStr := out.String()
	if !strings.Contains(outStr, "warning:") {
		t.Fatalf("expected warning output, got %q", outStr)
	}
	if !strings.Contains(outStr, "budget") || !strings.Contains(outStr, "ratchet") {
		t.Fatalf("expected budget/ratchet conflict in output, got %q", outStr)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}
