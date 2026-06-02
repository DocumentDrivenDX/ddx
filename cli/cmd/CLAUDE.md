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

DDx personas are project-scoped bindings layered over the skill install
topology, not home-directory state.

- The role-to-persona map lives in `.ddx/config.yaml` under
  `persona_bindings`.
- The default `ddx` package resolves project-local installs first,
  then the global fallback under `${XDG_DATA_HOME}/ddx/global/plugins/ddx`,
  then the baked-in default package.
- The agent-facing skill outputs live in the project-local
  `.agents/skills/` and `.claude/skills/` directories.
- `ddx doctor` reports whether the project install exists or is falling
  through to the global layer.
- Home-directory skill installs are retired; do not add new ones.

<!-- PERSONAS:START -->
## Active Personas

<!-- PERSONAS:END -->
