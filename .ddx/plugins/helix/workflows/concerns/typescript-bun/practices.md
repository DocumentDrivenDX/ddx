# Practices: typescript-bun

## Requirements (Frame phase)
- All user stories involving TypeScript must assume Bun as runtime and package manager
- If a library dependency requires a Node.js adapter, flag it as a concern at framing — it may require a Bun-compatible alternative

## Design
- Use Bun workspaces for monorepos: `"workspaces": ["packages/*"]` in root `package.json`
- Separate packages by concern: `shared` (types/schemas), `server` (API), `web` (frontend)
- Use workspace references (`workspace:*`) for cross-package dependencies
- HTTP servers: `Bun.serve()` — not Express, Fastify with Node adapter, or `@hono/node-server`
- For Hono: use `hono` directly with `Bun.serve()` export, not the node-server adapter

## Implementation
- Run TypeScript directly: `bun src/index.ts` — no build step required for server/CLI
- Scripts in `package.json` must use `bun run` / `bun test` / `bun add`, not `npm run`
- Use Bun-native APIs:
  - File I/O: `Bun.file()`, `Bun.write()`
  - Subprocesses: `Bun.spawn()`, `Bun.spawnSync()`
  - HTTP: `Bun.serve()`
  - Environment: `Bun.env`
- TypeScript config: `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `verbatimModuleSyntax`
- No `any` — TypeScript strict mode is enforced
- Formatting: Biome with tabs, line width 100
- Linting: Biome recommended rules + `noUnusedImports: error`, `noUnusedVariables: warn`
- Imports: use `type` keyword for type-only imports (`import type { Foo }`)

## Testing
- Framework: `bun:test` (built-in)
- Run: `bun test`
- Use `mock()` from `bun:test` for module mocking
- Fake data: `@faker-js/faker` or equivalent — not static fixtures
- Prefer stubs to mocks; verify behavior, not call sequences
- Integration tests can use real databases via `docker compose up -d` or testcontainers

## Quality Gates (pre-commit / CI)
- `bun test` — all tests pass
- `bun run typecheck` — `tsc --noEmit` passes for all packages
- `bun run lint` — Biome lint + format check passes
- No `package-lock.json` committed (indicates npm was used)
- `bun.lock` committed and up to date

## Dependency Management
- Add: `bun add <pkg>` (not `npm install`)
- Dev deps: `bun add -d <pkg>`
- Workspace deps: reference with `"workspace:*"` in package.json
- Lock file: `bun.lock` (text format, committed)
- Audit: `bun audit` for known vulnerabilities
