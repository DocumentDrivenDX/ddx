# Project Concerns

## Active Concerns
- typescript-bun (tech-stack)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `api` | HTTP server, endpoints |

## Project Overrides

### typescript-bun
- **HTTP framework**: raw `Bun.serve()` — no Express, no Fastify
- **Test framework**: `bun:test` — do not use Vitest or Jest
- **Linter/formatter**: Biome — do not use ESLint or Prettier
