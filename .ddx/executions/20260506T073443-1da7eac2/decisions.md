WIRE internal/persona/binding.go:18 NewBindingManager via cli/cmd/command_factory.go:174-178 -> persona.KeepReachabilityForDeadcode() and internal/persona/reachability.go:34-39.
WIRE internal/persona/claude.go:17 NewClaudeInjector via cli/cmd/command_factory.go:174-178 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath via cli/cmd/command_factory.go:174-178 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona via cli/cmd/persona.go:736 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple via cli/cmd/persona.go:740 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas via cli/cmd/persona.go:727 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas via cli/cmd/persona.go:617 and internal/persona/reachability.go:42-60.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection via internal/persona/reachability.go:47-49 and internal/persona/claude.go:129-167.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection via internal/persona/reachability.go:49-55 and internal/persona/claude.go:169-196.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs via internal/persona/reachability.go:56 and internal/persona/claude.go:199-232.
WIRE internal/persona/claude.go:236 formatRoleDisplay via internal/persona/reachability.go:57 and internal/persona/claude.go:236-258.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile via internal/persona/reachability.go:58 and internal/persona/claude.go:261-280.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas via internal/persona/reachability.go:59 and internal/persona/claude.go:284-367.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay via internal/persona/reachability.go:60 and internal/persona/claude.go:370-380.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir via internal/persona/reachability.go:62-63 and internal/persona/loader.go:38-52.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs via internal/persona/reachability.go:63-68 and internal/persona/loader.go:47-52.
