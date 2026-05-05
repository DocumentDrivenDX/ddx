WIRE internal/persona/binding.go:18 NewBindingManager - kept alive by `cli/internal/persona/reachability.go` init guard and used through the CLI persona command path.
WIRE internal/persona/claude.go:17 NewClaudeInjector - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - kept alive by `cli/internal/persona/reachability.go` init guard and used through `cli/cmd/persona.go`.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:236 formatRoleDisplay - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - kept alive by `cli/internal/persona/reachability.go` init guard and used internally by injector methods.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - kept alive by `cli/internal/persona/reachability.go` init guard and used by `cli/cmd/persona.go`.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - kept alive by `cli/internal/persona/reachability.go` init guard and used by `cli/internal/persona/loader.go` and CLI/server call paths.
