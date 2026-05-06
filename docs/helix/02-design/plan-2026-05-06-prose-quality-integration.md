---
ddx:
  id: PLAN-2026-05-06-prose-quality-integration
  depends_on:
    - FEAT-027
    - TD-036
  status: draft
---
# Plan: Prose Quality Integration Reset

## Purpose

DDx needs one opinionated prose-quality path, not a menu of optional linters.
The public surface is `ddx doc prose`; the checker engine is an implementation
detail. If a third-party checker cannot be made to feel like part of DDx, it is
not the right default engine.

## Goal

DDx should automatically improve project documents by making prose more
specific, reviewable, and executable. The system should catch common
LLM-default prose constructions, preserve DDx terminology and technical
structure, and run quietly in normal document workflows.

## Component Lanes

### Deterministic checks

- Select exactly one default checker engine.
- Keep DDx-owned finding fields: file, line, rule id, severity, rationale, and
  suggested edit.
- Build a labeled DDx prose corpus from real docs and synthetic edge cases.
- Evaluate candidate engines as internal implementation options, not public
  runner choices.
- Reject any engine that requires user-managed language runtimes, package
  managers, project-local config files, or manual setup.

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

The spikes feed an ADR, not a TD. The ADR should choose the prose checker
engine and record rejected alternatives. A TD is only warranted after the ADR
if the chosen path requires architecture beyond a small command/checker
implementation.

## Hard Constraints

- DDx remains an opinionated single-binary product from the user's point of
  view.
- No public matrix of optional prose tools.
- No project-level checker config required for the default path.
- No Python, Node, Java, or external package-manager dependency in the default
  installation path.
- Missing or broken internals produce a DDx diagnostic, not third-party setup
  homework.

