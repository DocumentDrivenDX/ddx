WIRE internal/persona/binding.go:18 NewBindingManager - reachable from main() via cli/cmd/root.go:46 -> cmd.Execute -> cli/cmd/command_factory.go:183 -> persona.KeepReachabilityForDeadcode -> internal/persona/reachability.go:34.
WIRE internal/persona/claude.go:17 NewClaudeInjector - reachable from main() via cli/cmd/root.go:46 -> cmd.Execute -> cli/cmd/command_factory.go:183 -> persona.KeepReachabilityForDeadcode -> internal/persona/reachability.go:42.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - reachable from main() via cli/cmd/persona.go:616,726 and the deadcode anchor path.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - reachable from cli/cmd/persona.go:736 and the deadcode anchor path.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - reachable from cli/cmd/persona.go:740 and the deadcode anchor path.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - reachable from cli/cmd/persona.go:727 and the deadcode anchor path.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - reachable from cli/cmd/persona.go:617 and the deadcode anchor path.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - reachable through InjectMultiple/RemovePersonas and the deadcode anchor path.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - reachable through InjectMultiple and the deadcode anchor path.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - reachable through GetLoadedPersonas and the deadcode anchor path.
WIRE internal/persona/claude.go:236 formatRoleDisplay - reachable through buildPersonasSection and the deadcode anchor path.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - reachable through InjectMultiple/RemovePersonas and the deadcode anchor path.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - reachable through InjectPersona and the deadcode anchor path.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - reachable through getExistingPersonas and the deadcode anchor path.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - reachable from main() via cli/cmd/persona.go:659 and the deadcode anchor path.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - reachable from main() via cli/cmd/persona.go:659 and the deadcode anchor path.
