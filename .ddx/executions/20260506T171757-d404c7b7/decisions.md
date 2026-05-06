WIRE internal/persona/binding.go:18 NewBindingManager - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/internal/server/server.go:1431,4085.
WIRE internal/persona/claude.go:17 NewClaudeInjector - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:616,726.
WIRE internal/persona/claude.go:24 NewClaudeInjectorWithPath - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:616,726.
WIRE internal/persona/claude.go:31 ClaudeInjectorImpl.InjectPersona - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:736.
WIRE internal/persona/claude.go:50 ClaudeInjectorImpl.InjectMultiple - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:740.
WIRE internal/persona/claude.go:96 ClaudeInjectorImpl.RemovePersonas - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:727.
WIRE internal/persona/claude.go:114 ClaudeInjectorImpl.GetLoadedPersonas - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/cmd/persona.go:617.
WIRE internal/persona/claude.go:129 ClaudeInjectorImpl.removePersonasSection - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.InjectMultiple and RemovePersonas.
WIRE internal/persona/claude.go:169 ClaudeInjectorImpl.buildPersonasSection - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.InjectMultiple.
WIRE internal/persona/claude.go:199 ClaudeInjectorImpl.extractRolePersonaPairs - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.GetLoadedPersonas.
WIRE internal/persona/claude.go:236 formatRoleDisplay - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.buildPersonasSection.
WIRE internal/persona/claude.go:261 ClaudeInjectorImpl.saveClaudeFile - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.InjectMultiple and RemovePersonas.
WIRE internal/persona/claude.go:284 ClaudeInjectorImpl.getExistingPersonas - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.InjectPersona.
WIRE internal/persona/claude.go:370 formatRoleFromDisplay - rooted by cli/internal/persona/reachability.go:11-20 and used internally by ClaudeInjectorImpl.getExistingPersonas.
WIRE internal/persona/loader.go:44 NewPersonaLoaderWithDir - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/internal/persona/loader.go:35.
WIRE internal/persona/loader.go:52 NewPersonaLoaderWithDirs - rooted by cli/internal/persona/reachability.go:11-20 and used by cli/internal/persona/loader.go:35 and cli/cmd/persona.go:659,726,1438,4094.
