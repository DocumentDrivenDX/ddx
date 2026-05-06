Deadcode reachability decisions for `internal/persona`

Verification:
- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from `cli/` reports no remaining `internal/persona` dead symbols in the current tree.
- `cd cli && go test ./...` passes.

WIRE internal/persona/binding.go:18 NewBindingManager - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and exercised by runtime binding paths in `cli/cmd/persona.go:616`, `cli/internal/server/server.go:1431`, `cli/internal/server/graphql/resolver_feat008.go:223`.
WIRE internal/persona/claude.go:17 NewClaudeInjector - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and exercised by runtime injection paths in `cli/cmd/persona.go:616` and `cli/cmd/persona.go:726`.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and used at runtime by `cli/cmd/persona.go:616`, `cli/cmd/persona.go:726`, `cli/internal/server/server.go:1431`, `cli/internal/server/server.go:4085`, and `cli/internal/server/graphql/resolver_feat008.go:223`.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and called by `cli/cmd/persona.go:736`.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and called by `cli/cmd/persona.go:740`.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and called by `cli/cmd/persona.go:727`.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and called by `cli/cmd/persona.go:617` and `cli/internal/persona/reachability.go:44`.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:48`.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:49`.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:56`.
WIRE internal/persona/claude.go:236 formatRoleDisplay - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:57`.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:58`.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:59`.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and referenced by `cli/internal/persona/reachability.go:60`.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and used by `cli/internal/persona/reachability.go:62` and loader tests.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - rooted by `cmd.NewRootCommand()` -> `persona.KeepReachabilityForDeadcode()` and used by `cli/cmd/persona.go:659`, `cli/internal/persona/reachability.go:63`, and project persona tests.
