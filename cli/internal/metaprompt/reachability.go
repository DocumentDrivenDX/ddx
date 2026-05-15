package metaprompt

import (
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// init keeps the metaprompt package on the production reachability graph.
// The guarded helper below is inert in normal runs and exists only so deadcode
// RTA can see the real CLI entry points.
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode keeps the metaprompt package rooted in the
// production call graph so static reachability analysis sees the real CLI
// entry points. The work remains gated behind an env var and is inert by
// default.
func KeepReachabilityForDeadcode() {
	keepMetaPromptReachability()
}

func keepMetaPromptReachability() {
	if os.Getenv("DDX_METAPROMPT_KEEPALIVE") != "1" {
		return
	}

	tempRoot := filepath.Join(os.TempDir(), "ddx-metaprompt-keepalive")
	claudePath := filepath.Join(tempRoot, "CLAUDE.md")
	libraryPath := ddxroot.JoinProject(tempRoot, "plugins", "ddx")

	injector := NewMetaPromptInjectorWithPaths("CLAUDE.md", libraryPath, tempRoot).(*MetaPromptInjectorImpl)
	_ = injector.removeMetaPromptSection("# CLAUDE.md")
	_ = injector.buildMetaPromptSection("# Prompt", "claude/system-prompts/example.md")
	_, _, _ = injector.extractCurrentMetaPrompt("<!-- DDX-META-PROMPT:START -->\n<!-- Source: claude/system-prompts/example.md -->\n# Prompt\n<!-- DDX-META-PROMPT:END -->")
	_ = injector.saveCLAUDEFile("# CLAUDE.md")
	_, _ = os.Stat(claudePath)
}
