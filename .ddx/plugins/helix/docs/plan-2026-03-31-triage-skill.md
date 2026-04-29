# Design Plan: helix-triage Skill

**Date**: 2026-03-31
**Status**: CONVERGED
**Refinement Rounds**: 1

## Problem Statement

When the HELIX run loop creates issues — during decomposition, follow-on
capture, or blocker recording — the issues are often missing critical metadata
that makes them execution-eligible. This causes:

1. Issues rot in the ready queue because `helix_select_ready_issue` skips them
   (no `spec-id`, `acceptance`, or `design` fields).
2. Agents waste cycles reading issues only to discover they lack enough context
   to implement.
3. Label taxonomy is inconsistent — missing phase, kind, or area labels.
4. Blocked issues lack structured descriptions, making human triage harder.
5. Epic decomposition creates children that don't reference the parent's
   acceptance criteria, so there's no way to verify coverage.

A `helix-triage` skill enforces quality gates on issue creation and mutation,
ensuring every issue enters the tracker execution-ready.

## Requirements

### Functional

1. **Validate required fields by type**: tasks need `spec-id`, `acceptance`,
   and at least one phase label. Epics need `acceptance` that defines "done."
2. **Enforce label taxonomy**: every issue must have `helix` + one `phase:*`
   label. Warn on missing `kind:*` or `area:*` labels.
3. **Validate dependencies**: deps must reference existing issue IDs. Detect
   and reject circular dependencies.
4. **Enforce blocker format**: when marking an issue blocked, require: what's
   blocked, why, and what would unblock it.
5. **Validate decomposition coverage**: when creating children of an epic,
   verify that the children's acceptance criteria collectively cover the
   parent's acceptance criteria (advisory, not blocking).
6. **Promote tasks to epics**: when a task accumulates 2+ children, warn that
   it should be promoted to an epic.
7. **Execution eligibility check**: warn when an issue would not be selected
   by `helix_select_ready_issue` due to missing metadata.

### Non-Functional

- Validation must be fast (< 1s) — it runs inline during issue creation.
- Must work with both `ddx bead create` and programmatic JSONL edits.
- Must not break existing workflows — warnings, not hard blocks, for
  advisory checks.

### Constraints

- Implemented as a bash function library in `scripts/tracker.sh` (validation
  hooks) plus a skill prompt in `skills/helix-triage/SKILL.md`.
- The skill prompt guides agents through proper issue creation.
- The validation hooks enforce structure at the tracker layer.

## Architecture Decisions

### Decision 1: Validation layer placement

- **Question**: Where should validation run — in the skill prompt, the tracker
  library, or both?
- **Alternatives**:
  - A) Skill prompt only — agents get guidance but programmatic creation bypasses
  - B) Tracker library only — enforced but no agent guidance
  - C) Both — tracker validates structure, skill guides content quality
- **Chosen**: C — both layers
- **Rationale**: Tracker validation catches structural issues (missing fields,
  bad deps) regardless of how issues are created. The skill prompt guides
  agents to write good acceptance criteria, descriptions, and spec-id
  references — things that can't be validated structurally.

### Decision 2: Hard errors vs warnings

- **Question**: Should validation failures block issue creation?
- **Alternatives**:
  - A) All failures block — strict but may frustrate rapid iteration
  - B) Structural failures block, content quality is advisory
  - C) Everything is advisory
- **Chosen**: B — structural blocks, content warnings
- **Rationale**: Missing labels or circular deps are bugs. But we can't
  machine-validate whether acceptance criteria are "good enough." The skill
  prompt handles content quality via agent guidance.

### Decision 3: Skill scope

- **Question**: Should the skill also handle issue updates and bulk operations?
- **Alternatives**:
  - A) Create-only — simplest
  - B) Create + update — catches mutations that break structure
  - C) Create + update + bulk triage of existing issues
- **Chosen**: B — create and update
- **Rationale**: Updates can break structure (e.g., removing spec-id). Bulk
  triage is a separate concern better handled by `helix polish`.

## Interface Contracts

### Skill CLI Surface

```bash
helix triage "Issue title" [--type task|epic|bug|chore] [options...]
```

Maps to `ddx bead create` with validation. All `tracker create` flags
are passed through.

### Tracker Validation Hooks

