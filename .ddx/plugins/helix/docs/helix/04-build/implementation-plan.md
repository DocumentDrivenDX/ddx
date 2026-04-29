---
dun:
  id: helix.implementation-plan
  depends_on:
    - helix.workflow
    - TP-002
    - TD-002
---
# Implementation Plan

## Scope

This build plan defines the current HELIX execution landscape: which features
are implemented, which have remaining work, what the governing artifacts are,
and what ordering constraints keep implementation aligned with the authority
stack.

**Governing Artifacts**:
- `docs/helix/01-frame/prd.md`
- `docs/helix/01-frame/features/FEAT-001-helix-supervisory-control.md`
- `docs/helix/01-frame/features/FEAT-002-helix-cli.md`
- `docs/helix/01-frame/features/FEAT-003-principles.md`
- `docs/helix/01-frame/features/FEAT-004-plugin-packaging.md`
- `docs/helix/01-frame/features/FEAT-005-execution-backed-output.md`
- `docs/helix/01-frame/features/FEAT-006-concerns-practices-context-digest.md`
- `docs/helix/02-design/adr/ADR-001-supervisory-control-model.md`
- `docs/helix/02-design/solution-designs/SD-001-helix-supervisory-control.md`
- `docs/helix/02-design/solution-designs/SD-002-first-class-principles.md`
- `docs/helix/02-design/technical-designs/TD-002-helix-cli.md`
- `docs/helix/02-design/contracts/API-001-helix-tracker-mutation.md`
- `docs/helix/03-test/test-plans/TP-002-helix-cli.md`
- `workflows/README.md`
- `workflows/EXECUTION.md`

## Shared Constraints

- Keep execution tracker-first. Active work must be selected, claimed,
  revalidated, and closed through `ddx bead`.
- Keep `spec-id` anchored to the nearest governing artifact.
- Limit implementation to bounded slices that satisfy documented acceptance and
  deterministic verification. Do not fold design reconciliation or new product
  scope into build issues.
- Use the proof lane for HELIX contract changes:
  `bash tests/helix-cli.sh`, `bash tests/validate-skills.sh`, and
  `git diff --check`.

## Feature Status

| Feature | Status | Remaining Work |
|---------|--------|---------------|
| **FEAT-001** Supervisory Control | ALIGNED | None — fully implemented, 87+ tests |
| **FEAT-002** HELIX CLI | ALIGNED | Incremental quality work only |
| **FEAT-003** First-Class Principles | COMPLETE | Defaults rewritten, scaffolding updated, injection wired, bootstrap in frame |
| **FEAT-004** Plugin Packaging | PARTIAL | Manifest exists; `bin/helix` wrapper and validator updates need implementation |
| **FEAT-005** Execution-Backed Output | BLOCKED | Depends on DDx FEAT-010 (`ddx exec`); epic `helix-4dec7483` open |
| **FEAT-006** Concerns & Context Digest | SPECIFIED | Feature spec, reference docs, concern library, and action prompt wiring complete; no runtime implementation yet |

## Current Open Issues

### Execution-eligible work

| Issue | Priority | Feature | Goal | Dependencies |
|-------|----------|---------|------|-------------|
| hx-bf99e0ee | P2 | FEAT-002 | Deduplicate blocker notes for unchanged issues | None |
| hx-407ed8b8 | P2 | FEAT-002 | UX: helix start/stop for background operation | None (design phase) |

### Blocked work

| Issue | Priority | Feature | Blocked By |
|-------|----------|---------|-----------|
| helix-4dec7483 | P1 | FEAT-005 | DDx FEAT-010 (`ddx exec` not available) |
| helix-6ae1e84c | P2 | FEAT-005 | Parent epic blocked |
| helix-5f846566 | P2 | FEAT-002 | Needs concern-aware review to file structured beads |
| hx-44a5dbfe | P3 | FEAT-002 | Review note dedup requires alignment review changes |

## Build Sequencing

| Order | Area | Governing Artifacts | Notes |
|-------|------|-------------------|-------|
| 1 | FEAT-006 runtime implementation | FEAT-006, concern-resolution.md, context-digest.md | Wire concern loading and digest assembly into triage, polish, evolve CLI paths |
| 2 | FEAT-004 plugin completion | FEAT-004, SD-001 | Create `bin/helix`, update `validate-skills.sh` for plugin layout |
| 3 | FEAT-005 execution capture | FEAT-005 | Blocked on DDx FEAT-010; defer until `ddx exec` ships |
| 4 | Quality backlog | FEAT-002, TP-002 | Blocker note dedup, review evidence dedup, helix start/stop UX |

## Verification Expectations

| Area | Required Verification |
|------|-----------------------|
| Concern/digest runtime | Action prompts load concerns; triage assembles digest; polish refreshes; manual scenario verification |
| Plugin packaging | `bash tests/validate-skills.sh`; plugin manifest validation; `bin/helix` delegation test |
| CLI contract changes | `bash tests/helix-cli.sh`; `git diff --check` |
| Documentation updates | `git diff --check`; cross-reference audit against governing artifacts |

## Quality Gates

- [ ] Failing or governing tests exist before implementation starts for code changes.
- [ ] All required HELIX verification commands pass before a build issue closes.
- [ ] Issue metadata cites the governing artifacts and this build plan.
- [ ] Behavior or contract changes update the canonical HELIX docs explicitly.
- [ ] Follow-on work outside the current issue scope is captured as new tracker
      issues instead of absorbed silently.

## Risks

| Risk | Impact | Response |
|------|--------|----------|
| FEAT-005 stays blocked indefinitely on DDx FEAT-010 | Medium | HELIX operates without execution capture; status reporting limited to run-state.json |
| FEAT-006 concern loading adds per-prompt overhead | Low | Token budget targets ~1000-1500 tokens; size ceiling on concerns mirrors principles ceiling |
| Plugin mode requires Claude Code features not yet available | Medium | Symlink installer remains as fallback; plugin manifest ready when features ship |

## Exit Criteria

- [ ] Current open issues are sequenced with explicit governing artifacts and
      dependencies.
- [ ] This plan reflects the live tracker queue (no stale issue references).
- [ ] Verification expectations are explicit enough for bounded issue closure.
