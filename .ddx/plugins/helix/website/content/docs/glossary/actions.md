---
title: Actions
weight: 3
prev: /docs/glossary/artifacts
next: /docs/glossary/concerns
---

# HELIX Actions

Actions are bounded supervisory commands that drive HELIX execution. Each action does one thing and exits. The `helix run` supervisor chains them based on queue state.

## Execution Actions

These actions do work or decide what work to do next.

### build

Execute one ready tracker issue end-to-end.

| | |
|---|---|
| **Command** | `helix build [issue-id\|scope]` |
| **Alias** | `helix implement` |
| **Prompt** | `workflows/actions/implementation.md` |
| **Modifies** | Source code, tests, docs, tracker |

Claims a ready issue, loads governing artifacts, implements the work, runs verification (including concern-declared quality gates), commits with traceability, and closes the issue. The core execution primitive.

### check

Inspect queue health and recommend the next action.

| | |
|---|---|
| **Command** | `helix check [scope]` |
| **Prompt** | `workflows/actions/check.md` |
| **Modifies** | Nothing (read-only) |
| **Returns** | `NEXT_ACTION` decision code |

Decision codes: `BUILD`, `DESIGN`, `POLISH`, `ALIGN`, `BACKFILL`, `WAIT`, `GUIDANCE`, `STOP`. The supervisor uses this to route the loop.

### run

The supervisory autopilot. Chains check, build, review, and other actions until human input is needed.

| | |
|---|---|
| **Command** | `helix run [--max-cycles N] [--review-every N]` |
| **Prompt** | Orchestrates other actions |
| **Modifies** | Everything (delegates to other actions) |

Runs the loop: check queue state, select action, execute, optionally review, repeat. Stops on `WAIT`, `GUIDANCE`, or `STOP`.

### status

Report the current run-controller state.

| | |
|---|---|
| **Command** | `helix status` |
| **Modifies** | Nothing (read-only) |

Shows current issue, blockers, cycle timing, token usage, and next recommended action.

---

## Steering Actions

These actions shape the work before or after execution.

### frame

Create or refine product vision, PRD, feature specs, and user stories.

| | |
|---|---|
| **Command** | `helix frame [scope]` |
| **Prompt** | `workflows/actions/frame.md` |
| **Modifies** | `docs/helix/00-discover/`, `docs/helix/01-frame/`, tracker |

Operates at authority levels 1-3. Also bootstraps [principles](/docs/glossary/concepts#principles) and [concerns](/docs/glossary/concerns) if missing.

### design

Create a comprehensive design document through iterative self-critique.

| | |
|---|---|
| **Command** | `helix design [scope] [--rounds N]` |
| **Alias** | `helix plan` |
| **Prompt** | `workflows/actions/plan.md` |
| **Modifies** | `docs/helix/02-design/`, tracker |

Produces architecture decisions, interface contracts, data models, error handling, security considerations, test strategy, and implementation ordering. Default 5 refinement rounds with convergence detection.

### evolve

Thread a requirement change through the entire artifact stack.

| | |
|---|---|
| **Command** | `helix evolve "requirement description"` |
| **Prompt** | `workflows/actions/evolve.md` |
| **Modifies** | All governing artifacts, tracker |

Takes a new requirement, detects conflicts with existing artifacts and concerns, updates documents top-down (highest authority first), creates tracker issues for implementation.

### polish

Refine tracker issues before implementation begins.

| | |
|---|---|
| **Command** | `helix polish [scope] [--rounds N]` |
| **Prompt** | `workflows/actions/polish.md` |
| **Modifies** | Tracker issues |

Deduplicates issues, verifies plan coverage, sharpens acceptance criteria, fixes dependencies, enforces area labels for concern matching, and checks that acceptance criteria reference the correct concern tools.

### triage

Create tracker issues with proper governing artifact references.

| | |
|---|---|
| **Command** | `helix triage "title" [--type task\|chore\|decision]` |
| **Modifies** | Tracker |

Assembles a [context digest](/docs/glossary/concepts#context-digest) and prepends it to the bead description.

---

## Quality Actions

These actions verify and improve completed work.

### review

Post-implementation fresh-eyes review.

| | |
|---|---|
| **Command** | `helix review [last-commit\|issue-id\|file-path]` |
| **Prompt** | `workflows/actions/fresh-eyes-review.md` |
| **Modifies** | Tracker (creates finding issues), possibly AGENTS.md |

Four passes: correctness, integration, concern-aware quality (security, tech-stack drift, observability), and operational learnings. Files actionable findings as tracker issues.

### align

Top-down reconciliation review of plan vs. implementation.

| | |
|---|---|
| **Command** | `helix align [scope]` |
| **Prompt** | `workflows/actions/reconcile-alignment.md` |
| **Modifies** | Tracker, `docs/helix/06-iterate/alignment-reviews/` |

Reconstructs intent from planning artifacts, validates traceability, detects concern drift across all layers (docs, designs, code), classifies gaps, and creates execution issues for follow-up work.

### experiment

Metric-driven optimization within a bounded iteration.

| | |
|---|---|
| **Command** | `helix experiment [issue-id\|goal] [--close]` |
| **Prompt** | `workflows/actions/experiment.md` |
| **Modifies** | Source code (scoped files only), tracker |

Hypothesize, edit, test, benchmark, keep or discard. Tracks results in JSONL with confidence scoring. Squash-merges kept changes on close.

### backfill

Reconstruct missing HELIX documentation from existing evidence.

| | |
|---|---|
| **Command** | `helix backfill [scope]` |
| **Prompt** | `workflows/actions/backfill-helix-docs.md` |
| **Modifies** | `docs/helix/`, tracker |

Research-first reconstruction: inventories evidence, extracts findings bottom-up, drafts artifacts top-down with confidence levels. Gates low-confidence content on user confirmation. Also discovers and proposes active concerns if `concerns.md` is missing.

---

## Utility Actions

### commit

Commit with HELIX-compliant message format and pre-push build gate.

| | |
|---|---|
| **Command** | `helix commit` |
| **Modifies** | Git history, tracker |

Auto-stages, runs the full quality gate, commits with issue traceability, pushes, and closes the tracker issue.

### next

Recommend the next issue to work on.

| | |
|---|---|
| **Command** | `helix next` |
| **Modifies** | Nothing (read-only) |

Examines the ready queue and suggests the best candidate based on dependency ordering, governing artifact clarity, and critical path position.
