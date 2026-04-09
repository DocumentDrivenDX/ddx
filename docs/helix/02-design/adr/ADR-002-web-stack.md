---
ddx:
  id: ADR-002
  depends_on:
    - helix.prd
    - FEAT-008
---
# ADR-002: Web Application Stack

**Status:** Accepted
**Date:** 2026-04-04
**Context:** The DDx server web UI (FEAT-008) needs a TypeScript frontend embedded in the Go binary. This ADR standardizes the web stack.

## Decision

Use **Bun** as the JavaScript/TypeScript runtime, **Vite** as the dev server and bundler, **Biome** for linting and formatting, and **Vitest** + **Playwright** for testing.

### 1. Runtime: Bun

Bun replaces Node.js for all frontend tooling: package management, script execution, test running, and TypeScript transpilation.

- **Package manager:** `bun install` with `bun.lock` (text-based, committed to git)
- **Script runner:** `bun run build`, `bun run dev`, etc.
- **TypeScript:** Bun strips types natively — no build step for running TS scripts
- **Bun does NOT type-check** — `tsc --noEmit` runs separately in CI

CI setup:
```yaml
- uses: oven-sh/setup-bun@v2
  with: { bun-version: latest }
```

### 2. Bundler and Dev Server: Vite

Vite handles the development server (with HMR) and production builds. Run Vite under Bun for faster startup:

```bash
bun --bun vite          # dev server
bun --bun vite build    # production build to dist/
```

Vite's dev server proxies API requests to the Go backend:
```ts
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/mcp': 'http://localhost:8080',
    }
  }
})
```

Production output embeds into the Go binary via `embed.FS`.

### 3. Framework: React

React for the UI framework. It has the largest ecosystem, best tooling support, and most hiring pool. SolidJS is a viable alternative if bundle size becomes critical.

Key libraries:
- **TanStack Router** — type-safe file-based routing
- **TanStack Query** — data fetching with caching and refetching
- **Tailwind CSS** — consistent with the Hugo website
- **D3.js** or **Cytoscape.js** — document dependency graph visualization

### 4. TypeScript Configuration

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
    "jsx": "react-jsx",
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  },
  "include": ["src"]
}
```

Key choices:
- `"strict": true` — non-negotiable
- `"noUncheckedIndexedAccess": true` — catches array/object access bugs
- `"verbatimModuleSyntax": true` — enforces explicit `type` imports
- `"moduleResolution": "bundler"` — aligns with Vite's resolution

### 5. Linting and Formatting: Biome

Use **Biome** (not ESLint/Prettier) — single tool for both linting and formatting, 100x faster, lower config surface.

```json
// biome.json
{
  "$schema": "https://biomejs.dev/schemas/1.9.0/schema.json",
  "organizeImports": { "enabled": true },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true,
      "correctness": {
        "noUnusedImports": "error",
        "noUnusedVariables": "error"
      },
      "suspicious": {
        "noExplicitAny": "error"
      },
      "style": {
        "useConst": "error",
        "noNonNullAssertion": "warn"
      }
    }
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "tab",
    "lineWidth": 100
  }
}
```

CI: `bunx biome check .` (lint + format in one command, exit non-zero on violations).

### 6. Dependency Management

- **`bun.lock`** committed to git (text-based since Bun 1.1.39+)
- **`bun install --frozen-lockfile`** in CI for reproducible builds
- **Security:** Use `socket.dev` or Snyk for dependency scanning (Bun's built-in audit is limited)
- **`trustedDependencies`** in package.json to control install scripts

### 7. Testing

**Three layers:**

| Layer | Tool | What It Tests |
|-------|------|--------------|
| Unit + Integration | **Vitest** | Components, hooks, utilities, API client logic |
| Component | **Testing Library** (`@testing-library/react`) | User-facing component behavior |
| E2E | **Playwright** | Full browser flows against running server |

**Vitest** (not `bun test`) for the web frontend because of better DOM environment integration (jsdom/happy-dom), Vite-native config sharing, and richer plugin ecosystem.

```ts
// vitest.config.ts
import { defineConfig } from 'vitest/config'
export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      thresholds: {
        statements: 70,
        branches: 60,
        functions: 70,
        lines: 70,
      }
    }
  }
})
```

**Playwright** for E2E:
```ts
// playwright.config.ts
import { defineConfig } from '@playwright/test'
export default defineConfig({
  webServer: {
    command: 'bun run dev',
    port: 5173,
  },
  use: {
    baseURL: 'http://localhost:5173',
  },
})
```

Run tests:
```bash
bun run vitest              # unit + component tests
bun run vitest --coverage   # with coverage
bunx playwright test        # E2E
```

### 8. Type Checking

Bun strips types but doesn't check them. Run `tsc` separately:

```bash
bun run typecheck   # package.json: "typecheck": "tsc --noEmit"
```

**In CI**, type checking runs as a separate job — failures block merge:
```yaml
typecheck:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: oven-sh/setup-bun@v2
    - run: bun install --frozen-lockfile
    - run: bun run typecheck