Two new functions in `scripts/tracker.sh`:

```bash
tracker_validate_create(fields...)  # Called before tracker_create_impl
tracker_validate_update(id, fields...)  # Called before tracker_update_impl
```

Return 0 on success, 1 on hard error (with message to stderr),
2 on advisory warning (with message to stderr, creation proceeds).

### Validation Rules

| Rule | Type | Scope |
|------|------|-------|
| `helix` label present | hard | create, update |
| At least one `phase:*` label | hard | create |
| `spec-id` non-empty for tasks | hard | create |
| `acceptance` non-empty for tasks and epics | hard | create |
| deps reference existing issues | hard | create, update |
| No circular dependencies | hard | create, update |
| `kind:*` label present | warn | create |
| `area:*` label present | warn | create |
| Task with 2+ children should be epic | warn | update (child creation) |
| Blocked issues have structured notes | warn | update |

### Skill Prompt Contract

The `SKILL.md` prompt guides the agent to:

1. Identify the governing artifact (spec-id)
2. Write deterministic acceptance criteria
3. Choose correct type, phase, and kind labels
4. Set parent and dependencies
5. Assess execution eligibility

## Data Model

No new data — uses existing `.helix/issues.jsonl` schema. Validation is
applied at the tracker function layer.

## Error Handling

- Hard validation errors: print message to stderr, return exit code 1,
  issue is NOT created/updated.
- Warnings: print message to stderr prefixed with `[triage]`, issue IS
  created/updated.
- Validation errors include the specific rule that failed and what value
  was expected.

## Security

N/A — local issue tracker, no auth surface.

## Test Strategy

- **Unit**: Add test cases to `tests/helix-cli.sh`:
  - Create task without spec-id → error
  - Create task without acceptance → error
  - Create task without helix label → error
  - Create task with circular dep → error
  - Create task with valid fields → success
  - Create epic without acceptance → error
  - Update removing required field → error
  - Warning for missing kind/area labels
- **Integration**: Verify `helix triage` CLI command works end-to-end
- **E2E**: Run `helix run` and verify new issues created during decomposition
  pass validation

## Implementation Plan

### Dependency Graph

```
1. Tracker validation hooks (scripts/tracker.sh)
   ↓
2. Wire hooks into tracker_create_impl and tracker_update_impl
   ↓
3. Skill prompt (skills/helix-triage/SKILL.md)
   ↓
4. CLI command (scripts/helix — triage subcommand)
   ↓
5. Tests (tests/helix-cli.sh)
```

### Issue Breakdown

1. **Tracker validation hooks** — Add `tracker_validate_create` and
   `tracker_validate_update` to `scripts/tracker.sh`. Hard checks for
   required fields, deps, circular deps. Advisory checks for label quality.
   AC: all hard rules from the table above are enforced; tests pass.

2. **Wire validation into tracker mutations** — Call validation hooks from
   `tracker_create_impl` and `tracker_update_impl`. Respect exit codes.
   AC: `ddx bead create` with bad inputs fails with clear error.

3. **Skill prompt** — Create `skills/helix-triage/SKILL.md` with agent
   guidance for issue quality. Reference tracker conventions and label
   taxonomy.
   AC: skill is installable and passes `tests/validate-skills.sh`.

4. **CLI triage subcommand** — Add `helix triage` that wraps
   `ddx bead create` with the skill prompt for interactive use.
   AC: `helix triage "title"` creates a valid issue.

5. **Tests** — Add validation test cases to `tests/helix-cli.sh`.
   AC: all rules from the validation table have test coverage.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Validation breaks existing issue creation in run loop | M | H | Make content checks advisory; only structural checks block |
| Agents ignore skill guidance and create bad issues anyway | M | M | Tracker validation catches structural issues regardless |
| Circular dep detection is O(n²) on large trackers | L | L | Tracker is append-only JSONL, typically < 500 issues |

## Observability

- Validation errors logged to stderr with `[triage]` prefix
- Count of hard errors vs warnings could be added to `ddx bead status`

## Governing Artifacts

- `workflows/TRACKER.md` — tracker field conventions
- `workflows/EXECUTION.md` — run loop issue selection
- `AGENTS.md` — issue tracking rules
- `scripts/tracker.sh` — tracker implementation
- `scripts/helix` — CLI and run loop
