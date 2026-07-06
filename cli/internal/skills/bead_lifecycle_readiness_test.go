package skills

import (
	"strings"
	"testing"
)

// TestBeadLifecycleSkill_ReadinessIsTextOnly guards ddx-d5d1ada7: the readiness
// guidance in the embedded bead-lifecycle skill must judge the bead's own text,
// not send the agent to investigate the repository.
func TestBeadLifecycleSkill_ReadinessIsTextOnly(t *testing.T) {
	data, err := SkillFiles.ReadFile("ddx/bead-lifecycle/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded bead-lifecycle skill: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Do NOT read files, run commands, grep, or otherwise explore the") {
		t.Error("readiness guidance must forbid repository exploration (text-only assessment)")
	}
	if strings.Contains(content, "cheap repository evidence") {
		t.Error("readiness guidance must not invite 'cheap repository evidence'")
	}
}
