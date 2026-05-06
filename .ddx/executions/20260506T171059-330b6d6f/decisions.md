WIRE internal/persona/binding.go:18 NewBindingManager - reachable through the CLI production root in cli/cmd/root.go:37-47 and the persona keepalive hook in cli/cmd/command_factory.go:176-184; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:17 NewClaudeInjector - reachable through the CLI production root in cli/cmd/root.go:37-47 and the persona keepalive hook in cli/cmd/command_factory.go:176-184; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - reachable through the CLI persona command path in cli/cmd/command_factory_commands.go:207 and cli/cmd/persona.go:616-740; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - reachable through cli/cmd/persona.go:725-738; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - reachable through cli/cmd/persona.go:725-742; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - reachable through cli/cmd/persona.go:725-730; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - reachable through cli/cmd/persona.go:616-620; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - reachable as an internal helper from ClaudeInjectorImpl.RemovePersonas/InjectMultiple via cli/cmd/persona.go:725-742; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - reachable as an internal helper from ClaudeInjectorImpl.InjectMultiple via cli/cmd/persona.go:725-742; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - reachable through ClaudeInjectorImpl.GetLoadedPersonas via cli/cmd/persona.go:616-620; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:236 formatRoleDisplay - reachable as an internal helper from ClaudeInjectorImpl.buildPersonasSection via cli/cmd/persona.go:725-742; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - reachable through cli/cmd/persona.go:725-742; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - reachable as an internal helper from ClaudeInjectorImpl.InjectPersona via cli/cmd/persona.go:725-738; deadcode no longer reports this symbol.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - reachable as an internal helper from ClaudeInjectorImpl.getExistingPersonas via cli/cmd/persona.go:725-738; deadcode no longer reports this symbol.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - reachable through cli/cmd/persona.go:659 and cli/cmd/persona.go:680; deadcode no longer reports this symbol.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - reachable through cli/cmd/persona.go:659 and cli/cmd/persona.go:680; deadcode no longer reports this symbol.
