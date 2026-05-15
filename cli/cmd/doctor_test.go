package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func TestDoctorProseChecker_MissingVale(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	status := checkProseChecker()

	if status.Issue == nil {
		t.Fatal("expected missing vale to produce a prose checker issue")
	}
	if status.Issue.Type != "prose_checker" {
		t.Fatalf("issue.Type = %q, want prose_checker", status.Issue.Type)
	}
	if !strings.Contains(status.Issue.Description, "not installed") {
		t.Fatalf("expected missing vale diagnostic, got %q", status.Issue.Description)
	}

	output, err := runDoctorForProseCheckerTest(t, "")
	if err != nil {
		t.Fatalf("doctor must remain non-fatal for missing Vale; got %v", err)
	}
	if !strings.Contains(output, "Vale prose checker is not installed or is not on PATH") {
		t.Fatalf("expected missing Vale output, got:\n%s", output)
	}
}

func TestDoctorProseChecker_WrongValeVersion(t *testing.T) {
	valePath := writeFakeVale(t, "vale version 3.12.0")
	t.Setenv("PATH", filepath.Dir(valePath))

	status := checkProseChecker()

	if status.Issue == nil {
		t.Fatal("expected wrong vale version to produce a prose checker issue")
	}
	if status.Path != valePath {
		t.Fatalf("status.Path = %q, want %q", status.Path, valePath)
	}
	if status.Version != "vale version 3.12.0" {
		t.Fatalf("status.Version = %q, want vale version 3.12.0", status.Version)
	}
	if !strings.Contains(status.Issue.Description, "Unsupported Vale prose checker version") {
		t.Fatalf("expected unsupported version diagnostic, got %q", status.Issue.Description)
	}

	output, err := runDoctorForProseCheckerTest(t, "vale version 3.12.0")
	if err != nil {
		t.Fatalf("doctor must remain non-fatal for unsupported Vale; got %v", err)
	}
	if !strings.Contains(output, `expected "vale version 3.13.0"`) {
		t.Fatalf("expected pinned Vale version output, got:\n%s", output)
	}
}

func TestDoctorProseChecker_SupportedValeVersion(t *testing.T) {
	valePath := writeFakeVale(t, supportedValeVersion)
	t.Setenv("PATH", filepath.Dir(valePath))

	status := checkProseChecker()

	if status.Issue != nil {
		t.Fatalf("expected supported vale version to pass, got issue: %+v", status.Issue)
	}
	if status.Path != valePath {
		t.Fatalf("status.Path = %q, want %q", status.Path, valePath)
	}
	if status.Version != supportedValeVersion {
		t.Fatalf("status.Version = %q, want %q", status.Version, supportedValeVersion)
	}
}

func TestDoctorProseChecker_VerboseShowsPathAndVersion(t *testing.T) {
	valePath := writeFakeVale(t, supportedValeVersion)
	t.Setenv("PATH", filepath.Dir(valePath))
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)

	factory := NewCommandFactory(t.TempDir())
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--verbose")
	if err != nil {
		t.Fatalf("doctor --verbose returned error: %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "vale path: "+valePath) {
		t.Fatalf("expected verbose output to include Vale path %q, got:\n%s", valePath, output)
	}
	if !strings.Contains(output, "vale version: "+supportedValeVersion) {
		t.Fatalf("expected verbose output to include Vale version, got:\n%s", output)
	}
}

func writeFakeVale(t *testing.T, version string) string {
	t.Helper()

	binDir := t.TempDir()
	valePath := filepath.Join(binDir, "vale")
	script := "#!/bin/sh\nprintf '%s\\n' '" + version + "'\n"
	if err := os.WriteFile(valePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake vale: %v", err)
	}
	return valePath
}

func runDoctorForProseCheckerTest(t *testing.T, valeVersion string) (string, error) {
	t.Helper()

	binDir := t.TempDir()
	if valeVersion != "" {
		valePath := filepath.Join(binDir, "vale")
		script := "#!/bin/sh\nprintf '%s\\n' '" + valeVersion + "'\n"
		if err := os.WriteFile(valePath, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake vale: %v", err)
		}
	}
	t.Setenv("PATH", binDir)
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)

	factory := NewCommandFactory(t.TempDir())
	return executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
}

