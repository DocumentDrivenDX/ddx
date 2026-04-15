# Execute Bead

You are running inside DDx's isolated execution worktree for this bead.
Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.

## Bead
- ID: `ddx-4a91da5d`
- Title: Adopt XML-tagged execute-bead prompt template
- Parent: `ddx-32b3008e`
- Labels: helix, phase:planning, kind:architecture, area:agent, area:exec, area:docs
- spec-id: `FEAT-006`
- Base revision: `c5ce6c8e5cb8dcca3ee64d8e805e62e55eb44c07`
- Execution bundle: `.ddx/executions/20260411T164126-7e61cfc8`

## Description
<context-digest>
Review area: execute-bead prompt-template structure. Evidence covers the shipped synthesized prompt in cli/cmd/agent_execute_bead.go, the new tracked execution bundle under .ddx/executions/, and the need for a more deterministic machine-readable prompt contract for replay, diffing, and future autoresearch.
</context-digest>
Evolve the synthesized execute-bead prompt from markdown-heading sections to an explicit XML-tagged structure.

## Goals
- Replace markdown section headings in the synthesized execute-bead prompt with XML-style tags for machine-significant structure
- Preserve the current human-readable content while making the prompt easier to parse, diff, and validate deterministically
- Define which sections are required, optional, and repeatable in the prompt template
- Keep the prompt aligned with the tracked execution manifest and future commit-provenance rendering
- Update the governing specs and contracts so the prompt-template contract is documented rather than only implied by code

## Required spec work
- Update FEAT-006 and any applicable execute-bead contract docs to define the prompt-template structure
- Align the execution-evidence lane docs so prompt.md is explicitly structured, machine-readable evidence

## Required implementation work
- Update prompt synthesis in cli/cmd/agent_execute_bead.go to emit XML-tagged sections
- Update automated coverage to assert the new tagged structure and prevent markdown-header regression

## Acceptance Criteria
FEAT-006 and the execute-bead contract docs define an XML-tagged prompt template for execute-bead; cli/cmd/agent_execute_bead.go emits that tagged structure into prompt.md; and tests verify the required tags and reject regression to markdown-heading-only prompt structure

## Governing References
- `FEAT-006` — `docs/helix/01-frame/features/FEAT-006-agent-service.md` (Feature: DDx Agent Service)

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
