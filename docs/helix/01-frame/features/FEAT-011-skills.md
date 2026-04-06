---
ddx:
  id: FEAT-011
  depends_on:
    - helix.prd
    - FEAT-001
    - FEAT-009
---
# Feature: DDx Agent Skills

**ID:** FEAT-011
**Status:** Complete
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx ships agent-facing skills (Claude Code slash commands) that guide users
and agents through complex DDx CLI operations. Skills provide validated,
contextual workflows on top of raw CLI commands — they know what flags exist,
what values are valid, and what metadata is required.

Skills follow the same SKILL.md convention used by HELIX and install to
`~/.agents/skills/ddx-*`.

## Problem Statement

**Current situation:**
- `ddx bead create` has 8+ flags; users forget `--labels`, `--spec-id`,
  `--acceptance` and create beads that aren't execution-ready
- `ddx agent run` requires knowing which harnesses are available, what models
  they support, and what effort levels mean — none of which is obvious from
  `--help`
- `ddx install` (FEAT-009) introduces registry concepts users haven't seen
- Every DDx command that agents invoke needs the agent to reverse-engineer
  valid flag combinations from help text

**Desired outcome:** `/ddx-bead` creates a bead with all required metadata.
`/ddx-agent` dispatches an agent with the right model and effort for the task.
`/ddx-install` discovers and installs packages with guided search. Skills
are the "smart" layer over the mechanical CLI.

## Architecture

### Skill Format

Each skill is a directory under `~/.agents/skills/` containing a `SKILL.md`:

```
~/.agents/skills/
├── ddx-bead/SKILL.md
├── ddx-agent/SKILL.md
├── ddx-install/SKILL.md
└── ddx-status/SKILL.md
```

The `SKILL.md` frontmatter declares the skill name, description, and argument
hints. The body provides the guidance the agent follows when the skill is
invoked.

### Installation

- `ddx init` symlinks DDx skills from a known location (either bundled in
  the binary's embedded resources or installed from the registry)
- `ddx install ddx-skills` (via FEAT-009) updates to latest skill versions
- Skills can also be managed manually

## Core Skills

### `/ddx-bead` — Guided bead creation and triage

Wraps `ddx bead create` with:
- Prompts for title, type, description, acceptance criteria
- Validates required labels (checks hook configuration for label rules)
- Suggests `--spec-id` from the document graph (nearest governing artifact)
- Sets priority based on context
- Runs `ddx bead create` with assembled flags
- Also supports `ddx bead update`, `ddx bead dep add` workflows

### `/ddx-agent` — Guided agent dispatch

Wraps `ddx agent run` with:
- Lists available harnesses via `ddx agent list`
- Shows capabilities via `ddx agent capabilities <harness>`
- Helps select model and effort level based on task complexity
- Assembles the full `ddx agent run` command
- Reports results from `ddx agent log`

### `/ddx-install` — Guided package installation

Wraps `ddx install` with:
- Searches registry via `ddx search`
- Shows package details before installing
- Confirms installation targets
- Verifies post-install health

### `/ddx-status` — Project health overview

Wraps multiple commands into one overview:
- `ddx status` + `ddx doctor` + `ddx bead list` summary
- Shows stale documents, blocked beads, pending updates

### `/ddx-review` — Guided quorum and fresh-eyes review

Wraps `ddx agent run --quorum` for structured code review:
- Prompts for scope (file paths, bead ID, or "all changes since <ref>")
- Assembles the review prompt with context from governing artifacts
- Dispatches a quorum review across multiple harnesses (e.g., codex + claude)
- Reports consensus: agreed findings, disagreements, and recommended actions
- Prevents "codex-style review" hallucinations by using real `ddx agent run`
  with explicit harness selection and structured output parsing

### `/ddx-run` — Execute a bead with proper agent dispatch

Wraps the bead → agent → verify → close lifecycle:
- Takes a bead ID (or picks the top ready bead)
- Reads the bead's spec-id, acceptance criteria, and governing artifacts
- Assembles a prompt with full context
- Dispatches via `ddx agent run --harness <selected> --prompt <file>`
- After agent completes: verifies tests pass, checks acceptance criteria
- Closes the bead if acceptance met, or reports what failed
- Prevents agents from "claiming" to have done work without verification

## Requirements

### Functional

1. DDx ships at least 6 core skills: `ddx-bead`, `ddx-agent`, `ddx-install`,
   `ddx-status`, `ddx-review`, `ddx-run`
2. Skills install to `~/.agents/skills/ddx-*` following the SKILL.md convention
3. `ddx init` registers DDx skills (symlink or copy)
4. Skills call DDx CLI commands — they are guidance wrappers, not reimplementations
5. Skills validate inputs before invoking commands (e.g., check label rules,
   verify harness availability)
6. Skills provide contextual suggestions (e.g., suggest spec-id from document
   graph, suggest model from capabilities)

### Non-Functional

- Skills are plain Markdown (SKILL.md) — no compiled code, no runtime dependencies
- Skills work with any agent that supports the SKILL.md convention (Claude Code)
- Skills degrade gracefully if DDx CLI is not installed (clear error message)

## User Stories

### US-110: Agent Creates a Well-Formed Bead
**As an** AI agent creating a work item
**I want** the `/ddx-bead` skill to guide me through required metadata
**So that** every bead I create passes triage validation

**Acceptance Criteria:**
- Given I invoke `/ddx-bead "Add login feature"`, then the skill prompts for
  type, labels, spec-id, and acceptance criteria
- Given hook validation requires `helix` label, then the skill includes it
  automatically
- Given I provide all required fields, then the skill runs `ddx bead create`
  with the correct flags

### US-111: Developer Dispatches Agent with Right Config
**As a** developer wanting to run an agent review
**I want** `/ddx-agent` to show me available models and effort levels
**So that** I pick the right configuration without memorizing flag names

**Acceptance Criteria:**
- Given I invoke `/ddx-agent`, then it shows available harnesses and their status
- Given I select a harness, then it shows available models and effort levels
- Given I confirm, then it runs `ddx agent run` with the assembled flags

### US-112: Developer Installs a Package with Guidance
**As a** developer discovering DDx packages
**I want** `/ddx-install` to search and preview before installing
**So that** I understand what I'm installing

**Acceptance Criteria:**
- Given I invoke `/ddx-install helix`, then it shows the package description
  and install targets before proceeding
- Given I confirm, then it runs `ddx install helix` and reports success

## Dependencies

- FEAT-001 (CLI commands that skills wrap)
- FEAT-009 (registry for `ddx-install` skill)
- Claude Code skill convention (SKILL.md format)

## Out of Scope

- Workflow-specific skills (HELIX provides those)
- Skills for commands that don't benefit from guidance (e.g., `ddx version`)
- GUI or interactive TUI — skills are agent-facing, not terminal-facing
