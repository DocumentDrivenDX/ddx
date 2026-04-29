# Design Plan: helix-evolve Skill

**Date**: 2026-03-31
**Status**: CONVERGED
**Refinement Rounds**: 1

## Problem Statement

New requirements arrive continuously — feature requests, constraint changes,
spec amendments, customer feedback, incident learnings. Currently there's no
structured path to thread a new requirement through the existing artifact
stack. The operator must manually:

1. Figure out which governing artifacts need updating
2. Update them in the right order (authority stack)
3. Create tracker issues for the implementation work
4. Wire dependencies to existing open issues

This is error-prone: artifacts get updated without propagating downstream,
issues get created without governing context, and existing work conflicts
with new requirements silently.

`helix-evolve` takes a requirement and propagates it through the full
artifact stack, from vision down to tracker issues, respecting the authority
order and surfacing conflicts with existing work.

## Requirements

### Functional

1. **Accept requirements in multiple forms**: natural language description,
   spec reference, customer feedback quote, incident postmortem action item,
   or amendment to an existing artifact.

2. **Identify affected artifacts**: determine which documents in the authority
   stack need updating — feature specs, SDs, ADRs, TDs, TPs, acceptance
   manifests.

3. **Update artifacts in authority order**: higher-authority docs first.
   Product vision/requirements → feature specs → architecture/ADRs →
   solution designs → test plans → acceptance manifests.

4. **Detect conflicts**: when the new requirement contradicts existing
   artifacts or open tracker issues, surface the conflict clearly with
   specific references. Don't silently overwrite.

5. **Create tracker issues**: decompose the implementation work into
   properly triaged issues (passing helix-triage validation) with spec-ids
   pointing to the updated artifacts.

6. **Wire dependencies**: connect new issues to existing open issues where
   there's overlap or ordering dependency.

7. **Produce an evolution report**: structured output showing what changed,
   what was created, and what conflicts need human resolution.

### Non-Functional

- Must work incrementally — evolving one requirement shouldn't require
  re-reading the entire artifact stack.
- Should handle both small amendments ("add a config flag for X") and
  large feature additions ("support multi-region replication").
- Conflicts should halt the affected artifact update, not the whole
  evolution.

### Constraints

- Implemented as a skill prompt + CLI subcommand (like helix-plan).
- Uses the agent to do the actual reading/writing — the skill provides
  the workflow, not the implementation.
- Must respect NIFLHEIM_ACCEPTANCE_CHANGE=1 gate when updating acceptance
  artifacts.

## Architecture Decisions

### Decision 1: Skill scope — agent-driven or script-driven

- **Question**: Should the skill be a prompt that guides the agent, or a
  script that orchestrates steps programmatically?
- **Alternatives**:
  - A) Pure prompt — agent reads, decides, writes
  - B) Script orchestration — script identifies files, agent fills content
  - C) Prompt with structured phases — agent follows phases but makes all
    decisions
- **Chosen**: C — structured phases in a prompt
- **Rationale**: The artifact identification and conflict detection require
  judgment that a script can't provide. But structured phases keep the
  agent focused and prevent drift. Same pattern as helix-implement.

### Decision 2: Conflict handling

- **Question**: What happens when the new requirement conflicts with
  existing artifacts?
- **Alternatives**:
  - A) Halt entirely — user resolves all conflicts first
  - B) Skip conflicting artifacts, proceed with non-conflicting ones
  - C) Update non-conflicting artifacts, flag conflicts in the report
- **Chosen**: C — proceed and flag
- **Rationale**: Most requirements touch multiple artifacts. A conflict
  in one (e.g., an ADR) shouldn't block updating the test plan. The
  evolution report makes conflicts visible for human resolution.

### Decision 3: Artifact discovery

- **Question**: How does the skill find which artifacts to update?
- **Alternatives**:
  - A) User specifies affected artifacts
  - B) Agent searches the artifact tree based on the requirement scope
  - C) Both — user can hint, agent discovers
