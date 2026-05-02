---
ddx:
  id: RSCH-001
  status: draft
  depends_on:
    - REF-005
    - REF-006
id: RSCH-001
title: "Spec-First Development"
kind: research-synthesis
summary: "Specifications, not code, are the durable artifact AI agents consume; tooling that treats spec → plan → tasks as the primary workflow outperforms code-first prompting."
tags: [spec-driven, methodology, ddx-principle]
---

# Spec-First Development

## Principle

The specification is the source of truth. Code is the regenerable expression of
a spec at a moment in time, in a particular language and framework.

## Synthesis

Two independent industrial efforts have converged on this inversion. GitHub's
Spec Kit (REF-005) ships a `/specify → /plan → /tasks` workflow that treats the
PRD as executable input — the spec generates the plan, the plan generates the
task list, and only then does code generation begin. AWS Kiro (REF-006) embeds
the same loop inside an IDE: natural-language requirements become structured
specs, design notes, and implementation steps before any code is produced. Both
tools share the bet that AI assistants regress to plausible-looking but
incorrect code when they are pointed at a vague request, and that the way to
discipline them is to first nail down a precise, reviewable spec.

The evidence for the inversion is operational rather than statistical: when
Spec Kit and Kiro users change a requirement, the system re-derives the plan,
contracts, and tests rather than asking the developer to manually propagate
the change through stale code. This is the failure mode that traditional
"docs alongside code" workflows accept as inevitable — drift between intent
and implementation — and the inversion eliminates it by making the spec the
artifact that gets edited, branched, reviewed, and merged.

The inversion only works if specs are complete enough to actually drive
generation. Both REF-005 and REF-006 force this by template: required
sections, explicit `[NEEDS CLARIFICATION]` markers when the prompt is
ambiguous, and gate checklists ("requirements are testable", "no speculative
features") that the agent must satisfy before moving on. The lesson is that
spec-first development is not a cultural request; it is a tooling constraint.

## DDx Implication

DDx adopts spec-first as a load-bearing assumption. The document library
(prompts, personas, templates, patterns) and the bead tracker exist precisely
because the things AI agents read — specs, acceptance criteria, dependency
edges — are first-class artifacts that need version control, dependency
tracking, and audit. `ddx bead create` captures intent before work starts;
the bead description and AC are the contract the agent executes against, the
same way Spec Kit's `spec.md` is the contract its task agents execute against.

DDx does not encode any specific spec template (HELIX provides those); it
provides the substrate — typed artifacts, dependency edges, addressable IDs —
that any spec-first workflow needs to function.
