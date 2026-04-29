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