- **Chosen**: C — user hints, agent discovers
- **Rationale**: The user often knows the general area ("this affects the
  WAL") but not every artifact that references it. The agent should search
  for cross-references.

## Interface Contracts

### CLI Surface

```bash
helix evolve "requirement description"
helix evolve --scope area:wal "Add WAL compression support"
helix evolve --artifact SD-017 "Amend verification engine to support loom"
helix evolve --from incident "INC-042 showed we need circuit breakers in ingest"
```

### Skill Prompt Phases

**Phase 1 — Requirement Analysis**: Parse the input. Identify the core
requirement, its scope, and its type (new feature, amendment, constraint,
bugfix policy).

**Phase 2 — Artifact Discovery**: Search the docs tree for governing
artifacts in the requirement's scope. Build a list ordered by authority
(highest first).

**Phase 3 — Conflict Detection**: For each affected artifact, check
whether the new requirement contradicts existing content. Flag conflicts
with specific line references.

**Phase 4 — Artifact Evolution**: Update each non-conflicting artifact
in authority order. For each update:
- Read the current artifact
- Draft the amendment
- Validate it doesn't contradict higher-authority artifacts already updated
- Write the update

**Phase 5 — Issue Decomposition**: Create tracker issues for the
implementation work implied by the updated artifacts. Use helix-triage
validation. Set spec-ids to the updated artifacts.

**Phase 6 — Dependency Wiring**: Search existing open issues for overlap
with new issues. Add dependencies where ordering matters.

**Phase 7 — Evolution Report**: Output a structured report:
- Requirement summary
- Artifacts updated (with diff summaries)
- Artifacts skipped due to conflicts (with conflict descriptions)
- Issues created (with IDs and acceptance criteria)
- Dependencies wired
- Open conflicts requiring human resolution

### Output Trailer

```
EVOLUTION_STATUS: COMPLETE|CONFLICTS|BLOCKED
ARTIFACTS_UPDATED: N
ISSUES_CREATED: N
CONFLICTS: N
```

## Data Model

No new data structures. Uses existing:
- `docs/helix/` artifact tree
- `.helix/issues.jsonl` tracker
- Git for artifact versioning

## Error Handling

- Artifact read failures: skip artifact, flag in report
- Triage validation failures: fix metadata and retry once, then flag
- Git conflicts: stage changes per-artifact, flag any that can't merge
- Agent timeout: report partial progress, list what wasn't completed

## Security

N/A — local artifacts and tracker.

## Test Strategy

- **Unit**: Add to `tests/helix-cli.sh`:
  - `helix evolve --dry-run` produces the expected prompt
  - Prompt references the evolve action
- **Integration**: Manual — evolve a requirement in a test project and
  verify artifacts updated, issues created
- **E2E**: Verified by running in niflheim

## Implementation Plan

### Dependency Graph

```
1. Action prompt (workflows/actions/evolve.md)
   ↓
2. Skill prompt (skills/helix-evolve/SKILL.md)
   ↓
3. CLI subcommand (scripts/helix — evolve)
   ↓
4. Tests
```

### Issue Breakdown

1. **Action prompt** — Write `workflows/actions/evolve.md` defining the
   7-phase workflow. Reference authority order, artifact locations,
   tracker conventions.
   AC: action prompt exists; phases match this design; references are valid.

2. **Skill prompt** — Create `skills/helix-evolve/SKILL.md` with name,
   description, argument-hint. Reference the action.
   AC: passes validate-skills.sh; installed in .agents/skills.

3. **CLI subcommand** — Add `helix evolve` to scripts/helix. Build the
   prompt, dispatch to agent.
   AC: `helix evolve --dry-run "test"` prints the agent command;
   `helix evolve --help` shows usage.

4. **Tests** — Dry-run and prompt content tests in helix-cli.sh.
   AC: tests pass.

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Agent updates artifact incorrectly | M | H | Phase 4 validates against higher-authority artifacts already updated |
| Evolution touches too many artifacts | M | M | Scope flag limits blast radius; agent can ask for guidance |
| Conflicts not detected | L | H | Phase 3 explicitly checks before writing |

## Observability

- Evolution report written to `docs/helix/06-iterate/evolution-reports/`
- Trailer lines for machine parsing by the run loop

## Governing Artifacts

- Authority order: `workflows/actions/implementation.md` (Phase 2)
- Artifact locations: project-specific `docs/helix/` tree
- Tracker conventions: `workflows/TRACKER.md`
- Triage validation: `scripts/tracker.sh`
