---
ddx:
  id: ddx-concerns
  depends_on:
    - helix.prd
---
# Project Concerns

## Active Concerns

- go-std (tech-stack, cli/api)
- typescript-bun (tech-stack, ui)
- e2e-playwright (testing, ui/site)
- frontend-design (design-system, ui)
- hugo-hextra (microsite, site)
- demo-asciinema (demo, all)
- security-owasp (security, all)
- testing (quality-attribute, all)

## Area Labels

| Label | Applies to |
|-------|-----------|
| `all` | Every bead |
| `cli` | Core DDx binary, commands, internal packages |
| `api` | Server HTTP/MCP endpoints |
| `agent` | DDx agent runtime, execution, routing, and migration work |
| `bead` | Bead tracker maintenance, bead-governance reviews, and bead-scoped workflow contract work |
| `ui` | Web UI frontend (SvelteKit embedded in Go binary via embed.FS) |
| `site` | website/, microsite content and deployment |
| `docs` | Governing docs, plans, reviews, and alignment reports |
| `infra` | CI, Docker, release infrastructure |

Bead-scoped work uses `area:bead` as an additive label, not a replacement for any domain-specific area labels. A bead should still carry the broader labels that match the work it governs so concern matching remains explicit.

## Project Overrides

### go-std

- **Source**: Go source is in `cli/` within this monorepo
- **CLI framework**: Cobra with CommandFactory pattern
- **Testing**: `go test` with acceptance, integration, contract, and E2E smoke tests
- **Linter**: golangci-lint v2 with project `.golangci.yml`
- **Build**: `make build` in `cli/`, multi-platform via `make build-all`
- **Pre-commit**: lefthook runs `go fmt`, `go vet`, `golangci-lint`, `go test`

### typescript-bun

