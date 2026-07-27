package docprose

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestValeStylePackProducesFindingsOnKnownViolation exercises the packaged DDx
// Vale style pack (library/checks/prose-quality/styles/DDx) as shipped, not a
// hand-built inline rule. It fails if the pack is inert (e.g. every rule still
// carries scope: summary and matches nothing in Markdown).
func TestValeStylePackProducesFindingsOnKnownViolation(t *testing.T) {
	// Static guard: packaged rules must not use HTML-only `summary` scope.
	// That scope never matches DDx's Markdown corpus and silently zeros findings.
	assertPackagedRulesHaveNoSummaryScope(t)

	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed on PATH; install vale to exercise the packaged style pack")
	}

	settings, err := DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := NewTempValeConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Cleanup()

	// Confirm the temp config points at the packaged pack (symlink), not a copy
	// or hand-built rules that could drift from what we ship.
	stylesDir := filepath.Join(filepath.Dir(cfg.INIPath()), "styles")
	packLink := filepath.Join(stylesDir, "DDx")
	target, err := os.Readlink(packLink)
	if err != nil {
		t.Fatalf("expected packaged DDx styles symlink at %s: %v", packLink, err)
	}
	assetRoot, err := defaultAssetRoot()
	if err != nil {
		t.Fatal(err)
	}
	wantPack := filepath.Join(assetRoot, "styles", "DDx")
	if filepath.Clean(target) != filepath.Clean(wantPack) {
		t.Fatalf("StylesPath DDx link = %q, want packaged pack %q", target, wantPack)
	}

	// Fixture with a known DDx.UnsupportedClaim token ("world-class").
	workDir := t.TempDir()
	fixturePath := filepath.Join(workDir, "known-violation.md")
	const fixtureBody = "This implementation delivers world-class reliability without evidence.\n"
	if err := os.WriteFile(fixturePath, []byte(fixtureBody), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Invoke vale directly against the generated config so this regression
	// tests pack matching independent of the runner's version pin.
	cmd := exec.CommandContext(context.Background(), "vale",
		"--config", cfg.INIPath(),
		"--output=JSON",
		"--no-global",
		fixturePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if stderr.Len() > 0 && strings.Contains(stderr.String(), "E100") {
		t.Fatalf("vale failed with E100 (config/vocab layout?): %s", stderr.String())
	}

	alerts, parseErr := parseValeAlertsJSON(stdout.Bytes())
	if parseErr != nil {
		t.Fatalf("parse vale JSON: %v\nstdout=%s\nstderr=%s", parseErr, stdout.String(), stderr.String())
	}
	// Vale exits 1 when it reports alerts; exit 0 with empty JSON is the inert-pack failure mode.
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("vale run failed: %v\nstderr=%s\nstdout=%s", runErr, stderr.String(), stdout.String())
		}
	}

	if len(alerts) == 0 {
		t.Fatalf("packaged DDx style pack produced zero findings on a known unsupported claim; pack is inert (check rule scopes)\nstdout=%s\nstderr=%s",
			stdout.String(), stderr.String())
	}

	var foundUnsupported bool
	for _, a := range alerts {
		if a.Check == "DDx.UnsupportedClaim" {
			foundUnsupported = true
			if !strings.Contains(strings.ToLower(a.Match), "world-class") &&
				!strings.Contains(strings.ToLower(fixtureBody), strings.ToLower(a.Match)) {
				// Match should come from the fixture; world-class is the intended token.
				t.Logf("alert Match=%q Check=%q Message=%q", a.Match, a.Check, a.Message)
			}
			break
		}
	}
	if !foundUnsupported {
		checks := make([]string, 0, len(alerts))
		for _, a := range alerts {
			checks = append(checks, a.Check+":"+a.Match)
		}
		t.Fatalf("expected at least one DDx.UnsupportedClaim finding, got: %v", checks)
	}
}

func assertPackagedRulesHaveNoSummaryScope(t *testing.T) {
	t.Helper()
	assetRoot, err := defaultAssetRoot()
	if err != nil {
		t.Fatal(err)
	}
	ddxDir := filepath.Join(assetRoot, "styles", "DDx")
	entries, err := os.ReadDir(ddxDir)
	if err != nil {
		t.Fatalf("read packaged DDx styles: %v", err)
	}
	var ymlCount int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		ymlCount++
		path := filepath.Join(ddxDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "scope: summary" {
				t.Errorf("%s:%d carries scope: summary (HTML-only; inert on Markdown)", path, i+1)
			}
		}
	}
	if ymlCount == 0 {
		t.Fatalf("no packaged rule files under %s", ddxDir)
	}
}
