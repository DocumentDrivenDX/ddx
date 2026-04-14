---
ddx:
  id: ADR-002
  depends_on:
    - helix.prd
    - FEAT-008
---
# ADR-002: Web Application Stack

**Status:** Accepted 2026-04-14
**Date:** 2026-04-14
**Context:** The DDx server web UI (FEAT-008) needs a TypeScript frontend embedded in the Go binary. This ADR standardizes the web stack.

## Decision

Use **SvelteKit** as the frontend framework with **gqlgen** for GraphQL backend, running on **Bun** as the JavaScript runtime. The stack provides end-to-end type safety through GraphQL code generation and first-class E2E testing.

### 1. Backend: gqlgen

**gqlgen** is the GraphQL server library for Go that enables schema-first API development.

- **Schema-first design:** `schema.graphql` is the single source of truth for all API operations
- **Code generation:** `go run github.com/99designs/gqlgen generate` produces strongly-typed Go resolvers
- **Type-safe client:** Houdini generates TypeScript types from the schema for SvelteKit

GraphQL endpoint: `POST /graphql` with GraphiQL IDE at `/graphiql`

REST API remains unchanged for CLI/MCP compatibility; GraphQL is the preferred path forward.

### 2. Frontend: SvelteKit

**SvelteKit** is the official framework for building user interfaces with Svelte.

- **Svelte 5 runes:** `$props()`, `$state()`, `derived` for reactive state
- **adapter-static:** Builds to static files served by Go via `//go:embed`
- **URL scheme:** `/nodes/:nodeId/projects/:projectId/*` — implemented fresh
- **Houdini:** First-class GraphQL client for SvelteKit with typed `load()` functions

Svelte 5 offers:
- Compacted runtime
- Compile-time optimizations (no virtual DOM)
- Simpler mental model for reactivity

### 3. Runtime: Bun

**Bun** replaces Node.js for all frontend tooling:

- **Package manager:** `bun install` with `bun.lock` (text-based, committed to git)
- **Script runner:** `bun run build`, `bun run dev`, `bun run test`, etc.
- **Build tool:** Bun's built-in bundler for production builds
- **Fast startup:** Significantly faster than Node.js/npm

CI setup:
```yaml
- uses: oven-sh/setup-bun@v2
  with: { bun-version: latest }
```

### 4. GraphQL Client: Houdini

**Houdini** is the SvelteKit-native GraphQL client:

- **Code generation:** TypeScript types generated from `schema.graphql`
- **Typed load():** Automatic query generation for routes
- **Subscriptions:** First-class support for `graphql-ws` subscriptions

Example query in a route:
```ts
// src/routes/nodes/[nodeId]/+page.ts
import { query } from './$houdini';

export function load({ params }) {
  return query(`query { node(id: "${params.nodeId}") { id name } }`);
}
```

### 5. UI Primitives: bits-ui + lucide-svelte

- **bits-ui:** Headless, accessible UI primitives
- **lucide-svelte:** Svelte-compatible icon set
- **mode-watcher:** Dark/light mode toggle

All components use Tailwind for styling.

### 6. Styling: Tailwind CSS

Tailwind provides utility-first styling consistent with the Hugo website:

```js
// tailwind.config.js
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: { extend: {} },
  plugins: [],
}
```

### 7. Testing

| Layer | Tool | What It Tests |
|-------|------|---------------|
| Unit | **Vitest** | Component and utility functions |
| E2E | **Playwright** | Full browser flows against running server |
| Typecheck | **svelte-check** | Svelte component type safety |

Run commands:
```bash
bun run test           # Vitest unit tests
bun run test:e2e       # Playwright e2e tests
bun run check          # svelte-check typechecking
```

Playwright configuration:
```ts
// playwright.config.ts
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: { baseURL: 'http://127.0.0.1:7743', headless: true },
  webServer: {
    command: 'cd cli && go build -o ddx . && ./ddx server',
    port: 7743,
  },
})
```

### 8. TypeScript Configuration

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true,
    "verbatimModuleSyntax": true,
    "noPropertyAccessFromIndexSignature": true,
    "declaration": false,
    "noEmit": true,
    "jsx": "preserve",
    "baseUrl": ".",
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["src/**/*.ts", "src/**/*.svelte"]
}
```

Key choices:
- `"strict": true` — non-negotiable
- `"verbatimModuleSyntax": true` — explicit type imports
- `"moduleResolution": "bundler"` — aligns with bundler-based resolution

### 9. Linting and Formatting: Prettier + eslint-plugin-svelte

- **Prettier:** Opinionated formatting with `prettier-plugin-svelte`
- **ESLint:** Svelte-specific rules via `eslint-plugin-svelte`

Configuration:
```json
{
  "overrides": [
    { "files": ["**/*.{svelte,ts}"], "extends": ["plugin:svelte/recommended"] }
  ]
}
```

### 10. Dependency Management

- **`bun.lock`** committed to git
- **`bun install --frozen-lockfile`** in CI for reproducible builds
- **Security:** Use socket.dev or Snyk for dependency scanning

### 11. Bundle Analysis and Performance

**Bundle size targets:**
- Initial load < 200KB gzipped
- LCP < 2.5s, INP < 200ms, CLS < 0.1

Use `bunx bundle-analyzer` for analysis when needed.

### 12. Deployment

The frontend is built to static files and embedded into the Go binary:

```go
// cli/internal/server/embed.go
//go:embed all:frontend/build
var frontendFS embed.FS
```

Build pipeline:
```makefile
build:
	cd cli/internal/server/frontend && bun install --frozen-lockfile && bun run build
	go build -o ddx ./cli

docker:
	# Build frontend, then embed in Go binary
```

No Node.js runtime required in production — just the static assets.

### 13. CI/CD Integration

```yaml
name: Frontend CI
on: [push, pull_request]
jobs:
  typecheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run check

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run test

  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bunx playwright install --with-deps
      - run: bun run test:e2e

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run build
```

## Consequences

- **End-to-end type safety:** GraphQL schema is source of truth; TypeScript types generated automatically
- **Real-time capabilities:** `graphql-ws` subscriptions for live updates (bead lifecycle, worker progress)
- **Relay cursor connections:** All list operations use modern pagination
- **Single runtime:** Bun replaces Node.js for faster CI and development
- **Static frontend:** SvelteKit builds to static assets; embedded in Go binary

## Alternatives Considered

The following alternatives were evaluated but rejected for this migration:

- **Alternative bundlers:** Larger runtime footprint; optimized for development server rather than static site generation
- **Data fetching libraries:** Unnecessary complexity since Houdini covers both routing and data fetching concerns
- **Alternative package managers:** Bun's integrated tooling is faster and simpler for all frontend operations
- **Service mocking:** Replaced by Houdini's codegen and real GraphQL endpoint; no mocking needed

## Migration Path

See `docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md` for the full four-stage migration plan:
1. Schema + spec lockdown
2. GraphQL backend (gqlgen resolvers)
3. SvelteKit scaffold
4. Pages + tests + release