```

### 9. Bundle Analysis and Performance

**Bundle size enforcement** with `size-limit`:
```json
// package.json
"size-limit": [
  { "path": "dist/assets/*.js", "limit": "200 kB" }
]
```

**Web Vitals** measurement:
- `web-vitals` package reports LCP, INP, CLS to the server or analytics
- Core Web Vitals thresholds: LCP < 2.5s, INP < 200ms, CLS < 0.1

**Lighthouse CI** for automated performance audits:
```yaml
- run: bunx @lhci/cli autorun
```

### 10. Observability and Error Tracking

**Error tracking:** **Sentry** (`@sentry/react`) — catches unhandled errors, provides session replay, performance monitoring.

**Frontend telemetry:** OpenTelemetry for browser:
- `@opentelemetry/sdk-trace-web` — distributed tracing
- `@opentelemetry/instrumentation-fetch` — auto-instrument API calls
- Traces correlate with Go backend traces via `traceparent` header

**Structured logging:** Console-based in development, JSON to server endpoint in production.

For v1, Sentry alone is sufficient. Add OpenTelemetry when distributed tracing across Go server + frontend becomes necessary.

### 11. Deployment

The frontend is embedded into the Go binary via `embed.FS`. No separate deployment.

**Build pipeline:**
```makefile
build:
    cd frontend && bun install --frozen-lockfile && bun run build
    cd server && go build -o ddx-server .
```

**Docker** (when needed):
```dockerfile
FROM oven/bun:1 AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ .
RUN bun run build

FROM golang:1.26 AS backend
WORKDIR /app
COPY --from=frontend /app/frontend/dist server/frontend/dist
COPY . .
RUN cd server && go build -o /ddx-server .

FROM gcr.io/distroless/base
COPY --from=backend /ddx-server /
CMD ["/ddx-server"]
```

### 12. CI/CD Integration

```yaml
name: Frontend CI
on: [push, pull_request]
jobs:
  lint-format:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bunx biome check .

  typecheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run typecheck

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run vitest --coverage
      - run: bunx playwright install --with-deps
      - run: bunx playwright test

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: bun run build
      - run: bunx size-limit
```

## Consequences

- Single runtime (Bun) for all frontend tooling — simpler CI, faster installs
- Biome replaces ESLint + Prettier — one tool, faster, less config
- Three-layer testing (Vitest + Testing Library + Playwright) covers all verification needs
- `tsc --noEmit` remains mandatory in CI despite Bun's type stripping
- Frontend embeds into Go binary — single binary deployment maintained
- Vite dev server with Bun gives fast DX; production builds embed cleanly

## Alternatives Considered

- **Node.js runtime:** Mature but slower. Bun's speed advantages compound in CI and development.
- **ESLint + Prettier:** More configurable but two tools, slower, more config. Biome covers our needs.
- **`bun test` for frontend:** Less mature DOM environment support. Vitest is better for React component testing.
- **SolidJS:** Smaller bundle, faster runtime. Chose React for ecosystem breadth. Can revisit if bundle size is a problem.
- **Svelte:** Excellent DX but smaller ecosystem. Not ideal when we need rich graph visualization libraries.
