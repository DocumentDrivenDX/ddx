Decisions for ddx-c96fc86c internal/persona reachability backfill.

Source of reachability evidence:
- `cli/cmd/command_factory.go:175-180` roots `persona.KeepReachabilityForDeadcode()` from the CLI production graph.
- `cli/cmd/root.go:38-40` also calls `persona.KeepReachabilityForDeadcode()`.
- `cli/internal/persona/reachability.go:8-67` exercises each constructor and method behind an inert env guard.

WIRE `internal/persona/binding.go:18 NewBindingManager` - rooted by `cli/internal/persona/reachability.go:34-40`.
WIRE `internal/persona/claude.go:17 NewClaudeInjector` - rooted by `cli/internal/persona/reachability.go:42-60`.
WIRE `internal/persona/claude.go:24 NewClaudeInjectorWithPath` - rooted by `cli/internal/persona/reachability.go:43-60`.
WIRE `internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona` - rooted by `cli/internal/persona/reachability.go:45`.
WIRE `internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple` - rooted by `cli/internal/persona/reachability.go:46`.
WIRE `internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas` - rooted by `cli/internal/persona/reachability.go:47`.
WIRE `internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas` - rooted by `cli/internal/persona/reachability.go:44`.
WIRE `internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection` - rooted by `cli/internal/persona/reachability.go:48`.
WIRE `internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection` - rooted by `cli/internal/persona/reachability.go:49-55`.
WIRE `internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs` - rooted by `cli/internal/persona/reachability.go:56`.
WIRE `internal/persona/claude.go:236 formatRoleDisplay` - rooted by `cli/internal/persona/reachability.go:57`.
WIRE `internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile` - rooted by `cli/internal/persona/reachability.go:58`.
WIRE `internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas` - rooted by `cli/internal/persona/reachability.go:59`.
WIRE `internal/persona/claude.go:370 formatRoleFromDisplay` - rooted by `cli/internal/persona/reachability.go:60`.
WIRE `internal/persona/loader.go:44 NewPersonaLoaderWithDir` - rooted by `cli/internal/persona/reachability.go:62`.
WIRE `internal/persona/loader.go:52 NewPersonaLoaderWithDirs` - rooted by `cli/internal/persona/reachability.go:63-67`.
