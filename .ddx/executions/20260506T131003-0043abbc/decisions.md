NewBindingManager: WIRE rooted by `cli/cmd/command_factory.go:177` -> `persona.KeepReachabilityForDeadcode()` and `cli/internal/persona/reachability.go:34-35`.
NewClaudeInjector: WIRE rooted by `cli/cmd/command_factory.go:177` -> `persona.KeepReachabilityForDeadcode()` and `cli/internal/persona/reachability.go:42-43`.
NewClaudeInjectorWithPath: WIRE rooted by `cli/cmd/persona.go:616` and `cli/cmd/persona.go:726`, with the explicit deadcode anchor in `cli/internal/persona/reachability.go:42-43`.
ClaudeInjectorImpl.InjectPersona: WIRE rooted by `cli/cmd/persona.go:736` and `cli/internal/persona/reachability.go:45`.
ClaudeInjectorImpl.InjectMultiple: WIRE rooted by `cli/cmd/persona.go:740` and `cli/internal/persona/reachability.go:46`.
ClaudeInjectorImpl.RemovePersonas: WIRE rooted by `cli/cmd/persona.go:727` and `cli/internal/persona/reachability.go:47`.
ClaudeInjectorImpl.GetLoadedPersonas: WIRE rooted by `cli/cmd/persona.go:617` and `cli/internal/persona/reachability.go:44`.
ClaudeInjectorImpl.removePersonasSection: WIRE rooted by `cli/cmd/persona.go:727` and `cli/internal/persona/reachability.go:48`.
ClaudeInjectorImpl.buildPersonasSection: WIRE rooted by `cli/cmd/persona.go:740` and `cli/internal/persona/reachability.go:49-55`.
ClaudeInjectorImpl.extractRolePersonaPairs: WIRE rooted by `cli/cmd/persona.go:617` and `cli/internal/persona/reachability.go:56`.
formatRoleDisplay: WIRE rooted by `cli/internal/persona/reachability.go:57` and `cli/internal/persona/claude.go:185`.
ClaudeInjectorImpl.saveClaudeFile: WIRE rooted by `cli/internal/persona/reachability.go:58` and exercised by `cli/cmd/persona.go:727-740`.
ClaudeInjectorImpl.getExistingPersonas: WIRE rooted by `cli/internal/persona/reachability.go:59` and exercised by `cli/internal/persona/claude.go:43`.
formatRoleFromDisplay: WIRE rooted by `cli/internal/persona/reachability.go:60` and exercised by `cli/internal/persona/claude.go:327,357`.
NewPersonaLoaderWithDir: WIRE rooted by `cli/cmd/command_factory.go:177` -> `persona.KeepReachabilityForDeadcode()` and `cli/internal/persona/reachability.go:62`.
NewPersonaLoaderWithDirs: WIRE rooted by `cli/cmd/persona.go:659` and `cli/internal/persona/reachability.go:63`.
