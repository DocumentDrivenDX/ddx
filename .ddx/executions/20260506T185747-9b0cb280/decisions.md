WIRE internal/persona/binding.go:18 NewBindingManager - rooted from main via cli/cmd/command_factory.go:183 -> cli/internal/persona/reachability.go:34-40.
WIRE internal/persona/claude.go:17 NewClaudeInjector - rooted from main via cli/cmd/command_factory.go:183 -> cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - called by cli/cmd/persona.go:616-617 and 726-727, and also anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - called by cli/cmd/persona.go:736-737 and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - called by cli/cmd/persona.go:739-741 and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - called by cli/cmd/persona.go:726-729 and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - called by cli/cmd/persona.go:616-619 and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - reached via InjectMultiple/RemovePersonas and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - reached via InjectMultiple and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - reached via GetLoadedPersonas and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:236 formatRoleDisplay - reached via buildPersonasSection and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - reached via InjectMultiple/RemovePersonas and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - reached via InjectPersona and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - reached via getExistingPersonas and anchored from cli/internal/persona/reachability.go:42-60.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - rooted from cli/cmd/command_factory.go:183 -> cli/internal/persona/reachability.go:62-67.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - called by cli/internal/persona/loader.go:35 and rooted from cli/cmd/command_factory.go:183 -> cli/internal/persona/reachability.go:62-67.
