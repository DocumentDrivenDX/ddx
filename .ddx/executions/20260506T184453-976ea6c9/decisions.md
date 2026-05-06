WIRE internal/persona/binding.go:18 NewBindingManager - rooted by `cmd.Execute` -> `persona.KeepReachabilityForDeadcode()` -> `keepPersonaReachability()` in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:17 NewClaudeInjector - rooted by the same production keepalive path in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - rooted by the same production keepalive path in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:236 formatRoleDisplay - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - exercised by the keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - rooted by the production keepalive helper in `cli/internal/persona/reachability.go`.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - rooted by the production keepalive helper in `cli/internal/persona/reachability.go`.
