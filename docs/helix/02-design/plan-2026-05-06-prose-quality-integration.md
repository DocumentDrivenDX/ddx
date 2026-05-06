---
ddx:
  id: PLAN-2026-05-06-prose-quality-integration
  depends_on:
    - FEAT-027
    - TD-036
    - ADR-025
  status: draft
---
# Plan: Prose Quality Integration Reset

## Purpose

DDx needs one opinionated prose-quality path, not a menu of optional linters.
The public surface is `ddx doc prose`; Vale is the pinned deterministic engine
behind that surface per ADR-025. DDx owns the rules, output, workflow behavior,
and diagnostics.

## Goal

DDx should automatically improve project documents by making prose more
specific, reviewable, and executable. The system should catch common
LLM-default prose constructions, preserve DDx terminology and technical
structure, and run quietly in normal document workflows.

## Component Lanes

### Deterministic checks

- Integrate Vale 3.13.0 as the default checker engine behind `ddx doc prose`.
- Keep DDx-owned finding fields: file, line, rule id, severity, rationale, and
  suggested edit.
- Build a labeled DDx prose corpus from real docs and synthetic edge cases.
- Generate temporary Vale config from DDx-packaged rules instead of requiring
  project-local `.vale.ini`.
- Normalize Vale JSON into DDx findings.
- Add `ddx doctor` validation for the pinned Vale version on `PATH`.

### Agent skill

- Treat `human-writing-support` as the agent workflow for prose work.
- Instruct agents to run `ddx doc prose --changed` after Markdown edits under
  `docs/`.
- Teach agents to fix high-signal findings and preserve legitimate technical
  density.
- Teach agents to identify common AI-slop constructions without treating
  findings as authorship claims.

### DDx workflow hooks

- Trigger prose review automatically when docs change in normal DDx workflows.
- Keep findings advisory by default.
- Feed findings to agents before finalization so obvious issues are fixed
  without operator reminders.
- Store concise prose-check evidence in execution artifacts when the check runs
  as part of a DDx attempt.

## Required Spikes

1. `SPIKE-2026-05-06-prose-checker-engine-selection` compares candidate
   engines against DDx requirements.
2. `SPIKE-2026-05-06-vale-internal-engine` tests whether Vale can be packaged,
   invoked, and normalized as a native DDx implementation detail.
3. `SPIKE-2026-05-06-prose-workflow-integration` defines where prose checks run
   automatically and what agents/users see.

## Decision Flow

The spikes fed ADR-025. Implementation can proceed from that ADR without a TD
unless the Vale adapter requires broader architecture than command invocation,
temporary config generation, JSON normalization, and workflow wiring.

## Hard Constraints

- DDx remains an opinionated single-binary product from the user's point of
  view.
- No public matrix of optional prose tools.
- No project-level checker config required for the default path.
- No Python, Node, Java, or external package-manager dependency in the default
  installation path.
- Missing or broken internals produce a DDx diagnostic, not third-party setup
  homework.
- Vale installation is delegated to Vale's official release/install channel,
  pinned by DDx, and verified by `ddx doctor`.
