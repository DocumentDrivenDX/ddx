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
**Status:** Implemented (root `ddx` skill with nested workflow skills in package-owned `library/skills/ddx/` and package installer outputs at `.agents/skills/ddx/` and `.claude/skills/ddx/`)
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx ships a single root agent-facing skill (`ddx`) that makes any
skills-compatible coding agent "ddx-aware" after `ddx init`. Under that root,
DDx may ship nested workflow skills such as `bead-breakdown/`, `replay-bead/`,
`compare-prompts/`, and the bead-lifecycle quality skill governed by ADR-023.
The root skill and nested skills are written to the
[agentskills.io](https://agentskills.io) open standard so they work identically
in Claude Code, OpenAI Codex, Gemini CLI, Cursor, OpenCode, and any other
harness that implements the standard.

When the user says "do work", "review this", "what's on the queue",
"create a bead", or any DDx concept, the harness discovers the `ddx`
skill via its description, loads `SKILL.md`, and routes via an
explicit intent table into `reference/*.md` files that contain the
domain guidance.

## Problem Statement

Prior iterations of FEAT-011 shipped ~7 sibling skills
(`ddx-bead`, `ddx-run`, `ddx-review`, `ddx-status`,
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

### Root skill plus nested workflow skills

```
library/skills/ddx/        # package-owned canonical source
├── SKILL.md                # overview, vocabulary, intent-router directive
├── reference/
│   ├── beads.md            # writing execution-ready beads (best practices)
│   ├── work.md             # draining the queue, execute-bead, verify + close
│   ├── review.md           # bead-review (AC grade) + quorum code review
│   ├── agents.md           # harness/profile dispatch, personas
│   ├── interactive.md      # default queue-steward conversation workflow
│   └── status.md           # queue state, doctor, health checks
├── evals/
│   └── routing.jsonl       # phrase→mode/reference/action fixtures
├── bead-breakdown/
│   └── SKILL.md            # workflow skill composition over ddx bead create
├── replay-bead/
│   └── SKILL.md            # workflow skill composition over ddx run / try
└── ...
```

At harness startup, only the skill's `name` + `description` metadata
is pre-loaded. On activation, the harness reads `SKILL.md`. When the
intent router matches a phrase, the harness reads the matching
`reference/*.md`. Nothing else loads. This is the Anthropic
"Pattern 2: Domain-specific organization" pattern and matches how
Codex and other agentskills.io implementations progressively disclose
skill content.

Nested workflow skills are independent skills stored below the root `ddx/`
directory so DDx can package them with the core domain vocabulary without
returning to the stale flat sibling model (`ddx-bead`, `ddx-run`,
`ddx-review`, etc.). They describe reusable workflows over DDx primitives:
break down an epic into beads, replay a bead under altered conditions, compare
prompt variants, estimate effort, run adversarial review, and perform
bead readiness assessment, lint/rubric scoring, and post-attempt triage. The
bead-lifecycle skill owns bead readiness assessment, the rubric pass inside
readiness, post-attempt triage, and refine guidance. They do not
introduce new DDx run kinds; FEAT-010's three-layer architecture remains the
execution substrate.

### Portability contract

Frontmatter is the portable-safe minimum — `name` + `description` only.

```yaml
---
name: ddx
description: Operates the DDx toolkit for document-driven development. Covers beads (work items), the queue, executions, agents, harnesses, personas, reviews, spec-id. Use when the user says "do work", "drain the queue", "run the next bead", "execute a bead", "review this", "review with fresh eyes", "fold in this guidance", "review again", "break this down into specs and beads", "make this testable", "check against spec", "what's on the queue", "what's ready", "what's blocking the queue", "create a bead", "file this as work", "run an agent", "dispatch", "use a persona", "how am I doing", "ddx doctor", or mentions any ddx CLI command.
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

### Interactive-steward routing

`reference/interactive.md` is the first stop for broad conversational DDx
prompts that combine queue orientation, planning, review, guidance folding,
spec alignment, and bead breakdown. It delegates to `status.md`, `review.md`,
`beads.md`, and `work.md` instead of duplicating their domain guidance.

Router precedence is load-bearing:

1. Explicit worker commands such as `ddx work`, `ddx try <id>`, "execute bead
   `<id>`", or "start the worker" route to `bead_execution` and `work.md`.
2. Broad prompts such as "what's blocking the queue", "review with fresh
   eyes", "fold in this guidance", "review again", "make this testable", and
   "break this down into specs and beads" route to `interactive-steward` and
   `interactive.md`.
3. Explicit code/doc edit requests such as "fix this bug" or "update this
   file" route to `direct_user_implementation`.
4. Explicit review-only requests route to `review` and remain read-only unless
   the user asks for fixes.

`interactive.md` defines phase output contracts:

- `orient` returns `QueueFacts`. It probes `ddx work focus --help`; if the
  command is unavailable, it falls back to `ddx bead status`, `ddx bead blocked`,
  `ddx bead ready`, and targeted `ddx bead show`. If `ddx work focus` exists
  but fails, the steward reports that failure.
- `plan` returns `SessionBrief`.
- `fresh-eyes review` returns `Findings` and `Verdict`. Multi-harness
  adversarial review is consent-gated unless explicitly requested.
- `fold guidance` returns `Accepted`, `Rejected`, `Unresolved`, and a revised
  plan.
- `align specs` returns `SpecDeltas`.
- `breakdown` returns filed bead IDs, parent/dependency edges, named tests, and
  verification commands.

The route fixtures under `library/skills/ddx/evals/routing.jsonl` assert a stable row
schema: `phrase`, `mode`, `references`, `queue_commands`,
`tracker_mutation_allowed`, `code_edits_allowed`, and
`expected_next_action`. The evaluation driver consumes those fields directly;
it must not keep validating only the legacy `expected_reference` /
`expected_cli` pair after the richer schema lands. Negative fixtures are
required: "what should I work on next" routes to `interactive-steward` with no
code edits, whereas "implement the top ready bead" is direct implementation only
because the implementation verb is explicit.

### Nested workflow skills

Subagent orchestration remains harness-specific (Claude Code has
`.claude/agents/` + `context: fork`; Codex has its own subagent surface;
others differ). DDx does not ship harness-specific subagent definitions.

DDx does ship nested workflow skills under the root skill tree when a
workflow has enough procedure to deserve its own reusable instructions. Current
examples include `bead-breakdown/`, `replay-bead/`, `compare-prompts/`,
`benchmark-suite/`, `effort-estimate/`, and `adversarial-review/`. ADR-023
(`../../02-design/adr/ADR-023-bead-lifecycle-quality-policy.md`) adds the same
model for bead-lifecycle quality: bead readiness assessment is the canonical
pre-claim decision, lint/rubric scoring is the rubric pass inside readiness,
and post-attempt triage is the after-evidence classification. Those steps
invoke a nested workflow skill while keeping routing through the root `ddx`
skill.

Nested workflow skills are compositions over FEAT-010's three layers. They may
tell the harness to run `ddx run`, `ddx try`, or `ddx work`, but they do not
create a fourth run layer, bespoke storage shape, or harness-specific
frontmatter contract.

The bead-lifecycle skill owns bead readiness assessment, lint/rubric scoring,
post-attempt triage, and refine guidance for this policy surface. Those
responsibilities are separate from the legacy `MODE: intake` compatibility
wording, which remains only as an implementation detail for older hooks.

### Installation

> See FEAT-015 (2026-05-12 amendment) and plan-2026-05-13-ddx-skill-package-layout
> for the authoritative install model: package-owned source, embedded package copy,
> and project-local installer outputs.

- Canonical DDx skill source lives in `library/skills/ddx/` and is owned by the
  default `ddx` plugin package. This is the single source of truth.
- The binary embeds the entire default package library via `//go:embed` at
  `cli/internal/registry/defaultplugin/library/`, so the skill ships offline
  without separate download.
- `ddx init` installs the default `ddx` plugin through the embedded package
  installer, producing real project files at `.agents/skills/ddx/` and
  `.claude/skills/ddx/` (no symlinks, git-trackable).
- On init and on `ddx init --force`, stale ddx-prefixed skill directories
  from prior DDx versions are removed:
  `ddx-bead`, `ddx-run`, `ddx-review`, `ddx-status`,
  `ddx-doctor`, `ddx-install`, `ddx-release`. Third-party skills are
  untouched.
- Nested workflow skills live under `library/skills/ddx/` so the entire tree
  is owned by the default package. On install, all nested subdirectories become
  project-local content at `.agents/skills/ddx/` and `.claude/skills/ddx/`.
- For development overlays, `ddx plugin install ddx --local library --force`
  creates project-local symlinks to the source for live editing without
  auto-committing.

### AGENTS.md: merge, not clobber

Codex treats `AGENTS.md` as primary guidance before work, and users
may have added content. `ddx init` uses marker-delimited injection:

```markdown
<!-- DDX-AGENTS:START -->
This project uses DDx. Use the `ddx` skill for beads, work, review,
agents, and status.

(default interactive sessions use interactive-steward / `queue_steward`; explicit
`ddx work` and `ddx try` prompts use bead_execution only for executing bead AC;
tracker, merge, commit, safety, and verification policy still apply)

(tracker/merge policy follows)
<!-- DDX-AGENTS:END -->
```

Content outside the markers is preserved. The block says
"the `ddx` skill", not "`/ddx`" (which is Claude-specific slash-
command phrasing). Re-running `ddx init` updates the block in place.

### Evaluation-driven validation

Anthropic's skill-authoring guidance treats evaluations as
load-bearing, not optional. The repo ships:

- `library/skills/ddx/evals/routing.jsonl` — at least 15 rows, each a user phrase plus
  mode, references, queue commands, mutation permissions, and expected next
  action, covering every intent-router entry and edge phrasing.
- `scripts/eval-skill.sh` — driver that validates the fixture schema, runs each
  row against `--harness claude` and `--harness codex`, and verifies the richer
  routing contract. `--validate` mode does agentskills.io spec conformance.
- `make eval-skill` in CI on PRs that touch `library/skills/ddx/`.

## Requirements

### Functional

1. DDx ships exactly one root skill tree (`ddx`) in `library/skills/ddx/`; no sibling
   `ddx-*` skill directories. Workflow-specific DDx skills live as nested
   subdirectories under the root tree in the package.
2. `SKILL.md` frontmatter contains only `name` and `description`.
3. `SKILL.md` body is under 500 lines and includes an explicit
   intent-router directive.
4. `reference/*.md` files are linked one level deep from the root `SKILL.md`;
   nested workflow skills carry their own `SKILL.md` and optional local
   `reference/`, `scripts/`, or `evals/` as needed.
5. `ddx init` copies the skill, removes stale `ddx-*` dirs, and
   merges the AGENTS.md block without clobbering user content.
6. `ddx init --force` refreshes `.claude/skills/ddx/` and removes stale
   dirs.
7. `library/skills/ddx/evals/routing.jsonl` contains at least 15 rows, each
   passing the richer route schema and then passing against `claude` and
   `codex` harnesses via `make eval-skill`.
8. `library/skills/ddx/SKILL.md` passes `scripts/eval-skill.sh --validate`
   (agentskills.io spec conformance).
9. `reference/interactive.md` is present in `library/skills/ddx/reference/` and
   defines the interactive-steward loop, phase output contracts, mutation policy,
   consent-gated adversarial review, and `ddx work focus` fallback behavior.
   After `ddx init`, this file is present at `.agents/skills/ddx/reference/interactive.md`
   and `.claude/skills/ddx/reference/interactive.md`.
10. Generated AGENTS guidance uses the same precedence as the skill: broad
    interactive prompts route to `interactive-steward`, explicit worker prompts
    route to `bead_execution`, and `DDX_MODE=bead_execution` never overrides
    tracker, merge, safety, commit, or verification policy.
11. `scripts/eval-skill.sh` consumes the interactive routing fields
    (`mode`, `references`, `queue_commands`, `tracker_mutation_allowed`,
    `code_edits_allowed`, `expected_next_action`) instead of relying only on
    the legacy `expected_reference` / `expected_cli` fields.
12. After `ddx init`, `.agents/skills/ddx/` and `.claude/skills/ddx/` contain
    project-local copies of `library/skills/ddx/` generated by the package
    installer. The canonical source is `library/skills/ddx/`.

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
- Each row includes `mode`, `references`, `queue_commands`,
  `tracker_mutation_allowed`, `code_edits_allowed`, and
  `expected_next_action`, and the eval driver validates those fields.
- Route fixtures cover "review with fresh eyes", "fold in this guidance",
  "review again", "break this down into specs and beads", "make this testable",
  "what should I work on next", "implement the top ready bead", and
  `ddx work --once`.

### US-110a: Interactive steward supports multi-turn planning
**As a** user steering a DDx project interactively
**I want** broad planning/review/breakdown phrases to follow a standard steward
loop
**So that** I do not have to retype "fresh-eyes review", "fold guidance",
"align specs", and "make testable beads" instructions every turn

**Acceptance Criteria:**
- `reference/interactive.md` documents the session brief fields and phase output
  contracts for orient, plan, fresh-eyes review, fold guidance, align specs, and
  breakdown.
- "Fresh eyes" defaults to local structured review; adversarial multi-harness
  review is suggested and consent-gated unless explicitly requested.
- Durable bead/spec outputs inline the accepted session brief and never rely on
  chat history or `/tmp` files.

### US-111: Skill stays under the token budget
**As a** harness loading the `ddx` skill
**I want** `SKILL.md` body under 500 lines
**So that** skill activation stays within the Anthropic-recommended
token budget and doesn't compete with conversation context

**Acceptance Criteria:**
- `wc -l library/skills/ddx/SKILL.md` < 500.
- `scripts/eval-skill.sh --validate` passes.

### US-112: `ddx init` handles existing projects cleanly
**As a** user upgrading from an older DDx version with
`.claude/skills/ddx-run/` and similar dirs already present
**I want** `ddx init` to remove the old dirs and install only the
  new root-skill layout
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

### US-114: `ddx init --force` refreshes skills
**As a** user who ran `ddx init` on an older DDx version
**I want** `ddx init --force` to refresh `.claude/skills/ddx/` to the
current shipped content
**So that** I can use one explicit project-bootstrap refresh command

**Acceptance Criteria:**
- After `ddx init --force`, `.claude/skills/ddx/` bytes match the embedded
  skill content.
- Stale `ddx-*` dirs are removed as in US-112.

## Dependencies

- FEAT-001 (CLI commands the skill wraps)
- FEAT-006 (agent service — harnesses, profiles, personas)
- FEAT-009 (registry for package-install guidance inside `reference/agents.md`)
- ADR-023 (bead-lifecycle quality policy using nested workflow skills)
- agentskills.io open standard

## Out of Scope

- Workflow-specific skills (HELIX provides `helix-*` in its own plugin).
- Claude-Code-specific skill features (`context: fork`, `allowed-tools`,
  `paths`, subagents under `.claude/agents/`) — portability > optimization.
- A CI-enforced vocabulary drift guard. One skill + one glossary is
  self-policing. Revisit only if drift recurs.

## Implementation Notes

The migration from the 7-sibling-skills layout to the root `ddx`
skill tree is sequenced into phases; see the Phase 1 / Phase 2 / Phase 3
epic beads for the work breakdown. Phase 1 is the critical path
(ship the new surface + eval suite + init/update changes); Phase 2
is cleanup of old references; Phase 3 is the persona roster trim
(FEAT-006 scope, not blocking this feature).
