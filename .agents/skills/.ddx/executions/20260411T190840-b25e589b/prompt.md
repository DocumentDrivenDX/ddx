# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-ffda4fb5`
- Title: Align host+user ddx-server planning stack and test strategy
- Parent: `ddx-8b6cd40e`
- Labels: helix, phase:planning, kind:architecture, area:api, area:agent, area:ui, area:docs
- spec-id: `FEAT-002`
- Base revision: `e0056f1f33d531d3539a2dcce9518ec8f88e6fad`
- Execution bundle: `.ddx/executions/20260411T190840-b25e589b`

## Description
<context-digest>
Review area: host+user ddx-server planning stack after the server model evolved beyond the earlier multi-project topology draft. Evidence covers FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, TP-002, the fresh-eyes server review, and the updated requirements for a per-user host daemon, localhost HTTP UI, SQLite-backed runtime/index state, embedded-agent progress visibility, provider dashboarding, replay-backed fixtures, and Playwright coverage for every new UI surface.
</context-digest>
Update the governing server planning docs so they describe the current intended host+user ddx-server model before follow-on server design and implementation beads continue.

## Goals
- Reconcile FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, and TP-002 around a single host+user ddx-server instance with explicit project registry, worker management boundaries, and queue semantics
- Define the split between git-backed project truth, tracked execution evidence, and host-local runtime/index state backed by an embedded database
- Define the current transport/security model: localhost unauthenticated HTTP for UI/API now, explicit future transport/auth evolution later
- Add the embedded-agent progress, provider dashboard, replay-backed fixture, and Playwright coverage requirements to the governed server plan
- Make the scheduling boundary explicit: server may allocate workers across projects, but bead ordering remains owned by each project queue

## Boundaries
- Keep Axon and multi-host service-backed topology as later-stage work
- Do not reopen the already-shipped single-project execute-bead loop contract except where the new server plan must reference it

## Acceptance Criteria
FEAT-002, FEAT-008, FEAT-013, FEAT-014, SD-019, and TP-002 all describe the same host+user ddx-server model, including runtime/index storage, worker boundaries, provider dashboarding, replay-backed fixtures, and mandatory Playwright coverage for new UI surfaces

## Governing References
- `FEAT-002` — `docs/helix/01-frame/features/FEAT-002-server.md` (Feature: DDx Server)

## Execution Rules
1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.
5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. DDx owns landing and preservation. Agent-created commits are optional; coherent tracked edits in the worktree still count as produced work.
8. If you choose to create commits, keep them coherent and limited to this bead. If you leave tracked edits without commits, DDx will still evaluate them.
9. If the work is already satisfied with no tracked changes needed, stop cleanly and let DDx record a no-change attempt instead of inventing a commit.
