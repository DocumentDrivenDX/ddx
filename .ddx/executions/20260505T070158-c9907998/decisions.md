internal/persona/binding.go:18 — WIRE: `NewBindingManagerWithPath` now delegates through `NewBindingManager`, and the existing production server path keeps the constructor on the live graph.
internal/persona/claude.go:17 — WIRE: `NewClaudeInjectorWithPath` now delegates through `NewClaudeInjector`, and `persona load` / `persona status` construct the path-based injector in production.
internal/persona/claude.go:24 — WIRE: production CLI code now reaches the path-based injector constructor, so the default constructor stays live through that chain.
internal/persona/claude.go:31 — WIRE: `persona load` uses `InjectPersona` for single-persona loads.
internal/persona/claude.go:50 — WIRE: `persona load` uses `InjectMultiple` for bound persona loads.
internal/persona/claude.go:96 — WIRE: `persona load` clears existing persona sections before reinjecting active personas.
internal/persona/claude.go:114 — WIRE: `persona status` reads the active persona map from CLAUDE.md in production.
internal/persona/claude.go:129 — WIRE: reachable through `RemovePersonas` from the CLI load flow.
internal/persona/claude.go:169 — WIRE: reachable through `InjectMultiple` from the CLI load flow.
internal/persona/claude.go:199 — WIRE: reachable through `GetLoadedPersonas` when the CLI renders persona status.
internal/persona/claude.go:236 — WIRE: reachable through `buildPersonasSection` while rendering loaded personas.
internal/persona/claude.go:261 — WIRE: reachable through `InjectPersona` / `InjectMultiple` file writes in production.
internal/persona/claude.go:284 — WIRE: reachable through `InjectPersona` when it merges existing CLAUDE.md personas.
internal/persona/claude.go:370 — WIRE: reachable through `getExistingPersonas` while merging existing persona sections.
internal/persona/loader.go:44 — WIRE: `NewPersonaLoader` now chains through `NewPersonaLoaderWithDirs`, so the helper is on the production path.
internal/persona/loader.go:52 — WIRE: `NewPersonaLoaderWithDirs` now chains through `NewPersonaLoaderWithDir`, so the helper is on the production path.
