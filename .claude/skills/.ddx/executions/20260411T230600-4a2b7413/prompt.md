# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-cb418dcf`
- Title: Align specs for prompt templates, tracked execution artifacts, and commit provenance
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:git, area:docs
- spec-id: `FEAT-006`
- Base revision: `3727433094e2e953f913ef952d8ea5bff756c755`
- Execution bundle: `.ddx/executions/20260411T230600-4a2b7413`

## Description
<context-digest>
Review area: governing contract for execute-bead prompts and execution evidence. Evidence covers the current FEAT-006/API-001 workflow, the prompt fallback bug, the desire to keep beads authored and relatively stable, and the new requirement that execution evidence be tracked in git so autoresearch can replay, compare, and analyze runs.
</context-digest>
Align the governing docs around one execute-bead evidence contract.

## Goals
- Define the execute-bead prompt template as a DDx-owned rationalizer over bead fields plus resolved governing references, not a bead mutation and not a speculative summary of whole specs
- Define the stable tracked artifact set for each execution attempt: prompt, manifest, result, checks, normalized log, and usage/provider pointers
- Define the split between tracked execution evidence and ignored local scratch/runtime state
- Define the default commit policy for successful and preserved attempts
- Define the rule that all programmatically-added commit message metadata must be rendered from tracked machine-readable files

## Acceptance Criteria
FEAT-006, FEAT-012, FEAT-014, FEAT-015, and API-001 describe the same contract: execute-bead compiles a deterministic prompt from bead data plus resolved references; each attempt produces tracked machine-readable execution artifacts; ignored runtime scratch is clearly separated from tracked evidence; and commit-message metadata is projected from tracked files rather than ad hoc runtime state

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.
8. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt.
