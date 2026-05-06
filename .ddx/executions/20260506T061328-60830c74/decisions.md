WIRE `internal/persona/binding.go:18` `NewBindingManager` - constructor is the shared root for `NewBindingManagerWithPath`, which is used by production server and GraphQL resolvers.
WIRE `internal/persona/binding.go:25` `NewBindingManagerWithPath` - called from `cli/internal/server/server.go:1431,4085` and `cli/internal/server/graphql/resolver_feat008.go:223`.
WIRE `internal/persona/claude.go:17` `NewClaudeInjector` - constructor is the shared root for `NewClaudeInjectorWithPath`, which is used by CLI persona commands.
WIRE `internal/persona/claude.go:24` `NewClaudeInjectorWithPath` - called from `cli/cmd/persona.go:616,726`.
WIRE `internal/persona/claude.go:31` `ClaudeInjectorImpl.InjectPersona` - reached from `cli/cmd/persona.go:736` via `personaLoad`.
WIRE `internal/persona/claude.go:50` `ClaudeInjectorImpl.InjectMultiple` - reached from `cli/cmd/persona.go:740` via `personaLoad`.
WIRE `internal/persona/claude.go:96` `ClaudeInjectorImpl.RemovePersonas` - reached from `cli/cmd/persona.go:727` via `personaLoad`.
WIRE `internal/persona/claude.go:114` `ClaudeInjectorImpl.GetLoadedPersonas` - reached from `cli/cmd/persona.go:617` via `personaStatus`.
WIRE `internal/persona/claude.go:129` `ClaudeInjectorImpl.removePersonasSection` - used by `InjectMultiple` and `RemovePersonas`.
WIRE `internal/persona/claude.go:169` `ClaudeInjectorImpl.buildPersonasSection` - used by `InjectMultiple`.
WIRE `internal/persona/claude.go:199` `ClaudeInjectorImpl.extractRolePersonaPairs` - used by `GetLoadedPersonas`.
WIRE `internal/persona/claude.go:236` `formatRoleDisplay` - used by `buildPersonasSection`.
WIRE `internal/persona/claude.go:261` `ClaudeInjectorImpl.saveClaudeFile` - used by `InjectMultiple` and `RemovePersonas`.
WIRE `internal/persona/claude.go:284` `ClaudeInjectorImpl.getExistingPersonas` - used by `InjectPersona`.
WIRE `internal/persona/claude.go:370` `formatRoleFromDisplay` - used by `getExistingPersonas`.
WIRE `internal/persona/loader.go:41` `NewPersonaLoaderWithDir` - used by `NewPersonaLoaderWithDirs`, which is called from production CLI/server paths.
WIRE `internal/persona/loader.go:49` `NewPersonaLoaderWithDirs` - used by `NewPersonaLoader`, which is called from production CLI/server paths.
