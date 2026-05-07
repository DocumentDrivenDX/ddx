package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDDxAgentModeGuidance_DocContract asserts that the three governing
// documents (AGENTS.md, FEAT-010, FEAT-011) together name all four
// DDX_MODE values and include the ddx work carve-out that separates
// interactive queue stewardship from worker execution.
func TestDDxAgentModeGuidance_DocContract(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	fourModes := []string{
		"queue_steward",
		"bead_execution",
		"direct_user_implementation",
		"review",
	}

	// --- AGENTS.md ---
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	agentsBytes, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("AGENTS.md must be readable: %v", err)
	}
	agents := string(agentsBytes)

	defaultModeIdx := strings.Index(agents, "Default Interactive Mode")
	if defaultModeIdx == -1 {
		t.Fatal("AGENTS.md must contain a 'Default Interactive Mode' section")
	}
	beadPolicyIdx := strings.Index(agents, "## Bead Policy")
	if beadPolicyIdx == -1 {
		t.Fatal("AGENTS.md must contain a '## Bead Policy' section")
	}
	if defaultModeIdx >= beadPolicyIdx {
		t.Errorf("AGENTS.md: 'Default Interactive Mode' (pos %d) must appear before '## Bead Policy' (pos %d)",
			defaultModeIdx, beadPolicyIdx)
	}

	for _, mode := range fourModes {
		if !strings.Contains(agents, mode) {
			t.Errorf("AGENTS.md must name mode %q", mode)
		}
	}
	if !strings.Contains(agents, "ddx work") {
		t.Error("AGENTS.md must include the ddx work carve-out")
	}

	// --- FEAT-010 ---
	feat010Path := filepath.Join(repoRoot, "docs/helix/01-frame/features/FEAT-010-task-execution.md")
	feat010Bytes, err := os.ReadFile(feat010Path)
	if err != nil {
		t.Fatalf("FEAT-010 must be readable: %v", err)
	}
	feat010 := string(feat010Bytes)

	for _, mode := range fourModes {
		if !strings.Contains(feat010, mode) {
			t.Errorf("FEAT-010 must define mode %q", mode)
		}
	}
	if !strings.Contains(feat010, "DDX_MODE") {
		t.Error("FEAT-010 must name DDX_MODE")
	}
	if !strings.Contains(feat010, "precedence") && !strings.Contains(feat010, "Precedence") {
		t.Error("FEAT-010 must define precedence rules for the modes")
	}
	if !strings.Contains(feat010, "ddx work") {
		t.Error("FEAT-010 must reference ddx work in the mode taxonomy")
	}

	// --- FEAT-011 ---
	feat011Path := filepath.Join(repoRoot, "docs/helix/01-frame/features/FEAT-011-skills.md")
	feat011Bytes, err := os.ReadFile(feat011Path)
	if err != nil {
		t.Fatalf("FEAT-011 must be readable: %v", err)
	}
	feat011 := string(feat011Bytes)

	if !strings.Contains(feat011, "queue_steward") {
		t.Error("FEAT-011 must reference queue_steward mode")
	}
	if !strings.Contains(feat011, "bead_execution") {
		t.Error("FEAT-011 must reference bead_execution mode")
	}
	if !strings.Contains(feat011, "ddx work") {
		t.Error("FEAT-011 must describe routing for explicit ddx work execution prompts")
	}
}
