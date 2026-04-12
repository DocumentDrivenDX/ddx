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

Use **Bun** as the JavaScript/TypeScript runtime, **Vite** as the dev server and bundler, and **Playwright** for frontend E2E testing. Biome and Vitest are deferred until the frontend stack is ready for a formal migration.

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

### 5. Linting and Formatting: Deferred

Biome is the intended future formatter/linter for the frontend stack, but it is not adopted yet. Keep the current project conventions until a dedicated migration issue updates this ADR and lands the toolchain.

### 6. Dependency Management

- **`bun.lock`** committed to git (text-based since Bun 1.1.39+)
- **`bun install --frozen-lockfile`** in CI for reproducible builds
- **Security:** Use `socket.dev` or Snyk for dependency scanning (Bun's built-in audit is limited)
- **`trustedDependencies`** in package.json to control install scripts

### 7. Testing

**Current layer:**

| Layer | Tool | What It Tests |
|-------|------|--------------|
| E2E | **Playwright** | Full browser flows against running server |

Unit, integration, and component test coverage via Vitest and Testing Library is deferred until the frontend stack grows into a formal migration issue.

**Playwright** for E2E in the current embedded frontend harness:
```ts
// playwright.config.ts
import { defineConfig } from '@playwright/test'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cliRoot = resolve(__dirname, '..', '..', '..')

export default defineConfig({
  testDir: './e2e',
  testIgnore: ['demo-recording.spec.ts'],
  timeout: 30000,
  use: {
    baseURL: 'http://127.0.0.1:18080',
    headless: true,
  },
  webServer: {
    command: `${cliRoot}/build/ddx server --port 18080`,
    cwd: cliRoot,
    port: 18080,
    reuseExistingServer: true,
    timeout: 10000,
  },
})
```

Before running the E2E suite, build the embedded server binary so `webServer`
can launch `${cliRoot}/build/ddx server --port 18080`.

Run tests:
```bash
bun run test:e2e        # E2E
```

### 8. Type Checking

Bun strips types but doesn't check them. Run `tsc` separately:

```bash
bunx tsc --noEmit
```

**In CI**, type checking runs as a separate job — failures block merge:
```yaml
typecheck:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: oven-sh/setup-bun@v2
    - run: bun install --frozen-lockfile
    - run: bunx tsc --noEmit
```

### 9. Bundle Analysis and Performance

**Bundle size enforcement** is deferred until the frontend adopts a dedicated budget tool.

**Web Vitals** measurement:
- `web-vitals` package reports LCP, INP, CLS to the server or analytics
- Core Web Vitals thresholds: LCP < 2.5s, INP < 200ms, CLS < 0.1

**Lighthouse CI** for automated performance audits is deferred until a tracked performance budget workflow lands.

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
    cd cli/internal/server/frontend && bun install --frozen-lockfile && bun run build
    cd cli/internal/server && go build -o ddx-server .
```

**Docker** (when needed):
```dockerfile
FROM oven/bun:1 AS frontend
WORKDIR /app/cli/internal/server/frontend
COPY cli/internal/server/frontend/package.json cli/internal/server/frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY cli/internal/server/frontend/ .
RUN bun run build

FROM golang:1.26 AS backend
WORKDIR /app
COPY . .
COPY --from=frontend /app/cli/internal/server/frontend/dist cli/internal/server/frontend/dist
RUN cd cli/internal/server && go build -o /ddx-server .

FROM gcr.io/distroless/base
COPY --from=backend /ddx-server /
CMD ["/ddx-server"]
```

### 12. CI/CD Integration

Biome is deferred for this frontend stack, so the current CI example covers
type checking, the embedded-server Playwright path, and builds only.

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
      - run: bunx tsc --noEmit

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - uses: actions/setup-go@v5
        with:
          go-version-file: cli/go.mod
      - run: cd cli && make build
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

- Single runtime (Bun) for all frontend tooling — simpler CI, faster installs
- Linting and unit/component testing migrations remain deferred until a dedicated frontend tooling update
- Playwright provides the current browser-flow verification surface
- `tsc --noEmit` remains mandatory in CI despite Bun's type stripping
- Frontend embeds into Go binary — single binary deployment maintained
- Vite dev server with Bun gives fast DX; production builds embed cleanly

## Alternatives Considered

- **Node.js runtime:** Mature but slower. Bun's speed advantages compound in CI and development.
- **ESLint + Prettier:** More configurable, but the current project conventions stay in place until the deferred migration is revisited.
- **`bun test` for frontend:** Less mature DOM environment support than a future Vitest-based setup, so it remains out of scope for now.
- **SolidJS:** Smaller bundle, faster runtime. Chose React for ecosystem breadth. Can revisit if bundle size is a problem.
- **Svelte:** Excellent DX but smaller ecosystem. Not ideal when we need rich graph visualization libraries.
