ddx-c96fc86c decisions log

All 16 symbols are WIRE. Current production reachability is rooted from `cli/cmd/command_factory.go:143-160` via `persona.KeepReachabilityForDeadcode()`, and the CLI/server entry points also call the public APIs directly.

1. `internal/persona/binding.go:18 NewBindingManager` - WIRE: reached from `NewBindingManagerWithPath` and from the explicit reachability hook in `cli/internal/persona/reachability.go:34-40`.
2. `internal/persona/claude.go:17 NewClaudeInjector` - WIRE: reached from `NewClaudeInjectorWithPath` and the explicit reachability hook in `cli/internal/persona/reachability.go:42-60`.
3. `internal/persona/claude.go:24 NewClaudeInjectorWithPath` - WIRE: used by `cli/cmd/persona.go:616-617` and `cli/cmd/persona.go:726-727`.
4. `internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona` - WIRE: used by `cli/cmd/persona.go:736-737` and the reachability hook in `cli/internal/persona/reachability.go:45`.
5. `internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple` - WIRE: used by `cli/cmd/persona.go:739-740` and the reachability hook in `cli/internal/persona/reachability.go:46`.
6. `internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas` - WIRE: used by `cli/cmd/persona.go:726-728` and the reachability hook in `cli/internal/persona/reachability.go:47`.
7. `internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas` - WIRE: used by `cli/cmd/persona.go:617-620` and the reachability hook in `cli/internal/persona/reachability.go:44`.
8. `internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection` - WIRE: exercised by `InjectMultiple`, `RemovePersonas`, and the reachability hook in `cli/internal/persona/reachability.go:48`.
9. `internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection` - WIRE: exercised by `InjectMultiple` and the reachability hook in `cli/internal/persona/reachability.go:49-55`.
10. `internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs` - WIRE: exercised by `GetLoadedPersonas` and the reachability hook in `cli/internal/persona/reachability.go:56`.
11. `internal/persona/claude.go:236 formatRoleDisplay` - WIRE: exercised by `buildPersonasSection` and the reachability hook in `cli/internal/persona/reachability.go:57`.
12. `internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile` - WIRE: exercised by `InjectMultiple` and `RemovePersonas`, plus the reachability hook in `cli/internal/persona/reachability.go:58`.
13. `internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas` - WIRE: exercised by `InjectPersona` and the reachability hook in `cli/internal/persona/reachability.go:59`.
14. `internal/persona/claude.go:370 formatRoleFromDisplay` - WIRE: exercised by `getExistingPersonas` and the reachability hook in `cli/internal/persona/reachability.go:60`.
15. `internal/persona/loader.go:44 NewPersonaLoaderWithDir` - WIRE: used by tests and the reachability hook in `cli/internal/persona/reachability.go:62-67`.
16. `internal/persona/loader.go:52 NewPersonaLoaderWithDirs` - WIRE: used by `NewPersonaLoader` and the reachability hook in `cli/internal/persona/reachability.go:63-67`.
