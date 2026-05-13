---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
    - FEAT-003
---
# Feature Specification: FEAT-006 — Concerns, Practices, and Context-Digest Beads

**Feature ID**: FEAT-006
**Status**: Draft
**Priority**: P1
**Owner**: HELIX maintainers

## Overview

HELIX agents make technology choices and follow conventions inconsistently
because nothing in the workflow encodes what the project cares about across
features — its technology stack, accessibility requirements, observability
strategy, or internationalization approach. This feature introduces
**concerns** (composable cross-cutting declarations), **practices**
(conventions that activate per-concern), and **context digests** (compact
authority summaries assembled into beads at triage time so implementing
agents rarely need to read upstream files).

This feature extends the cross-cutting injection model established by
FEAT-003 (principles). Where principles guide **judgment**, concerns guide
**choices and conventions** across every phase.

## Problem Statement

- **Current situation**: Beads carry `spec-id`, `acceptance`, `description`,
  and `design` — but none of these systematically include the active
  technology stack, tooling conventions, accessibility requirements,
  observability patterns, or relevant ADR decisions. Implementing agents
  must reconstruct the full authority context by reading 5-10 upstream
  files. They frequently skip files, read stale versions, or make choices
  inconsistent with prior decisions.
- **Pain points**:
  1. Stack drift — one agent chooses Jest, another chooses Vitest, because
     nothing declares the project's testing framework.
  2. Convention inconsistency — agents don't know to use Biome instead of
     Prettier, or clippy instead of manual linting, because tooling choices
     aren't injected.
  3. Context reconstruction cost — every implementation pass spends tokens
     reading upstream docs that could have been summarized once at triage.
  4. ADR amnesia — architecture decisions are made via spikes and ADRs but
     never flow into the beads that should honor them.
  5. Quality attribute blindness — cross-cutting concerns like a11y, i18n,
     o11y, and security have requirements that should shape every feature
     but are only remembered when someone explicitly asks.
- **Desired outcome**: A bead created by triage or evolve is a self-contained
  execution unit. The implementing agent can work from the bead alone without
  reading upstream files in the common case. Active concerns, practices,
  principles, and relevant ADRs are summarized into a compact context digest
  assembled at bead creation and refreshed at polish time.

## Design

### Cross-cutting context taxonomy

HELIX recognizes two layers of cross-cutting context:

| Layer | What it is | Scope | Example |
|-------|-----------|-------|---------|
| **Principles** | Values that guide judgment | Universal (project-wide) | "Design for simplicity", "Tests first", "Local-first UX" |
| **Concerns** | Composable declarations with associated practices | Per-project, area-scoped, from library | "typescript-bun", "a11y-wcag-aa", "o11y-otel", "i18n-icu" |

Principles are governed by FEAT-003. This feature governs concerns,
practices, and the context digest that wires everything into beads.

