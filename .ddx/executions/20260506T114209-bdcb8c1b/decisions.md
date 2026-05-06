WIRE internal/persona/binding.go:18 NewBindingManager - reachable from the production CLI graph via the persona command path in cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:17 NewClaudeInjector - reachable from the production CLI graph via the persona command path in cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - reachable from the production CLI graph via the persona command path in cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - reachable through InjectMultiple/RemovePersonas and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - reachable through InjectMultiple and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - reachable through GetLoadedPersonas and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:236 formatRoleDisplay - reachable through buildPersonasSection and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - reachable through InjectMultiple/RemovePersonas and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - reachable through InjectPersona and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - reachable through getExistingPersonas and the production reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - reachable from the production CLI graph via cli/cmd/persona.go and the reachability anchor in cli/internal/persona/reachability.go.
