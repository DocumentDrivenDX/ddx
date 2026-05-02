---
title: Glossary
weight: 4
---

Quick definitions for the terms used across the DDx docs.

## Acceptance Criteria

The checked items on a bead that define when the bead is complete. Reviews
check the criteria, not the developer's memory of the conversation. See
[Architecture: Bead Lifecycle](../architecture/#bead-lifecycle).

## Agent Service

The DDx subsystem that dispatches work to AI harnesses behind a unified
prompt envelope. Handles routing, session logging, quorum review, and the
underlying invocation runtime that `ddx run` / `ddx try` / `ddx work`
compose on top of. Surfaced as `ddx agent ...`.

## Bead

A self-contained unit of work with an ID, title, description, acceptance
criteria, labels, and a dependency DAG edge set. Beads live in the project's
bead store as JSONL. Anything an agent does should be against a bead.

## Bead Store

The on-disk JSONL store of beads inside the project (`<projectRoot>/.ddx/beads/`).
Diffable, mergeable, branch-aware. Importable from and exportable to `bd`/`br`.

## Document Library

The structured collection of agent-facing artifacts in a project — prompts,
personas, patterns, templates, MCP server registry, and more. Versioned in
git, syncable across projects.

## `ddx run` / `ddx try` / `ddx work`

The three-layer run architecture. `ddx run` is one agent invocation
(layer 1). `ddx try <bead>` is one bead attempt in an isolated worktree
(layer 2), wrapping one or more `ddx run` invocations. `ddx work` is one
queue drain (layer 3), iterating `ddx try` until a stop condition is met.
See [Run Architecture](../run-architecture/) and **FEAT-010**.

## Harness

A specific agent runtime that the agent service can dispatch to — for
example `claude`, `codex`, `gemini`, or a local `qwen` endpoint. All
harnesses speak the same prompt envelope.

## HELIX

The workflow methodology project that sits on top of DDx. Owns phases,
gates, supervisory dispatch, and bounded actions. HELIX is one valid
workflow on top of DDx; alternatives can exist.

## Dun

The quality check runner project that sits at the top of the cost-tiered
ladder. Owns check discovery, execution, and agent-friendly output of
deterministic verification.

## Persona

A reusable document that shapes how an agent behaves — for example
`code-reviewer`, `implementer`, `test-engineer`, `architect`. Personas are
composed into prompts when their bound role is dispatched.

## Plugin

A package of DDx resources installed under a project. Plugins ship
templates, patterns, prompts, personas, and other artifacts. The default
DDx plugin lives at `<projectRoot>/.ddx/plugins/ddx/` after `ddx init`.

## Project-Local

A DDx design property: install operations only write under `<projectRoot>`,
never under `~/`. Cloning the repo gives a collaborator the full DDx
surface for the project. The single global artifact is `ddx-server`.

## Prompt Envelope

The structured composition of bead context, persona, project config, and
relevant patterns that the agent service hands to a harness. The envelope
is what makes harnesses interchangeable.

## Quorum Review

A multi-agent dispatch mode where several harnesses review the same work
and a configured policy (e.g. `majority`) decides the outcome.

## Ready Queue

The subset of beads whose dependencies are all closed — the work that's
pickable right now. Surfaced via `ddx bead ready` and consumed by
`ddx work`.

## Role

An abstract function (e.g. "the reviewer", "the implementer") that a
project binds to a specific persona in `.ddx.yml`. `ddx try` and `ddx work`
dispatch by role; bindings decide which persona, and which tier of model,
fulfills it.

## Skill

An agent-facing capability surface installed under `<projectRoot>/.claude/skills/`
or `<projectRoot>/.agents/skills/`. DDx ships a single consolidated `ddx`
skill rather than a fleet of small ones.

## Three-Layer Stack

The architectural separation DDx is one layer of: **DDx** (platform
primitives), **HELIX** (workflow methodology), **Dun** (deterministic
quality checks). Each layer is independently useful and replaceable. See
[the overview](../).

## Worktree (Isolated)

A dedicated git worktree `ddx try` creates per bead attempt so the agent's
edits never collide with the user's working tree. The worktree is merged
back to the base ref on success or preserved for diagnosis on failure.
