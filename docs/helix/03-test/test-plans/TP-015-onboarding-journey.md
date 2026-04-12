---
ddx:
  id: TP-015
  depends_on:
    - FEAT-001
    - FEAT-009
    - FEAT-011
    - TP-007
---
# Test Plan: End-to-End Onboarding Journey

## Objective

Validate the complete user journey from install through building a working
application with HELIX, including evolution and inspection. This plan has
two tiers: mechanical tests (no agent needed, CI-safe) and live integration
tests (require agent access, run manually or in dedicated CI).

## Tier 1: Mechanical Tests (CI-safe, no agent)

These extend the existing E2E smoke tests (TP-007) with the install and
HELIX plugin steps.

### TC-020: Install plugin from registry
**Given** DDx is initialized in a fresh project
**When** `ddx install helix` runs
**Then** exit code 0, `ddx installed` shows helix, skills exist at
`~/.agents/skills/helix-*/SKILL.md`

### TC-021: Offline init creates working structure
**Given** a fresh git repo with no network access to ddx-library
**When** `ddx init` runs
**Then** exit code 0, `.ddx/library/` exists with subdirectories,
`ddx doctor` passes

### TC-022: Bead lifecycle with dependencies
**Given** an initialized project
**When** epic and task beads are created, dependency added, task closed
**Then** `ddx bead ready` shows the epic (dep satisfied),
`ddx bead dep tree` shows the closed task

### TC-023: Agent usage reports sessions
**Given** `.ddx/agent-logs/sessions.jsonl` has fixture entries
**When** `ddx agent usage --format json` runs
**Then** output contains correct per-harness aggregation

### TC-024: Doc history resolves artifacts
**Given** a document with `ddx:` frontmatter and git history
**When** `ddx doc history <id>` runs
**Then** output contains commit entries for that file

## Tier 2: Live Integration Tests (require agent access)

These test the full HELIX workflow. They require a Claude or codex harness
and will consume tokens. Run manually or in a dedicated integration CI job
with agent credentials.

### TC-030: Frame a project with HELIX
**Given** DDx initialized, HELIX installed, claude harness available
**When** `ddx agent run --harness claude --prompt <frame-prompt>` runs
**Then** agent creates spec documents in `docs/`, beads exist for
design/implementation work

**Frame prompt:**
```
You are building a simple CLI task tracker in Go. Use /helix-frame to:
1. Create a one-page PRD for the task tracker
2. Create a feature spec for task CRUD (create, list, complete, delete)
3. Create implementation beads for each operation
Keep it minimal — this is a demo.
```

### TC-031: Build the project with HELIX
**Given** TC-030 complete (specs and beads exist)
**When** `ddx agent run --harness claude --prompt <build-prompt>` runs
**Then** agent implements the task tracker, tests pass, beads are closed

**Build prompt:**
```
Use /helix-run to work through the ready beads. Build the task tracker
per the specs. Write tests first (TDD). Close each bead when done.
```

### TC-032: Evolve the project
**Given** TC-031 complete (working task tracker)
**When** `ddx agent run --harness claude --prompt <evolve-prompt>` runs
**Then** agent updates specs, creates new beads, implements the feature

**Evolve prompt:**
```
Use /helix-evolve to add task priorities (high/medium/low) to the tracker.
This should update the feature spec, create implementation beads, and
build the feature.
```

### TC-033: Inspect the work
**Given** TC-032 complete
**When** inspection commands run
**Then** all show meaningful output

```bash
ddx bead list          # shows mix of open/closed beads
ddx bead ready         # may be empty (all done) or show follow-up work
ddx agent usage        # shows token consumption from the session
ddx doc history <prd>  # shows commits from framing and evolution
ddx doc changed --since HEAD~20  # shows all artifacts touched
```

## Demo Script: Full Journey

`scripts/demos/06-full-journey.sh` — the asciinema-recordable version:

```bash
# Phase 1: Setup (mechanical, ~10s)
ddx init
ddx install helix
ddx doctor

# Phase 2: Frame (~60s, requires agent)
ddx agent run --harness claude --effort high \
  --prompt "$(cat scripts/demos/prompts/frame-tracker.md)"

# Phase 3: Build (~120s, requires agent)
ddx agent run --harness claude --effort high \
  --prompt "$(cat scripts/demos/prompts/build-tracker.md)"

# Phase 4: Verify
ddx bead list
ddx bead status

# Phase 5: Evolve (~90s, requires agent)
ddx agent run --harness claude --effort high \
  --prompt "$(cat scripts/demos/prompts/evolve-priorities.md)"

# Phase 6: Inspect
ddx bead list
ddx agent usage --since today
ddx doc changed --since HEAD~20
```

## Pass Criteria

- **Tier 1:** All TC-020 through TC-024 pass in CI
- **Tier 2:** TC-030 through TC-033 pass with claude harness; agent creates
  working code, tests pass, beads are properly managed
- **Demo script:** Produces a compelling asciinema recording showing the
  full DDx + HELIX experience
