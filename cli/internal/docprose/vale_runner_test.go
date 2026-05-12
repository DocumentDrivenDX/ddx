package docprose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValeRunner_InvokesWithConfigJSONNoGlobal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses sh")
	}

	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "vale-args.txt")
	scriptPath := filepath.Join(binDir, "vale")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'vale version 3.13.0'
  exit 0
fi
printf '%s\n' "$@" > "$VALE_CAPTURE_FILE"
printf '%s\n' '{"\/work\/docs\/example.md":[{"Check":"DDx.Test","Line":3,"Span":[4,11],"Severity":"warning","Message":"test message","Match":"example"}]}'
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake vale: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("VALE_CAPTURE_FILE", capturePath)

	runner := NewValeRunner()
	alerts, err := runner.Findings(context.Background(), settings, "docs/example.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %+v", alerts)
	}

	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read capture file: %v", err)
	}
	gotArgs := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(gotArgs) != 5 {
		t.Fatalf("unexpected args count: got %v", gotArgs)
	}
	wantFixed := []string{"--config", "--output=JSON", "--no-global", "docs/example.md"}
	for idx, want := range wantFixed {
		gotIdx := idx
		if idx >= 1 {
			gotIdx++
		}
		if gotArgs[gotIdx] != want {
			t.Fatalf("arg %d = %q, want %q (full args: %v)", gotIdx, gotArgs[gotIdx], want, gotArgs)
		}
	}
	if !strings.Contains(gotArgs[1], "ddx-vale-") {
		t.Fatalf("expected generated temp config path, got %q", gotArgs[1])
	}
	if !strings.HasSuffix(gotArgs[1], ".vale.ini") {
		t.Fatalf("expected generated config to be .vale.ini, got %q", gotArgs[1])
	}
}

func TestValeRunner_ParsesJSONFindings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses sh")
	}

	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "vale")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'vale version 3.13.0'
  exit 0
fi
printf '%s\n' '{"docs/guide.md":[{"Check":"DDx.UnsupportedClaim","Line":7,"Span":[12,19],"Severity":"warning","Message":"avoid broad claims","Match":"world-class"}]}'
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake vale: %v", err)
	}
	t.Setenv("PATH", binDir)

	runner := NewValeRunner()
	alerts, err := runner.Findings(context.Background(), settings, "docs/guide.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %+v", alerts)
	}

	got := alerts[0]
	if got.File != "docs/guide.md" {
		t.Fatalf("File = %q, want docs/guide.md", got.File)
	}
	if got.Line != 7 {
		t.Fatalf("Line = %d, want 7", got.Line)
	}
	if got.Span != [2]int{12, 19} {
		t.Fatalf("Span = %v, want [12 19]", got.Span)
	}
	if got.Check != "DDx.UnsupportedClaim" {
		t.Fatalf("Check = %q, want DDx.UnsupportedClaim", got.Check)
	}
	if got.Severity != "warning" {
		t.Fatalf("Severity = %q, want warning", got.Severity)
	}
	if got.Message != "avoid broad claims" {
		t.Fatalf("Message = %q, want avoid broad claims", got.Message)
	}
	if got.Match != "world-class" {
		t.Fatalf("Match = %q, want world-class", got.Match)
	}
}

func TestValeRunner_MissingValeDiagnostic(t *testing.T) {
	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", t.TempDir())

	runner := NewValeRunner()
	_, err = runner.Findings(context.Background(), settings, "docs/guide.md")
	if err == nil {
		t.Fatal("expected missing Vale diagnostic")
	}

	var diag *ValeDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *ValeDiagnosticError, got %T: %v", err, err)
	}
	if diag.Kind != ValeDiagnosticMissing {
		t.Fatalf("Kind = %q, want %q", diag.Kind, ValeDiagnosticMissing)
	}
	if !strings.Contains(diag.Error(), "not installed") {
		t.Fatalf("unexpected diagnostic message: %q", diag.Error())
	}
}

func TestValeRunner_UnsupportedValeDiagnostic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses sh")
	}

	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "vale")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'vale version 3.12.0'
  exit 0
fi
printf 'should not run linting when version is unsupported\n' >&2
exit 2
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake vale: %v", err)
	}
	t.Setenv("PATH", binDir)

	runner := NewValeRunner()
	_, err = runner.Findings(context.Background(), settings, "docs/guide.md")
	if err == nil {
		t.Fatal("expected unsupported Vale diagnostic")
	}

	var diag *ValeDiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *ValeDiagnosticError, got %T: %v", err, err)
	}
	if diag.Kind != ValeDiagnosticUnsupported {
		t.Fatalf("Kind = %q, want %q", diag.Kind, ValeDiagnosticUnsupported)
	}
	if !strings.Contains(diag.Error(), SupportedValeVersion) {
		t.Fatalf("diagnostic message does not mention supported version %q: %q", SupportedValeVersion, diag.Error())
	}
	if !strings.Contains(diag.Error(), "3.12.0") {
		t.Fatalf("diagnostic message does not mention observed version: %q", diag.Error())
	}
}
