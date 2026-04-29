# Concern: TypeScript + Bun

## Category
tech-stack

## Areas
all

## Components

- **Language**: TypeScript (strict mode)
- **Runtime**: Bun 1.x — NOT Node.js
- **Package manager**: Bun (`bun install`, `bun add`) — NOT npm, NOT yarn, NOT pnpm
- **Linter + Formatter**: Biome — NOT ESLint, NOT Prettier
- **Test runner**: `bun:test` — NOT Vitest, NOT Jest
- **Workspace layout**: Bun workspaces (`workspaces` in root `package.json`)
- **TypeScript config**: strict, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`

## Constraints

- All code must pass `tsc --noEmit` (or `bun run typecheck`) with strict config
- All code must pass Biome lint + format check (`bun run lint`)
- Scripts use `bun run`, not `npm run`
- Use Bun-native APIs: `Bun.serve()` for HTTP (not `@hono/node-server` or similar Node adapters), `Bun.file()` for file I/O, `Bun.spawn()` for subprocesses
- Do not use `tsx`, `ts-node`, or other TypeScript transpilers — Bun runs `.ts` natively
- Do not add an `engines.node` field — this project targets Bun, not Node.js
- No `package-lock.json` or `yarn.lock` — use `bun.lock`
- No `node dist/index.js` start commands — use `bun src/index.ts`
- Biome config: indent style tabs, line width 100, `noUnusedImports: error`

## Drift Signals (anti-patterns to reject in review)

- `npm run` in scripts → must be `bun run`
- `prettier` or `eslint` dependencies → replace with Biome
- `vitest` or `jest` → replace with `bun:test`
- `tsx` or `ts-node` → remove; Bun executes TypeScript natively
- `@hono/node-server` or any `*-node-*` HTTP adapter → use `Bun.serve()`
- `node dist/` start command → use `bun src/`
- `engines.node` constraint → remove

## When to use

TypeScript projects using Bun as the runtime and package manager. Applies to
monorepos and single-package projects alike. If a project historically drifted
to Node.js tooling (npm, tsx, prettier, vitest), the concern documents the
target state and the drift signals above identify what needs correction.

## ADR References
