# CLAUDE.md

Developer guidance for DDx command-line interface and frontend development.

## Development Commands

### Frontend Development (SvelteKit)

The web UI is a SvelteKit application built with Bun:

```bash
# Install dependencies and start dev server
cd cli/internal/server/frontend && bun install && bun run dev

# Run unit tests
bun run test

# Run e2e tests with Playwright
bun run test:e2e
```

Frontend build output is embedded into the Go binary.

### CLI Development

```bash
cd cli
make dev      # Start Go development server with air
make test     # Run Go tests
```

## Persona System

DDx personas are project-scoped bindings layered over the plugin lock/cache
topology, not home-directory state.

- The role-to-persona map lives in `.ddx/config.yaml` under
  `persona_bindings`.
- Marketplace plugins resolve from project lock metadata plus payloads under
  `${XDG_DATA_HOME}/ddx/cache/plugins/`; the default `ddx` package can fall
  back to the baked-in binary copy.
- The agent-facing skill outputs live in the project-local
  `.agents/skills/` and `.claude/skills/` directories as generated adapters.
- `ddx doctor --plugins` reports project lock, cache, and adapter state.
- Home-directory/global plugin installs are retired as the forward model; do
  not add new ones.

Plugin and persona lookup follows the project plugin lock and XDG cache for
marketplace plugins, with the baked-in binary default available only for the
`ddx` plugin. See `docs/helix/02-design/adr/ADR-027-skill-install-topology.md`
for the full decision record.

<!-- PERSONAS:START -->
## Active Personas

<!-- PERSONAS:END -->
