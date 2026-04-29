---
title: Concepts
weight: 5
prev: /docs/glossary/concerns
next: /docs/glossary/tracker
---

# Core Concepts

The ideas that make HELIX work.

## Authority Order

When HELIX artifacts disagree, resolve the conflict using this precedence:

1. **Product Vision** — highest authority
2. **Product Requirements** (PRD)
3. **Feature Specs / User Stories**
4. **Architecture / ADRs**
5. **Solution Designs / Technical Designs**
6. **Test Plans / Tests**
7. **Implementation Plans**
8. **Source Code / Build Artifacts** — lowest authority

Higher-order artifacts govern lower-order artifacts. Tests are executable specifications — code must satisfy tests, not the other way around. Source code is evidence of current state, not the source of truth for requirements.

If a lower-level artifact contradicts a higher one, fix the lower-level artifact. Only change higher-level artifacts when the evidence is strong and the governing artifacts are stale or incomplete.

## Specification-First Development

HELIX enforces writing tests before implementation (TDD). The cycle:

1. **Red** (Test phase) — Write a failing test that defines desired behavior
2. **Green** (Build phase) — Write minimal code to make the test pass
3. **Refactor** (Build phase) — Improve code quality while keeping tests green

Tests are the contract between design and implementation. Build cannot start until tests exist and fail. Implementation is complete when tests pass.

## Principles

Design and engineering values that guide judgment calls throughout the project.

- Loaded from `docs/helix/01-frame/principles.md` (project-specific)
- Falls back to `workflows/principles.md` (HELIX defaults) if no project file exists
- Applied by every action that makes technology or quality choices
- Only `helix frame` may create or modify the principles file

Examples: "design for simplicity", "tests first", "local-first UX", "prefer composition over inheritance".

## Context Digest

A compact summary (~1000-1500 tokens) assembled into tracker beads at triage/polish time. Makes beads self-contained execution units.

Contents:
- **Principles** — full list, compact format
- **Concerns** — area-matched concern names
- **Practices** — key conventions from matched concerns
- **ADRs** — decision statements and rationale from relevant ADRs
- **Governing spec** — the specific requirement or constraint this bead addresses

Format: XML-tagged block prepended to the bead description:

```xml
<context-digest>
<principles>Simplicity · Tests first · Local-first UX</principles>
<concerns>rust-cargo | security-owasp</concerns>
<practices>clippy pedantic · cargo deny · parameterized queries</practices>
<adrs>ADR-003 chose Axum over Actix for async compatibility</adrs>
<governing>FEAT-002 §3.1 — WAL must fsync before acknowledging writes</governing>
</context-digest>
```

Agents read the digest instead of loading upstream files. `helix polish` keeps digests current.

## Tracker-Driven Execution

The tracker is HELIX's steering wheel. All work flows through tracker issues (beads):

- **Operators steer** by creating, prioritizing, and blocking issues
- **Agents execute** by claiming and closing issues
- **The ready queue** is the only durable hand-off mechanism between sessions
- Follow-up work must be filed as beads before an action closes — prose suggestions without beads are lost

Every execution issue must cite the canonical artifacts that authorize the work via `spec-id`.

## Bounded Execution

Every HELIX action is intentionally bounded:

- `build` handles one issue and exits
- `check` reads the queue and recommends one action
- `review` examines one scope and files findings
- `design` produces one design document

The supervisor (`helix run`) chains bounded actions based on queue state. No action runs forever. This makes execution predictable, auditable, and interruptible.

## Cross-Model Verification

HELIX supports using different AI models for implementation and review:

- The build agent implements code
- A different review agent examines the work with fresh perspective
- Alternating models provides adversarial review — the reviewer has no implementation blindness

Configured via `helix run --review-agent <agent>` or `HELIX_ALT_AGENT` environment variable.

## Epic Focus

When the supervisor encounters a large scope (epic), it decomposes rather than deferring:

1. Break the epic into subtask issues
2. Implement the first subtask
3. Review, then continue to the next
4. Close the epic when all subtasks are done

Decomposition IS implementation work. The right response to a hard problem is to break it into smaller pieces, not to bail.

## Continuous Useful Work

Agents should maximize forward progress:

- **Absorb small adjacent work** — if a fix requires updating a nearby test or doc, do it in the same issue
- **Stay within scope** — don't expand beyond what the issue asks for
- **Finish with blocker reports** — if you can't complete the work, explain exactly what's blocked and create follow-on issues
- **Never stop silently** — a bead must be closed with evidence, or left open with a precise status note

## Quality Ratchets

A mechanism for ensuring quality metrics only improve over time:

- A **floor** is a committed minimum value for a metric (e.g., 70% test coverage)
- The floor can only go up, never down
- If the metric drops below the floor, it's a regression that must be fixed
- Experiments can auto-bump the floor when improvement exceeds a threshold

Ratchet definitions live in `workflows/ratchets.md` and floor fixtures are committed to the repo.

## Area Taxonomy

Areas scope which [concerns](/docs/glossary/concerns) apply to which beads:

| Area | Typical scope | Example concerns |
|------|--------------|-----------------|
| `all` | Every bead | Tech stacks, security |
| `ui` | Frontend, web | a11y, i18n |
| `api` | Backend, server | o11y, rate limiting |
| `data` | Database, storage | Data modeling |
| `infra` | Deployment, CI | k8s, monitoring |
| `cli` | CLI tools | CLI conventions |

Projects define their area labels in `docs/helix/01-frame/concerns.md`. A bead with `area:ui` gets a11y practices; a database migration bead does not.
