NewBindingManager: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live callers in `cli/internal/server/graphql/resolver_feat008.go` plus `cli/internal/server/server.go`.
NewClaudeInjector: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live callers in `cli/cmd/persona.go` (`personaStatus`, `personaLoad`).
NewClaudeInjectorWithPath: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live callers in `cli/cmd/persona.go` (`personaStatus`, `personaLoad`).
ClaudeInjectorImpl.InjectPersona: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live caller in `cli/cmd/persona.go` (`personaLoad`).
ClaudeInjectorImpl.InjectMultiple: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live caller in `cli/cmd/persona.go` (`personaLoad`).
ClaudeInjectorImpl.RemovePersonas: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live caller in `cli/cmd/persona.go` (`personaLoad`).
ClaudeInjectorImpl.GetLoadedPersonas: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live caller in `cli/cmd/persona.go` (`personaStatus`).
ClaudeInjectorImpl.removePersonasSection: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
ClaudeInjectorImpl.buildPersonasSection: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
ClaudeInjectorImpl.extractRolePersonaPairs: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
formatRoleDisplay: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
ClaudeInjectorImpl.saveClaudeFile: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
ClaudeInjectorImpl.getExistingPersonas: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
formatRoleFromDisplay: WIRE reachable as a helper of live `ClaudeInjectorImpl` methods invoked by `cli/cmd/persona.go`.
NewPersonaLoaderWithDir: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live callers in `cli/cmd/persona.go`, `cli/internal/server/server.go`, and `cli/internal/server/graphql/resolver_meta.go`.
NewPersonaLoaderWithDirs: WIRE reachable from `cmd.Execute()` -> `persona.KeepReachabilityForDeadcode()` and live callers in `cli/cmd/persona.go`, `cli/internal/server/server.go`, `cli/internal/server/graphql/resolver_meta.go`, and `cli/internal/persona/project_personas_test.go`.