- **Build tool**: SvelteKit (not Bun's native bundler) per ADR-002. SvelteKit provides
  hot module replacement and the plugin ecosystem for the embedded SPA.
- **Test runner**: Playwright for E2E (not `bun:test`). Tests need the Go
  server running.
- **Formatter**: Project uses existing SvelteKit/TypeScript conventions
  (prettier-plugin-svelte + eslint-plugin-svelte). Biome migration deferred.
- **Package manager**: Bun for install and scripts (`bun install`, `bun run build`).
- **Lockfile**: `bun.lock` committed (not `package-lock.json`).
- **Runtime**: Frontend is browser-only (SvelteKit embedded in Go binary via
  `embed.FS`). Bun-native server APIs (`Bun.serve()`) do not apply.
- **Data layer**: GraphQL with graphql-request + graphql-ws for typed queries and real-time subscriptions per ADR-002 v2.
- **Location**: `cli/internal/server/frontend/`

### e2e-playwright

Three Playwright configurations, each deliberately scoped to a
different backend so tests stay decoupled:

1. **Frontend functional + visual regression** (`cli/internal/server/frontend/playwright.config.ts`):
   - Tests in `cli/internal/server/frontend/e2e/`
   - Screenshots in `e2e/*.spec.ts-snapshots/`
   - Runs against the **SvelteKit static preview** on port 4173
     (`bun run build && bun run preview`). The preview serves the same
     static bundle embedded into the Go binary; GraphQL is mocked at
     the request layer in each spec.
   - Rationale: functional + visual tests should not depend on a live
     Go server or real GraphQL data. Request-route interception keeps
     snapshots deterministic.

2. **Demo recording** (`cli/internal/server/frontend/playwright.demo.config.ts`):
   - `demo-recording.spec.ts` navigates all 5 pages with interactions
   - Runs against the **Go server** on port 18080 (real binary, real
     data) to produce representative videos.
   - Video recording at 1280x720
   - Output in `demo-output/`
   - Run via `bun run demo:record`

3. **Microsite** (`website/playwright.config.ts`):
   - Tests in `website/e2e/microsite.spec.ts`
   - Screenshots for homepage, docs, navigation
   - Runs against `hugo server` on the configured port

### frontend-design

Cross-cutting design and accessibility concern for every UI surface DDx ships
(the SvelteKit web UI in `cli/internal/server/frontend/` and any future
embedded views). Applies to every feature that touches the UI — individual
feature specs do not restate its rules.

**Authoritative guide for aesthetic/UX judgement**

The installed `anthropics/skills@frontend-design` skill is the authoritative
guide for creating and reviewing UI surfaces under this concern. When a
feature requires a design judgement (layout, typography, interaction
affordance, visual hierarchy), delegate the judgement to that skill; the
concern below captures the non-negotiable constraints the skill's output
must satisfy.

**Palette discipline**

- Colors reference semantic tokens (e.g. `status.open`, `status.blocked`,
  `priority.p0`, `tier.cheap`) centralized in Tailwind config and/or
  `$lib/stores/theme.ts`. Raw hex in component markup is a drift signal;
  CI greps for `#[0-9a-fA-F]{3,8}` outside the theme files and fails.
- Every colored state has an accompanying non-color cue (text label, icon,
  pattern). Status conveyed by color alone is a violation.
- Status/priority/tier vocabularies use the SAME token across all pages so
  a "blocked" bead and a "blocked" worker look identical.

**Dark / light parity**

- Every page is legible in both dark and light modes without hover, focus,
  or menu interaction. The top-nav theme toggle switches modes; choice
  persists to localStorage and is bookmarkable via `?theme=dark|light`.
- Visual regression: `screenshots.spec.ts` captures EACH page twice —
  once per theme. Threshold-gated diffs catch palette drift.

**Accessibility floor**

- WCAG AA: 4.5:1 contrast for normal text, 3:1 for large text, in both
  modes.
- Every interactive control reachable by keyboard; focus rings visible
  in both themes.
- `@axe-core/playwright` runs against every page; CI fails on any
  `critical` or `serious` violation. Lane: `bun run test:a11y`.

**Quality gates (CI-enforced)**

| Gate | Command | Frequency |
|------|---------|-----------|
| Palette discipline grep | pre-commit lefthook check on `cli/internal/server/frontend/src/**/*.{svelte,ts}` | per commit |
| Axe-core page scans | `bun run test:a11y` | CI + pre-push |
| Dark/light visual regression | `playwright screenshots.spec.ts` with theme matrix | CI |
| Keyboard-reachability smoke | Playwright `tab` through interactive controls on each page | CI |

**Drift signals (alignment-review fodder)**

- Raw hex color in a component (grep miss) → palette discipline broken.
- Page visible only in one theme (manual reviewer's report; CI catches
  via visual-regression diff) → dark/light parity broken.
- A11y scan surfaces a new `critical` violation → blocking; not a warning.
- A user story's acceptance criterion restates "must work in dark mode"
  or "must be WCAG-AA" → the concern is leaking into feature specs;
  remove the restatement.

### hugo-hextra

- **Theme version**: Hextra pinned in `website/go.mod`
- **Deployment**: GitHub Pages at `DocumentDrivenDX.github.io/ddx/`
- **Custom shortcode**: `asciinema.html` for terminal recording embeds
- **E2E tests**: Playwright tests in `website/e2e/`

### demo-asciinema

- **Recording environment**: Docker containers for reproducibility. All
  recording tools (asciinema, agg) are containerized.
- **Demo scripts**: `scripts/demos/` with deterministic shell scripts
- **Embedding**: asciinema shortcode loads player from CDN, plays `.cast`
  files from `website/static/demos/`
- **Agent credentials**: Demos invoking AI agents mount `~/.claude.json` (ro)
  and `~/.claude/` (rw) from host into container.

### security-owasp

- **Dependency audit**: `govulncheck ./...` for Go, `bun audit` for frontend
- **Secret scanning**: lefthook pre-commit hook via gitleaks
- **Path traversal**: Server validates all document paths, rejects `..`
- **Localhost guard**: All write and dispatch endpoints reject non-localhost requests
- **Network auth**: ts-net (Tailscale) for authenticated remote access per ADR-006.
  No custom API keys. Identity from tailnet membership.

### testing

Inherits the full HELIX `testing` concern (see `workflows/concerns/testing/concern.md`). The rules below are DDx-specific applications of those principles, not replacements.

**Tooling**

- **Go tests**: Acceptance, integration, contract, E2E smoke in `cli/cmd/`
  and `cli/internal/*/`
- **Frontend tests**: Playwright E2E against running server
- **Pre-commit**: lefthook runs full Go test suite on changed packages
- **CI**: GitHub Actions runs `lefthook run ci` + E2E smoke tests

**No mocks, period — applied to DDx**

The HELIX concern says: *"Use real implementations where practical… Mock only at true system boundaries… Never mock the thing you are testing."* In DDx that becomes an absolute rule:

**Integration tests: zero mocks.** No exceptions. Integration tests exercise real production implementations of every dependency in the graph — real git (`t.TempDir()` + scrubbed `GIT_*` env), real filesystem, real bead store, real coordinator, real server, real agent harness (the `script` harness below is the only approved fake for the AI-provider boundary, and it does not mock — it performs real filesystem and git operations deterministically). A test that substitutes any mock for a production component is not an integration test. It is a unit test that is lying about its coverage.

**Unit tests: stubs, not mocks, and only at your own domain interfaces.** When a unit test needs to isolate the code under test from a dependency, the approved pattern is:

1. Define the dependency as an interface (e.g., `bead.Store`, `agent.ExecuteBeadLoopStore`, `agent.LandingGitOps`).
2. Write an **in-memory stub** that implements the interface — not a call-recording mock.
3. The stub must pass the SAME contract test suite as the production implementation. When both implementations pass the same tests, they are proven interchangeable. This is the "contract test for the stub" pattern: you write one suite of tests against the interface, and run it against both the in-memory stub and the real backend. Either both pass or both fail.
4. Unit tests wire the code under test with the in-memory stub, not with a mock. Assertions are on behavior (did the right bead close?), not call sequence (was `Close` called exactly once with these args?).

`testify/mock` is not used in new tests. `gomock` is not used. Call-sequence assertions are not used. If a test relies on asserting that method X was called Y times with argument Z, it is testing implementation details, not behavior — delete or rewrite.

**Never mock the thing you are testing.** A test of the land coordinator that substitutes a fake `LandingGitOps` is mocking the thing under test. Coordinator tests use `RealLandingGitOps` against a real git repo. Same for execute-bead, gate evaluation, merge logic, preserve-ref writes, and every other orchestration concern.

**The only approved fake in the DDx test suite is the `script` harness.** It is the *AI-provider* boundary replacement, and it is not a mock — it is a deterministic alternate implementation of the `agent.Runner` interface that reads a line-based directive file and performs real filesystem + git operations against the worktree. The `virtual` harness is a replay-only fake retained for prompt-routing unit tests; it does not write files or make commits.

**Contract-test pattern for new interfaces.** When you introduce a new domain interface with an in-memory stub:

```go
// contract_test.go — runs the same suite against both implementations
func TestStoreContract(t *testing.T) {
    cases := []struct{ name string; build func(t *testing.T) Store }{
        {"memory", func(t *testing.T) Store { return NewMemoryStore() }},
        {"file",   func(t *testing.T) Store { return NewFileStore(t.TempDir()) }},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            s := c.build(t)
            // one set of test cases, run against both implementations
            t.Run("CreateAndGet", func(t *testing.T) { /* ... */ })
            t.Run("UpdateLosesNoFields", func(t *testing.T) { /* ... */ })
            t.Run("ConcurrentWritersSerialized", func(t *testing.T) { /* ... */ })
        })
    }
}
```

If the stub passes the contract suite but the real backend fails, either the backend is broken or the stub is lying. Either way the contract suite exposes the drift immediately.

**DDx-specific drift signals (extend HELIX's list)**

- `orchTestGitOps`, `fakeExecuteBeadGit`, `gateTestGitOps`, or any struct that implements a git-shaped interface for tests → replace with real git in temp repos. Existing instances are legacy and must be migrated as the tests around them are touched.
- `TestConcurrentWorkers*` tests that instantiate one mock git per goroutine → the shared-writer invariant is not being tested. Replace with real git + the `script` harness.
- `testify/mock` or `gomock` for anything other than third-party SDK boundaries → use stubs or real implementations.
- Unit tests that claim to cover "merge" or "rebase" or "land" logic without running `git log` or `git rev-list` → broken test.
- `--compare` / comparison harness tests using `mockExecutor` without a real temp repo → legacy from FEAT-019, flag for migration.

**Canonical fakes and helpers**

- `cli/internal/agent/script.go` — deterministic `script` harness (the fake AI agent)
- `cli/internal/agent/integration_helper_test.go` — `newScriptHarnessRepo(t, beadCount)` helper for integration tests
- `cli/internal/git/git_basic_test.go` — `runGitInDir` helper for scrubbed-env git subprocesses
- `cli/internal/bead/store_test.go` — pattern for real bead store in temp dir

## ADR References

| Concern | ADR | Decision |
|---------|-----|----------|
| typescript-bun | ADR-002 | Web stack: Bun + SvelteKit + Tailwind |
| typescript-bun | ADR-005 | GraphQL + graphql-request client-side data layer for beads UI |
| security-owasp | ADR-003 | Package integrity via commit SHA + tree hash |
| security-owasp | ADR-004 | Bead-backed collections for runtime storage |
| security-owasp | ADR-006 | ts-net for network authentication (no API keys) |
