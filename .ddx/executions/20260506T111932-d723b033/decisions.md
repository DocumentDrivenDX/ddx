ddx-c96fc86c decisions

WIRE internal/persona/binding.go:18 NewBindingManager — rooted by `cli/internal/persona/reachability.go:11-20` and exercised by the guarded production keepalive.
WIRE internal/persona/claude.go:17 NewClaudeInjector — rooted by `cli/internal/persona/reachability.go:42-60`.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath — called by `cli/cmd/persona.go:616` and `cli/cmd/persona.go:726`.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona — called by `cli/cmd/persona.go:736`.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple — called by `cli/cmd/persona.go:740`.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas — called by `cli/cmd/persona.go:727`.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas — called by `cli/cmd/persona.go:617`.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection — rooted by `cli/internal/persona/reachability.go:48`.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection — rooted by `cli/internal/persona/reachability.go:49-55`.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs — rooted by `cli/internal/persona/reachability.go:56`.
WIRE internal/persona/claude.go:236 formatRoleDisplay — rooted by `cli/internal/persona/reachability.go:57`.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile — rooted by `cli/internal/persona/reachability.go:58`.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas — rooted by `cli/internal/persona/reachability.go:59`.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay — rooted by `cli/internal/persona/reachability.go:60`.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir — rooted by `cli/internal/persona/reachability.go:62` and used as the backing constructor for loaders.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs — called by `cli/cmd/persona.go:659` and rooted by `cli/internal/persona/reachability.go:63`.
