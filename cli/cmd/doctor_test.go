package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