// TestDoctor_FlagsLegacySymlinks verifies that ddx doctor exits non-zero and
// writes the expected diagnostic strings to stderr when project-local skill
// directories contain symlinks (pre-FEAT-015 install remnants).
func TestDoctor_FlagsLegacySymlinks(t *testing.T) {
	workDir := t.TempDir()

	// Create a symlink under .agents/skills/ to simulate a pre-migration install.
	agentSkillsDir := filepath.Join(workDir, ".agents", "skills")
	if err := os.MkdirAll(agentSkillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// The symlink target need not exist; we only care that a symlink is present.
	if err := os.Symlink("/nonexistent/old-global-path/helix-align", filepath.Join(agentSkillsDir, "helix-align")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	factory := NewCommandFactory(workDir)
	cmd := factory.NewRootCommand()
	cmd.SetArgs([]string{"doctor"})

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.SetOut(&stdoutBuf)
	cmd.SetErr(&stderrBuf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit (error) from doctor with legacy symlinks, got nil")
	}

	// Stderr must contain the symlink warning and the remediation hint.
	stderr := stderrBuf.String()
	if !strings.Contains(stderr, "symlink detected under .agents/skills") {
		t.Errorf("stderr missing 'symlink detected under .agents/skills'; got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "run: ddx update --force") {
		t.Errorf("stderr missing 'run: ddx update --force'; got:\n%s", stderr)
	}
}

func TestDoctor_WarnsOnLegacyModelCatalogFile(t *testing.T) {
	homeDir, err := os.MkdirTemp("", "ddx-home-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	t.Setenv("HOME", homeDir)

	legacyCatalogPath := filepath.Join(homeDir, ddxroot.DirName, "model-catalog.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyCatalogPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy catalog dir: %v", err)
	}
	if err := os.WriteFile(legacyCatalogPath, []byte("updated_at: 2026-01-01T00:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write legacy catalog: %v", err)
	}

	issue := checkLegacyModelCatalogFile()
	if issue == nil {
		t.Fatal("expected legacy model catalog issue, got nil")
	}
	if issue.Type != "legacy_model_catalog" {
		t.Fatalf("issue.Type = %q, want legacy_model_catalog", issue.Type)
	}
	if !strings.Contains(issue.Description, "Legacy DDx-side model catalog file") {
		t.Fatalf("unexpected issue description: %q", issue.Description)
	}
	if !strings.Contains(issue.Remediation[0], legacyCatalogPath) {
		t.Fatalf("issue remediation missing catalog path; got %v", issue.Remediation)
	}
}

// TestCheckPackageJSONLocations_NoPkgJSON verifies no issues when no package.json exists.
func TestCheckPackageJSONLocations_NoPkgJSON(t *testing.T) {
	workDir := t.TempDir()
	issues := checkPackageJSONLocations(workDir)
	if len(issues) != 0 {
		t.Errorf("expected no issues with no package.json, got %v", issues)
	}
}

// TestCheckPackageJSONLocations_MissingNodeModules detects absent node_modules.
func TestCheckPackageJSONLocations_MissingNodeModules(t *testing.T) {
	workDir := t.TempDir()

	// Write a package.json at repo root with no node_modules.
	if err := os.WriteFile(filepath.Join(workDir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	issues := checkPackageJSONLocations(workDir)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Type != "node_modules_missing" {
		t.Errorf("expected type node_modules_missing, got %s", issues[0].Type)
	}
	found := false
	for _, r := range issues[0].Remediation {
		if strings.Contains(r, "bun install") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'bun install' in remediation, got %v", issues[0].Remediation)
	}
}

// TestCheckPackageJSONLocations_StaleNodeModules detects node_modules older than bun.lock.
func TestCheckPackageJSONLocations_StaleNodeModules(t *testing.T) {
	workDir := t.TempDir()

	// Create package.json, node_modules, then a newer bun.lock.
	if err := os.WriteFile(filepath.Join(workDir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	// Write bun.lock after node_modules so it has a newer mtime.
	if err := os.WriteFile(filepath.Join(workDir, "bun.lock"), []byte(`lockfileVersion: 0`), 0o644); err != nil {
		t.Fatalf("write bun.lock: %v", err)
	}
	// Backdate node_modules to ensure bun.lock is strictly newer.
	past := filepath.Join(workDir, "node_modules")
	info, _ := os.Stat(past)
	lockInfo, _ := os.Stat(filepath.Join(workDir, "bun.lock"))
	if !lockInfo.ModTime().After(info.ModTime()) {
		// Force the difference by touching bun.lock again.
		now := lockInfo.ModTime().Add(2 * 1e9) // +2s
		if err := os.Chtimes(filepath.Join(workDir, "bun.lock"), now, now); err != nil {
			t.Fatalf("chtimes bun.lock: %v", err)
		}
	}

	issues := checkPackageJSONLocations(workDir)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0].Type != "node_modules_stale" {
		t.Errorf("expected type node_modules_stale, got %s", issues[0].Type)
	}
}

// TestCheckPackageJSONLocations_SubdirDetected verifies detection in known subdirs.
func TestCheckPackageJSONLocations_SubdirDetected(t *testing.T) {
	workDir := t.TempDir()

	subdir := filepath.Join(workDir, "website")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir website: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	issues := checkPackageJSONLocations(workDir)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue for website/, got %d: %v", len(issues), issues)
	}
	if issues[0].Type != "node_modules_missing" {
		t.Errorf("expected node_modules_missing, got %s", issues[0].Type)
	}
	if !strings.Contains(issues[0].Description, "website") {
		t.Errorf("expected description to mention 'website', got: %s", issues[0].Description)
	}
}

// TestCheckPackageJSONLocations_UpToDate verifies no issue when node_modules is newer than bun.lock.
func TestCheckPackageJSONLocations_UpToDate(t *testing.T) {
	workDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(workDir, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	// Write bun.lock first (older), then node_modules (newer).
	if err := os.WriteFile(filepath.Join(workDir, "bun.lock"), []byte(`lockfileVersion: 0`), 0o644); err != nil {
		t.Fatalf("write bun.lock: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	// Ensure node_modules mtime is strictly after bun.lock.
	lockInfo, _ := os.Stat(filepath.Join(workDir, "bun.lock"))
	future := lockInfo.ModTime().Add(2 * 1e9)
	if err := os.Chtimes(filepath.Join(workDir, "node_modules"), future, future); err != nil {
		t.Fatalf("chtimes node_modules: %v", err)
	}

	issues := checkPackageJSONLocations(workDir)
	if len(issues) != 0 {
		t.Errorf("expected no issues (up to date), got %v", issues)
	}
}

// TestLegacySkillSymlinkDirs_NoSymlinks verifies that legacySkillSymlinkDirs
// returns nil when skill dirs contain only real files.
func TestLegacySkillSymlinkDirs_NoSymlinks(t *testing.T) {
	workDir := t.TempDir()

	// Create a real directory (not a symlink) under .agents/skills/.
	if err := os.MkdirAll(filepath.Join(workDir, ".agents", "skills", "ddx"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := legacySkillSymlinkDirs(workDir)
	if len(got) != 0 {
		t.Errorf("expected no legacy dirs, got %v", got)
	}
}

// TestLegacySkillSymlinkDirs_DetectsClaudeSkills verifies detection in
// .claude/skills/ as well as .agents/skills/.
func TestLegacySkillSymlinkDirs_DetectsClaudeSkills(t *testing.T) {
	workDir := t.TempDir()

	claudeSkillsDir := filepath.Join(workDir, ".claude", "skills")
	if err := os.MkdirAll(claudeSkillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink("/nonexistent/target", filepath.Join(claudeSkillsDir, "helix-run")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got := legacySkillSymlinkDirs(workDir)
	if len(got) == 0 {
		t.Fatal("expected legacy dir detected, got none")
	}
	found := false
	for _, d := range got {
		if strings.Contains(d, ".claude/skills") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected .claude/skills in detected dirs, got %v", got)
	}
}
