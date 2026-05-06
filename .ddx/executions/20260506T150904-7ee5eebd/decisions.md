# ddx-c96fc86c decisions

All 16 symbols in `internal/persona` are wired into the production call graph through `cmd.Execute` -> `persona.KeepReachabilityForDeadcode()` -> `keepPersonaReachability()` in `cli/internal/persona/reachability.go`.

- `internal/persona/binding.go:18` `NewBindingManager` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:17` `NewClaudeInjector` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:24` `NewClaudeInjectorWithPath` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:31` `ClaudeInjectorImpl.InjectPersona` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:50` `ClaudeInjectorImpl.InjectMultiple` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:96` `ClaudeInjectorImpl.RemovePersonas` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:114` `ClaudeInjectorImpl.GetLoadedPersonas` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:129` `ClaudeInjectorImpl.removePersonasSection` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:169` `ClaudeInjectorImpl.buildPersonasSection` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:199` `ClaudeInjectorImpl.extractRolePersonaPairs` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:236` `formatRoleDisplay` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:261` `ClaudeInjectorImpl.saveClaudeFile` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:284` `ClaudeInjectorImpl.getExistingPersonas` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/claude.go:370` `formatRoleFromDisplay` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/loader.go:44` `NewPersonaLoaderWithDir` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
- `internal/persona/loader.go:52` `NewPersonaLoaderWithDirs` - WIRE: invoked from `keepPersonaReachability()` and reachable from `main()` through `cmd.Execute`.