Engineering principles ("tests first", "fake data over fixtures", "prefer
stubs to mocks", "cloud native", "local-first UX") live in the principles
document alongside design principles per FEAT-003.

### What is a concern?

A **concern** is a named, composable cross-cutting declaration. Concerns
cover several categories:

| Category | Examples | Nature |
|----------|----------|--------|
| **Tech stack** | typescript-bun, python-uv, rust-cargo | Runtime, language, package manager |
| **Infrastructure** | gcp-cloud-run, aws-lambda, k8s | Where it deploys |
| **Data** | postgres, redis, sqlite | Storage and query layer |
| **Accessibility** | a11y-wcag-aa | Compliance and UX inclusion |
| **Internationalization** | i18n-icu | Localization and message format |
| **Observability** | o11y-otel | Logging, metrics, tracing |
| **Security** | security-owasp | Security posture and practices |
| **UX/Design** | design-system-radix | Component library and patterns |

All categories use the same structure and machinery — the library doesn't
distinguish between a tech stack and an a11y concern. The distinction is
in the **area scope** (see below).

### Concern structure

Concerns live in a library and are selected by the project:

```
workflows/concerns/              # HELIX default library
  typescript-bun/
    concern.md                   # what it is, area scope, ADR refs
    practices.md                 # conventions that follow
  a11y-wcag-aa/
    concern.md
    practices.md
  o11y-otel/
    concern.md
    practices.md
  i18n-icu/
    concern.md
    practices.md
```

A `concern.md` declares:

```markdown
# Concern: TypeScript + Bun

## Category
tech-stack

## Areas
all

## Components
- **Language**: TypeScript (strict mode)
- **Runtime**: Bun 1.x
- **Package manager**: Bun (not npm, not yarn)

## Constraints
- All code must pass `tsc --noEmit` with strict config
- Use Bun-native APIs where available

## ADR References
- [List ADRs that justify this concern's selection]
```

### Area scoping

Each concern declares which **areas** it applies to. This determines which
beads get the concern injected into their context digest:

| Areas value | Meaning | Example |
|-------------|---------|---------|
| `all` | Applies to every bead | Tech stacks, security |
| `ui` | Only beads with `area:ui`, `area:frontend`, or UI-related work | a11y, i18n, design system |
| `api`, `backend` | Only beads with matching area labels | o11y (mostly backend) |
| `data` | Only beads with data-layer area labels | Data concerns |
| Comma-separated list | Multiple area matches | `ui,api` |

The digest assembler matches a bead's `area:*` labels against each concern's
declared areas. A concern with `areas: all` always matches. A concern with
`areas: ui` only matches beads labeled `area:ui`, `area:frontend`, etc.

This prevents every bead from getting every concern — an API backend bead
doesn't need the a11y practices, and a database migration bead doesn't need
the design system conventions.

### Practices

Each concern ships a `practices.md` with conventions organized by phase:

```markdown
# Practices: a11y-wcag-aa

## Requirements (Frame phase)
- All user stories involving UI must include a11y acceptance criteria
- WCAG 2.1 AA is the minimum compliance target

## Design
- All interactive components must be keyboard-navigable
- Color contrast ratios must meet AA (4.5:1 for text, 3:1 for large text)
- ARIA attributes required for custom components

## Implementation
- Use semantic HTML elements over divs with roles
- All images require alt text (decorative images use alt="")
- Form inputs require associated labels
- Focus management for modals and dynamic content

## Testing
- axe-core in CI for automated a11y checks
- Screen reader testing for critical user flows
- Keyboard-only navigation testing
```

Practices are advisory — they guide agent choices. Project overrides take
precedence over library practices.

### Project concern file

Projects select their active concerns and declare overrides:

```markdown
# Project Concerns (docs/helix/01-frame/concerns.md)

## Active Concerns
- typescript-bun (tech-stack)
- postgres (data)
- gcp-cloud-run (infra)
- a11y-wcag-aa (accessibility)
- o11y-otel (observability)
- i18n-icu (internationalization)

## Project Overrides
- Testing: use Vitest instead of bun:test (see ADR-005)
- o11y: use Datadog instead of Grafana for dashboards (see ADR-008)
```

### Relationship between concerns, ADRs, spikes, and POCs

These artifacts form a knowledge chain:

```
Spike/POC (gather evidence)
  → ADR (record decision with rationale)
    → Concern (index for context assembly)
      → Context Digest (injected into beads)
```

| Artifact | Role | Relationship to Concerns |
|----------|------|------------------------|
| **Concern** | Declares what the project cares about | The composable index used for context assembly |
| **ADR** | Records the rationale behind a concern | Provides the "why" — explains decisions, constraints, trade-offs. Referenced from concern.md and project overrides. |
| **Spike** | Gathers evidence for an ADR | Tests assumptions, benchmarks alternatives. Spike findings inform ADR decisions. |
| **POC** | Validates feasibility | Proves a concern's approach works before committing. |

The typical flow:

1. **Spike** or **POC** investigates a technology or approach question
2. **ADR** records the decision with rationale, citing spike/POC evidence
3. **Concern** is added to the project concerns file, referencing the ADR
4. **Practices** activate for the selected concern
5. **Context digest** in beads summarizes the concern + practices + ADR rationale

ADRs that modify concern behavior should be cited in the project overrides.
When an ADR is superseded, `helix polish` must flag affected concerns.

### Context digest

The context digest is a compact summary of everything an implementing agent
needs to stay consistent. It is assembled automatically at bead creation
(triage, evolve) and refreshed at bead refinement (polish).

#### Contents

A context digest includes:

1. **Active principles** — the full principles list (design + engineering),
   compact enough to include verbatim (~100 tokens)
2. **Active concerns** — area-filtered concern names with key constraints
   (~200 tokens)
3. **Merged practices** — practices from area-matched concerns, with
   project overrides applied (~300 tokens)
4. **Relevant ADRs** — decision + rationale for ADRs referenced by active
   concerns and matched by bead scope (~200 tokens per ADR)
5. **Governing spec context** — key requirements and acceptance criteria
   from the bead's `spec-id` target (~300 tokens)

Target size: **~1000-1500 tokens**. Less than one file read, better signal
than reading 5 files.

#### What stays as a pointer

The digest does not include:
- Full PRD (too large — pointer via spec-id chain)
- Full design documents (reference by section)
- ADR alternatives and exploration (just the decision)
- Test plan details (pointer to TP-XXX)
- Full feature spec (extract key requirements only)
- Concerns not matching the bead's area scope

#### Assembly algorithm

At bead creation:

1. Load `docs/helix/01-frame/principles.md` (or HELIX defaults)
2. Load `docs/helix/01-frame/concerns.md` — resolve active concerns
3. Filter concerns by area: match each concern's `areas` field against
   the bead's `area:*` labels. Concerns with `areas: all` always match.
4. For each matched concern, load `practices.md` from the library
5. Apply project overrides on top of library practices
6. Collect ADRs referenced by matched concerns and by the bead's spec-id
   chain. Extract decision statement and one-line rationale only.
7. Load the governing spec (`spec-id` target) and extract key requirements
   and acceptance criteria relevant to this bead
8. Assemble into a structured digest
9. Prepend the `<context-digest>` XML block to the bead's `description`

At polish time:

1. Re-run the assembly algorithm against current upstream state
2. Diff against the existing digest
3. If material changes exist, update the digest and note what changed

#### Digest format

The digest is prepended to the bead's `description` field using XML tags.
This avoids schema changes, is machine-parseable, and keeps the
human-written description cleanly separated below:

```xml
<context-digest>
<principles>Design for simplicity · Tests first · Fake data over fixtures ·
Local-first UX · Prefer reversible decisions</principles>
<concerns>typescript-bun | postgres | gcp-cloud-run | a11y-wcag-aa</concerns>
<practices>Biome (not Prettier) · ESLint strict · bun:test · strict tsconfig ·
semantic HTML · alt text required · WCAG AA contrast ratios</practices>
<adrs>ADR-003 event sourcing for audit trail · ADR-005 PgBouncer for
connection pooling · ADR-012 WCAG AA as minimum compliance</adrs>
<governing>FEAT-007 §3.2 — user notification preferences must support
email, push, and in-app channels with per-channel opt-out</governing>
</context-digest>

Implement user notification preferences per FEAT-007 Section 3.2.
Governing: FEAT-007, SD-007, TP-007.
```

Each XML element is optional — omit it if there is no relevant content.

`helix polish` locates the `<context-digest>` block by tag and replaces it
in-place, preserving the human-written description below. If no digest
exists yet, it is prepended.

### Injection points

| Consumer | What's injected | When |
|----------|----------------|------|
| `helix triage` | Assembles full context digest into new bead | At bead creation |
| `helix evolve` | Assembles context digest for beads it creates/modifies | At bead creation |
| `helix polish` | Refreshes context digest against current upstream | At refinement |
| `helix build` | Reads context digest from bead (no upstream reads needed) | At implementation |
| `helix review` | Reads context digest to verify implementation consistency | At review |
| `helix frame` | Concern selection available for scoping | At framing |
| `helix design` | Concern constraints inform architecture | At design |
| `helix align` | Concern drift is an alignment finding | At audit |

The key insight: **triage and polish are the write points, build and review
are the read points**. The context digest is written once (at triage) and
refreshed when needed (at polish), so implementation agents get pre-assembled
context without paying the read cost.

## Requirements

### Functional Requirements

1. HELIX must support a library of composable, named concerns in
   `workflows/concerns/`.
2. Each concern must declare its category, area scope, components,
   constraints, and associated practices in `concern.md` and `practices.md`.
3. Projects must be able to select multiple concerns and declare overrides
   in `docs/helix/01-frame/concerns.md`.
4. Project overrides must take full precedence over library practices.
5. Each concern must declare an `areas` field that controls which beads
   receive its practices in the context digest.
6. `helix triage` and `helix evolve` must assemble a context digest into
   every bead they create, including only area-matched concerns.
7. `helix polish` must refresh the context digest against current upstream
   state and flag material changes.
8. `helix build` and `helix review` must read the context digest from the
   bead rather than reconstructing context from upstream files.
9. The context digest must be compact enough (~1000-1500 tokens) that
   including it in the bead does not materially increase prompt cost.
10. ADR decisions referenced by active concerns must be included in the
    context digest with their rationale.
11. Spike and POC findings that led to concern selections must be traceable
    through the ADR that ratified the decision.
12. HELIX must ship reference concerns for common categories (tech stacks,
    a11y, o11y, i18n).
13. Concern practices and project overrides must reference governing ADRs
    when an override departs from the library default.

### Non-Functional Requirements

- **Consistency**: The same concerns, practices, and principles must appear
  in every bead for a given area scope — no agent should derive its own
  implicit choices.
- **Efficiency**: The context digest must reduce total tokens consumed per
  implementation pass compared to reading upstream files.
- **Maintainability**: Adding a new concern to the library should not require
  modifying any action prompt or skill.
- **Composability**: Concerns must be independently selectable and must not
  conflict at the practice level without explicit project resolution.

## User Stories

### US-001: Select project concerns [FEAT-006]
**As a** HELIX operator starting a new project
**I want** to select from a library of composable concerns
**So that** my project has consistent technology, quality, and convention
choices from the start

**Acceptance Criteria:**
- [ ] Given a HELIX library with concerns, when the user runs `helix frame`,
  then available concerns are presented for selection by category.
- [ ] Given the user selects "typescript-bun", "a11y-wcag-aa", and
  "o11y-otel", when the selection completes, then
  `docs/helix/01-frame/concerns.md` records all three.
- [ ] Given selected concerns, when any downstream skill runs, then it has
  access to the active concern configuration.

### US-002: Area-scoped concern injection [FEAT-006]
**As a** HELIX operator with UI and backend beads
**I want** a11y practices injected only into UI beads and o11y practices
  into backend beads
**So that** each bead gets relevant context without irrelevant noise

**Acceptance Criteria:**
- [ ] Given a11y-wcag-aa declares `areas: ui`, when a bead with `area:api`
  is created, then a11y practices are not in its digest.
- [ ] Given a11y-wcag-aa declares `areas: ui`, when a bead with `area:ui`
  is created, then a11y practices appear in its digest.
- [ ] Given typescript-bun declares `areas: all`, when any bead is created,
  then typescript practices appear in its digest.

### US-003: Self-contained implementation beads [FEAT-006]
**As a** HELIX operator running `helix run` in the background
**I want** implementation beads to contain enough context that the agent
  rarely reads upstream files
**So that** implementation is faster, cheaper, and more consistent

**Acceptance Criteria:**
- [ ] Given `helix triage` creates a bead, when the bead is created, then
  it contains a context digest with principles, area-matched concerns,
  practices, relevant ADRs, and governing spec context.
- [ ] Given an implementing agent reads the context digest, when it
  implements the bead, then it follows declared conventions consistently.
- [ ] Given the context digest is ~1000-1500 tokens, when compared to
  reading 5 upstream files, then total token usage per pass decreases.

### US-004: Refresh context on polish [FEAT-006]
**As a** HELIX operator who changed a concern or ADR
**I want** existing beads to get updated context digests
**So that** implementing agents don't work from stale context

**Acceptance Criteria:**
- [ ] Given a concern is added to the project, when `helix polish` runs,
  then beads matching the concern's area scope get updated digests.
- [ ] Given an ADR is superseded, when `helix polish` runs, then beads
  referencing the old ADR get refreshed digests.

### US-005: Concern decisions traced through ADRs and spikes [FEAT-006]
**As a** HELIX operator reviewing architecture
**I want** concern selections to reference the ADRs and spikes that justified
  them
**So that** the rationale for choices is traceable

**Acceptance Criteria:**
- [ ] Given a spike recommends adopting WCAG AA, when an ADR ratifies the
  decision, then the a11y concern references the ADR.
- [ ] Given the context digest includes an ADR summary, when a reviewer
  reads it, then they can trace back to the full ADR and spike evidence.

### US-006: Override concern practices [FEAT-006]
**As a** HELIX operator with project-specific needs
**I want** to override specific practices from my selected concerns
**So that** I can customize conventions without forking the concern definition

**Acceptance Criteria:**
- [ ] Given the typescript-bun concern recommends bun:test, when the project
  overrides to Vitest, then all downstream context uses Vitest.
- [ ] Given an override references an ADR, when the context digest is
  assembled, then the ADR rationale appears in the digest.

## Edge Cases and Error Handling

- **No concerns selected**: If `docs/helix/01-frame/concerns.md` does not
  exist, the context digest omits concerns and practices sections. No error.
- **Conflicting concerns**: If two selected concerns declare conflicting
  practices (e.g., "use Jest" vs "use bun:test"), the assembly algorithm
  must flag the conflict and require a project override to resolve it.
- **Very large digest**: If the context digest exceeds 2000 tokens (many
  concerns, many ADRs), prioritize: principles first, then concerns/practices
  by area relevance, then ADRs, then spec context. Truncate ADR summaries
  before dropping them.
- **Permitted digest omission**: If a governing workflow explicitly allows a
  bead to omit the digest, the bead must carry label
  `digest:omission-authorized`, machine-set field
  `digest-omission-path`, and its description must begin with
  `Explicit omission rationale: <reason>`. The rationale must be non-empty,
  explain why omission is allowed for that bead, and match a named
  workflow-authorized omission path. HELIX currently defines one such path:
  `helix-input:legacy-migration` for `helix input` when migrating legacy
  beads whose concern mapping is not yet complete enough to assemble a
  trustworthy digest.
- **Stale ADR in digest**: If a referenced ADR is superseded after the
  digest is assembled, `helix polish` must detect the supersession and
  update the digest.
- **No area match**: If a bead has no `area:*` labels, only concerns with
  `areas: all` are included.
- **Practice overridden without ADR reference**: Warn (not block) that the
  override lacks rationale.

## Success Metrics

- Implementation beads contain context digests in >90% of cases.
- Implementing agents make concern-consistent choices (measured by review
  findings for convention drift).
- Total token usage per implementation pass decreases compared to baseline.
- Concern selections are traceable to ADRs and spikes.

## Constraints and Assumptions

- The context digest is a static snapshot, not a live query. It is assembled
  at triage and refreshed at polish — not recomputed at build time.
- Concern definitions are markdown files in a library directory — no runtime
  infrastructure, no schema enforcement beyond convention.
- The library ships with HELIX but projects can add custom concerns.
- FEAT-003 (principles) must land first — the context digest depends on the
  principles resolution mechanism.

## Dependencies

- **FEAT-003**: Principles — the context digest includes active principles
- **FEAT-001**: Supervisory control — triage and polish dispatched by run loop
- **FEAT-002**: CLI — triage, polish, and evolve commands assemble digests
- **helix.prd**: PRD requirements for tracker-first execution
- **ADR template**: ADRs must have a stable "Decision" section extractable
  for the digest

## Out of Scope

- Concern enforcement in CI (concerns guide choices, they're not lint rules)
- Concern marketplace or remote registry (library is local)
- Automatic concern detection from existing code
- Concern versioning beyond what the library files contain
- Per-bead concern overrides (concerns are project-level)

## Open Questions

- ~~Should the context digest be a new bead field or in `description`?~~
  **Decided**: XML tags prepended to `description`. No schema changes.
- ~~Should this use "stacks" or a broader term?~~
  **Decided**: "Concerns" — covers tech stacks, a11y, i18n, o11y, design
  systems, and any other cross-cutting quality attribute.
- How should practice conflicts between concerns be detected — simple string
  matching on practice categories, or structured categories in concern.md?
- Should `helix build` ever fall back to reading upstream files when the
  digest is present? (Proposed: trust the digest, rely on polish.)
- What area taxonomy should HELIX define? Candidates: `all`, `ui`,
  `api`, `data`, `infra`, `cli`. Should areas be extensible per-project?
