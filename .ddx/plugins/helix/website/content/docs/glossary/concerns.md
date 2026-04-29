---
title: Concerns
weight: 4
prev: /docs/glossary/actions
next: /docs/glossary/concepts
---

<!-- AUTO-GENERATED from workflows/concerns/ — do not edit manually -->
<!-- Regenerate with: bash website/scripts/generate-concerns.sh -->

# Concerns

Concerns are composable cross-cutting declarations from a shared library. They encode technology choices, quality requirements, and conventions that apply across multiple beads and phases.

## How Concerns Work

1. **Select** — During [Frame](/docs/glossary/phases#phase-1-frame), declare active concerns in `docs/helix/01-frame/concerns.md`
2. **Filter** — Each concern declares which areas it applies to (`all`, `ui`, `api`, `data`, `infra`, `cli`)
3. **Inject** — At execution time, area-matched concerns and their practices are loaded into context
4. **Digest** — [Context digests](/docs/glossary/concepts#context-digest) carry concern practices into beads, making them self-contained

## Project Concerns File

Every HELIX project declares its concerns in `docs/helix/01-frame/concerns.md`:

```markdown
# Project Concerns

## Active Concerns
- rust-cargo (tech-stack)
- security-owasp (security)

## Area Labels
| Label | Applies to |
|-------|-----------|
| all   | Every bead |
| api   | Server, endpoints |
| cli   | CLI tool |

## Project Overrides
### rust-cargo
- **MSRV**: 1.75 (lower than library default)
```

Project overrides take full precedence over library defaults.

## Concern Library

The library lives at `workflows/concerns/`. Each concern has two files:

- `concern.md` — Category, areas, components, constraints, quality gates
- `practices.md` — Phase-specific practices (requirements, design, implementation, testing)

### Tech Stack Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **go-std** | all | Go (version pinned in `go.mod`) |
| **python-uv** | all | Python 3.12+ |
| **react-nextjs** | web, ui | React 19 — functional components and hooks only |
| **rust-cargo** | all | Rust (latest stable; MSRV pinned in `rust-toolchain.toml`) |
| **scala-sbt** | all | Scala 2.x (pinned per project) |
| **typescript-bun** | all | TypeScript (strict mode) |

### Security Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **security-owasp** | all | OWASP Top 10 (current edition) |

### Observability Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **o11y-otel** | api, backend, infra | OpenTelemetry (traces, metrics, logs) |

### Accessibility Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **a11y-wcag-aa** | ui, frontend | WCAG 2.1 Level AA |

### Internationalization Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **i18n-icu** | ui, frontend | ICU MessageFormat |

### Quality Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **testing** | all | Tests exist to find bugs in the code, not to prove it works |
| **ux-radix** | ui, frontend | Radix UI (headless, accessible by default) |

### Testing Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **e2e-kind** | api, infra | kind (Kubernetes in Docker) |
| **e2e-playwright** | ui, site | Playwright (`@playwright/test`) |

### Infrastructure Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **k8s-kind** | infra | `kind` (Kubernetes in Docker) — NOT docker-compose |

### Tooling Concerns

| Concern | Areas | Key Tools |
|---------|-------|-----------|
| **demo-asciinema** | all | Asciinema (`asciinema rec`) — terminal session recording |
| **demo-playwright** | ui, frontend | Playwright — headless browser automation with video capture |
| **hugo-hextra** | all | Hugo (extended edition) |


## Drift Signals

Tech-stack concerns can declare **drift signals** — patterns that indicate the project is straying from its declared technology choices. For example, the `typescript-bun` concern flags:

- `npm run` instead of `bun run`
- `prettier` or `eslint` instead of Biome
- `vitest` or `jest` instead of `bun:test`
- `@hono/node-server` instead of `Bun.serve()`
- `engines.node` in package.json

[Review](/docs/glossary/actions#review) and [align](/docs/glossary/actions#align) check for drift signals and report them as findings.

## Concerns in the Knowledge Chain

Concerns connect to other HELIX artifacts in a knowledge chain:

```
Spike/POC (gather evidence)
  → ADR (record decision with rationale)
    → Concern (index for context assembly)
      → Context Digest (injected into beads)
```

When a referenced ADR is superseded, [polish](/docs/glossary/actions#polish) flags the affected concern for re-evaluation.

## Where Concerns Are Used

Every HELIX action that involves technology or quality choices loads active concerns:

| Action | How it uses concerns |
|--------|---------------------|
| **build** | Loads practices, runs concern-declared quality gates |
| **review** | Checks for drift signals and practice violations |
| **design** | Concerns constrain architecture decisions |
| **evolve** | Detects technology changes conflicting with concerns |
| **align** | Flags concern drift across all layers (docs, designs, code) |
| **polish** | Enforces area labels, refreshes context digests, fixes tool references |
| **frame** | Concern selection happens during framing |
| **check** | Detects missing area labels, stale digests, missing concerns.md |
| **backfill** | Discovers concerns from project evidence |
