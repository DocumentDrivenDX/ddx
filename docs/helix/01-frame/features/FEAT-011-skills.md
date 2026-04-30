---
ddx:
  id: FEAT-011
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-006
    - FEAT-009
---
# Feature: DDx Agent Skills

**ID:** FEAT-011
**Status:** Revising (consolidation in progress; see Phase 1 epic in beads)
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx ships a single agent-facing skill (`ddx`) that makes any
skills-compatible coding agent "ddx-aware" after `ddx init`. The skill
is written to the [agentskills.io](https://agentskills.io) open
standard so it works identically in Claude Code, OpenAI Codex, Gemini
CLI, Cursor, OpenCode, and any other harness that implements the
standard.

When the user says "do work", "review this", "what's on the queue",
"create a bead", or any DDx concept, the harness discovers the `ddx`
skill via its description, loads `SKILL.md`, and routes via an
explicit intent table into `reference/*.md` files that contain the
domain guidance.

## Problem Statement

Prior iterations of FEAT-011 shipped ~7 sibling skills
(`ddx-bead`, `ddx-agent`, `ddx-run`, `ddx-review`, `ddx-status`,
`ddx-install`, `ddx-doctor`). Real-world usage exposed problems:

- **Intent ambiguity.** Users say "do work", not "/ddx-run". A flat
  list of named slash commands forces the harness to guess between
  `/ddx-bead` vs `/ddx-run` vs `/ddx-work` from natural-language
  phrases.
- **Vocabulary drift.** Each skill redefined terms (bead, queue,
  harness, review) inline; wording diverged across files and from
  FEAT-* specs.
- **Workflow-opinion leakage.** `ddx-bead` mandated `helix` labels
  and documented `phase:*` labels — HELIX methodology opinions
  inside a core DDx skill.
- **Skill tree drift.** Two copies of most skills
  (`cli/internal/skills/` embedded vs top-level `/skills/`) with
  real content divergence.
- **Init gap.** `ddx init` copied only 2 of 7 embedded skills; the
  rest never surfaced to Claude Code unless the user re-installed
  manually.
- **Portability.** Skills used Claude-Code-only frontmatter fields
  (`argument-hint`) that are silently ignored by Codex and other
  harnesses, and reached for Claude-Code-only patterns
  (`context: fork`) that don't exist elsewhere.

The consolidated design fixes each of these.

## Architecture

### Single skill, progressive disclosure

```
skills/ddx/
├── SKILL.md                # overview, vocabulary, intent-router directive
├── reference/
│   ├── beads.md            # writing execution-ready beads (best practices)
│   ├── work.md             # draining the queue, execute-bead, verify + close
│   ├── review.md           # bead-review (AC grade) + quorum code review
│   ├── agents.md           # harness/profile dispatch, personas
│   └── status.md           # queue state, doctor, health checks
└── evals/
    └── routing.jsonl       # ~15 phrase→expected-routing fixtures
```

At harness startup, only the skill's `name` + `description` metadata
is pre-loaded. On activation, the harness reads `SKILL.md`. When the
intent router matches a phrase, the harness reads the matching
`reference/*.md`. Nothing else loads. This is the Anthropic
"Pattern 2: Domain-specific organization" pattern and matches how
Codex and other agentskills.io implementations progressively disclose
skill content.

### Portability contract

Frontmatter is the portable-safe minimum — `name` + `description` only.

```yaml
---
name: ddx
description: Operates the DDx toolkit for document-driven development. Covers beads (work items), the queue, executions, agents, harnesses, personas, reviews, spec-id. Use when the user says "do work", "drain the queue", "run the next bead", "execute a bead", "review this", "check against spec", "what's on the queue", "what's ready", "create a bead", "file this as work", "run an agent", "dispatch", "use a persona", "how am I doing", "ddx doctor", or mentions any ddx CLI command.
---
```

No `argument-hint`, `when_to_use`, `context: fork`, `allowed-tools`,
`disable-model-invocation`, `user-invocable`, `paths`, `model`,
`effort`, `agent`, or `hooks` fields — those are Claude-Code
extensions that Codex and others ignore or reject. The description
front-loads the DDx nouns (bead, queue, execution, harness, persona,
spec-id) **and** the exact verb phrases users say verbatim, because
implicit-invocation matchers prefer substring-ish keyword matching
over semantic understanding.

### Intent-router directive

Claude does not reliably auto-chase reference links; the router in
`SKILL.md` is stated as an **explicit directive**, not a hint:

> "Before responding to any DDx-related request, read the matching
> reference file below. The router is not optional — your answer must
> be grounded in the reference file's guidance, not this overview
> alone."

### Subagents: ship none

Subagent orchestration is harness-specific (Claude Code has
`.claude/agents/` + `context: fork`; Codex has its own subagent
surface; others differ). The `ddx` skill describes *actions* such as
"run `ddx run` with this prompt" or "use the compare-prompts skill" and lets
each harness decide how to run them. Multi-agent comparison is a workflow skill
composition, not a skill-frontmatter directive.

### Installation

> See FEAT-015 (2026-04-29 amendment) for the authoritative install
> model: project-local copy semantics, zero symlinks created by
> `ddx install`, and the no-`~` manifest invariant.

- `ddx init` copies `skills/ddx/` into `.claude/skills/ddx/`,
  `.agents/skills/ddx/`, and `.ddx/skills/ddx/` as real files
  (symlinks break after `git clone` on a fresh machine, and DDx no
  longer creates symlinks during install — see FEAT-015).
- On init and on `ddx update`, stale ddx-prefixed skill directories
  from prior DDx versions are removed:
  `ddx-bead`, `ddx-run`, `ddx-agent`, `ddx-review`, `ddx-status`,
  `ddx-doctor`, `ddx-install`, `ddx-release`. Third-party skills are
  untouched.
- Skills embed into the binary via `//go:embed all:ddx` against a
  copy under `cli/internal/skills/ddx/`. Because `go:embed` cannot
  traverse upward, a `make copy-skills` target rsyncs
  `skills/ddx/` → `cli/internal/skills/ddx/` before every build.

### AGENTS.md: merge, not clobber

Codex treats `AGENTS.md` as primary guidance before work, and users
may have added content. `ddx init` uses marker-delimited injection:

```markdown
<!-- DDX-AGENTS:START -->
This project uses DDx. Use the `ddx` skill for beads, work, review,
agents, and status.

(tracker/merge policy follows)
<!-- DDX-AGENTS:END -->
```

Content outside the markers is preserved. The block says
"the `ddx` skill", not "`/ddx`" (which is Claude-specific slash-
command phrasing). Re-running `ddx init` updates the block in place.

### Evaluation-driven validation

Anthropic's skill-authoring guidance treats evaluations as
load-bearing, not optional. The repo ships:

- `skills/ddx/evals/routing.jsonl` — ~15 rows, each a user phrase +
  expected reference file + expected CLI invocation, covering every
  intent-router entry and edge phrasings.
- `scripts/eval-skill.sh` — driver that runs each row against
  `--harness claude` and `--harness codex` and verifies routing.
  `--validate` mode does agentskills.io spec conformance.
- `make eval-skill` in CI on PRs that touch `skills/ddx/`.

## Requirements

### Functional

1. DDx ships exactly one skill (`ddx`) in `skills/ddx/`; no sibling
   `ddx-*` skill directories.
2. `SKILL.md` frontmatter contains only `name` and `description`.
3. `SKILL.md` body is under 500 lines and includes an explicit
   intent-router directive.
4. `reference/*.md` files are linked one level deep from `SKILL.md`.
5. `ddx init` copies the skill, removes stale `ddx-*` dirs, and
   merges the AGENTS.md block without clobbering user content.
6. `ddx update` refreshes `.claude/skills/ddx/` and removes stale
   dirs.
7. `skills/ddx/evals/routing.jsonl` contains at least 15 rows, each
   passing against `claude` and `codex` harnesses via
   `make eval-skill`.
8. `SKILL.md` passes `scripts/eval-skill.sh --validate` (agentskills.io
   spec conformance).

### Non-Functional

- Skills work with any agent supporting the agentskills.io standard;
  no Claude-Code-only frontmatter or directives.
- Skills are plain Markdown — no runtime dependencies.
- Skills degrade gracefully if DDx CLI is not installed (clear error).
- HELIX-specific rules (`helix` label requirement, `phase:*`
  enumeration) do not appear in the `ddx` skill; HELIX opinions ship
  in the HELIX plugin.

## User Stories

### US-110: Harness routes natural-language DDx intent
**As a** user in a DDx project using Claude Code, Codex, or any other
skills-compatible harness
**I want** phrases like "do work", "drain the queue", "review this",
"what's on the queue", "create a bead" to route to DDx guidance
**So that** I don't have to remember slash-command names

**Acceptance Criteria:**
- Running `make eval-skill` passes all rows against both `--harness claude`
  and `--harness codex`.
- Each intent-router entry in `SKILL.md` has at least one matching
  row in `routing.jsonl`.

### US-111: Skill stays under the token budget
**As a** harness loading the `ddx` skill
**I want** `SKILL.md` body under 500 lines
**So that** skill activation stays within the Anthropic-recommended
token budget and doesn't compete with conversation context

**Acceptance Criteria:**
- `wc -l skills/ddx/SKILL.md` < 500.
- `scripts/eval-skill.sh --validate` passes.

### US-112: `ddx init` handles existing projects cleanly
**As a** user upgrading from an older DDx version with
`.claude/skills/ddx-run/` and similar dirs already present
**I want** `ddx init` to remove the old dirs and install only the
new single-skill layout
**So that** the harness doesn't see stale, conflicting skills

**Acceptance Criteria:**
- In a dir pre-seeded with
  `.claude/skills/{ddx-run,ddx-doctor,ddx-bead}/`, running
  `ddx init` leaves only `.claude/skills/ddx/`.
- Third-party skills under `.claude/skills/` are untouched.

### US-113: `AGENTS.md` merge preserves user content
**As a** user who has added content to `AGENTS.md`
**I want** `ddx init` to inject the DDx block without clobbering
what I wrote
**So that** my Codex / Claude / Gemini setup isn't broken

**Acceptance Criteria:**
- Given an `AGENTS.md` with user content both before and after the
  `<!-- DDX-AGENTS:START -->` / `<!-- DDX-AGENTS:END -->` markers,
  running `ddx init` updates the block between markers and preserves
  everything outside.
- Running `ddx init` a second time does not duplicate the block.

### US-114: `ddx update` refreshes skills
**As a** user who ran `ddx init` on an older DDx version
**I want** `ddx update` to refresh `.claude/skills/ddx/` to the
current shipped content
**So that** I don't have to re-run `ddx init` to pick up skill
improvements

**Acceptance Criteria:**
- After `ddx update`, `.claude/skills/ddx/` bytes match the embedded
  skill content.
- Stale `ddx-*` dirs are removed as in US-112.

## Dependencies

- FEAT-001 (CLI commands the skill wraps)
- FEAT-006 (agent service — harnesses, profiles, personas)
- FEAT-009 (registry for package-install guidance inside `reference/agents.md`)
- agentskills.io open standard

## Out of Scope

- Workflow-specific skills (HELIX provides `helix-*` in its own plugin).
- Claude-Code-specific skill features (`context: fork`, `allowed-tools`,
  `paths`, subagents under `.claude/agents/`) — portability > optimization.
- A CI-enforced vocabulary drift guard. One skill + one glossary is
  self-policing. Revisit only if drift recurs.

## Implementation Notes

The migration from the 7-sibling-skills layout to the single `ddx`
skill is sequenced into phases; see the Phase 1 / Phase 2 / Phase 3
epic beads for the work breakdown. Phase 1 is the critical path
(ship the new surface + eval suite + init/update changes); Phase 2
is cleanup of old references; Phase 3 is the persona roster trim
(FEAT-006 scope, not blocking this feature).
