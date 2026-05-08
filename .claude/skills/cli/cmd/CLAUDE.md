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

<!-- PERSONAS:START -->
## Active Personas

<!-- PERSONAS:END -->
