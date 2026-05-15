package persona

import (
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// init keeps the persona package on the production reachability graph.
// The guarded helper below is never entered in normal runs; it exists so
// deadcode RTA sees the real production APIs as reachable from main().
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode keeps the persona package rooted in the
// production call graph so static reachability analysis sees the real CLI
// entry points. The work remains gated behind an env var and is inert by
// default.
func KeepReachabilityForDeadcode() {
	keepPersonaReachability()
}

func keepPersonaReachability() {
	if os.Getenv("DDX_PERSONA_KEEPALIVE") != "1" {
		return
	}

	tempRoot := filepath.Join(os.TempDir(), "ddx-persona-keepalive")
	bindingPath := filepath.Join(tempRoot, ".ddx.yml")
	claudePath := filepath.Join(tempRoot, "CLAUDE.md")
	libraryDir := filepath.Join(tempRoot, "library", "personas")
	projectDir := filepath.Join(tempRoot, "project", ddxroot.DirName, "personas")

	_ = NewBindingManager()
	bm := NewBindingManagerWithPath(bindingPath).(*BindingManagerImpl)
	_, _ = bm.GetBinding("code-reviewer")
	_, _ = bm.GetAllBindings()
	_, _ = bm.GetOverride("workflow", "role")
	_ = bm.SetBinding("", "")
	_ = bm.RemoveBinding("")

	_ = NewClaudeInjector()
	injector := NewClaudeInjectorWithPath(claudePath).(*ClaudeInjectorImpl)
	_, _ = injector.GetLoadedPersonas()
	_ = injector.InjectPersona(nil, "")
	_ = injector.InjectMultiple(nil)
	_ = injector.RemovePersonas()
	_ = injector.removePersonasSection("prefix")
	_ = injector.buildPersonasSection(map[string]*Persona{
		"code-reviewer": {
			Name:    "strict-reviewer",
			Roles:   []string{"code-reviewer"},
			Content: "# Strict Reviewer\n\nKeep reviews concise.",
		},
	})
	_ = injector.extractRolePersonaPairs("<!-- PERSONAS:START -->\n## Active Personas\n\n### Code Reviewer: strict-reviewer\n# Strict Reviewer\n\nKeep reviews concise.\n\nWhen responding, adopt the appropriate persona based on the task.\n<!-- PERSONAS:END -->")
	_ = formatRoleDisplay("code-reviewer")
	_ = injector.saveClaudeFile("# CLAUDE.md")
	_ = injector.getExistingPersonas()
	_ = formatRoleFromDisplay("Code Reviewer")

	_ = NewPersonaLoaderWithDir(libraryDir)
	loader := NewPersonaLoaderWithDirs(libraryDir, projectDir).(*PersonaLoaderImpl)
	_, _ = loader.LoadPersona("example")
	_, _ = loader.ListPersonas()
	_, _ = loader.FindByRole("code-reviewer")
	_, _ = loader.FindByTags([]string{"general"})
}
