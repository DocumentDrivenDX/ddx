---
title: Spec-First Development
weight: 1
---

# Spec-first development

> Specifications, not code, are the durable artifact in a document-driven
> factory.

## The principle

Code is a perishable rendering of a spec into a particular language,
framework, and runtime — regenerable, replaceable, and often shorter-lived
than the requirement it serves. When the spec is the source of truth, AI
agents and human contributors share a stable target, reviews focus on
intent rather than syntax, and refactors stop being existential. The
alternative — code-first prompting where requirements live in chat history
and PR descriptions — produces software no one can confidently change,
because the "why" decays the moment the conversation scrolls.

A factory built on specs treats every implementation as a re-pour from the
mold; a factory built on code treats every change as archaeology. DDx
chooses the mold. The discipline is operational, not aspirational: the
spec is the artifact that gets edited, branched, reviewed, and merged,
and code generation runs downstream of an approved spec rather than
alongside it.

The inversion only works if specs are complete enough to actually drive
generation, which is why a spec-first system is also a tooling system —
required sections, ambiguity markers, gate checklists, and dependency
edges that the agent must satisfy before moving on. Spec-first is not a
cultural request; it is a tooling constraint.

## Evidence

- **REF-005 — GitHub Spec Kit.** Ships a `/specify → /plan → /tasks`
  workflow that treats the PRD as executable input: the spec generates the
  plan, the plan generates the task list, and only then does code
  generation begin.
- **REF-006 — AWS Kiro.** Embeds the same loop inside an IDE: natural-language
  requirements become structured specs, design notes, and implementation
  steps before any code is produced.
- **RSCH-001.** Synthesis of REF-005 and REF-006 — when a requirement
  changes, the system re-derives the plan, contracts, and tests rather
  than asking the developer to manually propagate the change through
  stale code.

See `docs/helix/00-discover/research/RSCH-001-spec-first-development.md`
for the full synthesis.

## DDx response

- **HELIX frame artifacts** (vision, PRD, feature specs, principles) are
  first-class documents in `docs/helix/`, versioned with the code.
- **Bead descriptions and acceptance criteria** are the executable
  contract a downstream agent runs against. `ddx bead create` captures
  intent before work starts.
- **`ddx.depends_on` edges** in artifact frontmatter wire every bead
  back to its governing spec and the references behind it.
- **The document library** (prompts, personas, templates, patterns)
  exists because the things AI agents read are first-class artifacts
  that need version control, dependency tracking, and audit.

DDx does not encode any specific spec template — that is a workflow
choice, owned by HELIX and other plugins. DDx provides the substrate:
typed artifacts, dependency edges, addressable IDs, and a queue that
respects them.
