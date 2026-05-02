---
title: Software Factory
weight: 6
---

DDx frames itself as a **software factory**. This page explains what that
phrase means here, the lineage it draws on, what DDx adds, and — equally
important — what DDx deliberately leaves out.

## Definition

A **software factory** is a system in which software construction is
industrialized: reusable assets, explicit contracts, and tooling drive the
production line, instead of every project rebuilding scaffolding from scratch.
The "factory" is not a building or a vendor — it is the combination of
**documents, schemas, templates, and runners** that turn an intent into a
shipped change.

In DDx the production line is concrete and on-disk:

- **Specs** are the raw input.
- **Beads** are the work tickets that flow through the line.
- **Personas, patterns, prompts, and templates** are the reusable assets
  agents draw from.
- **Harnesses** (Claude, Codex, Gemini, local models) are the tools at each
  station.
- **Evidence** — execution logs, review verdicts, merged commits — is the
  durable output, not just the code.

## Lineage

DDx's framing synthesizes three traditions, each with prior art the project
explicitly cites.

### What DDx inherits from the 2000s Microsoft tradition

The phrase *software factories* traces directly to Greenfield, Short, Cook,
and Kent's 2004 book — see
[REF-007 — Software Factories](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-007-software-factories.md).
That work argued software construction should be industrialized through
**model-driven development, domain-specific languages, and reusable
assets** — schemas and patterns that capture how a class of systems is
built so each new project does not start from zero.

DDx inherits the load-bearing claims:

- Software construction benefits from explicit, reusable assets.
- Schemas and contracts beat tribal knowledge.
- The tooling itself is part of the methodology — process without tools
  is aspiration.

DDx does **not** inherit the heavy UML/MDA apparatus that made the
2000s-era factories impractical. The reusable assets in DDx are
plain-text documents — Markdown specs, YAML configs, Markdown personas
— that both humans and language models can read directly.

### What the 2026 spec-driven movement adds

The contemporary spec-driven development (SDD) movement re-grounds the
factory idea for the agentic era. Two reference implementations frame it:

- [REF-005 — GitHub Spec Kit](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-005-github-spec-kit.md):
  treats the PRD/plan/tasks pipeline as executable input that drives
  AI-assisted code generation.
- [REF-006 — AWS Kiro](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-006-aws-kiro.md):
  embeds the same loop inside an IDE — natural language becomes
  structured specs, design notes, and implementation steps before any
  code is produced.

The 2026 movement adds two things the original software-factories work
could not assume:

1. **AI agents as the construction tool.** The factory's "tools" are no
   longer code generators driven by metamodels — they are language
   models driven by prompts composed from the document library.
2. **Documents as the executable artifact.** Specs are not commentary
   alongside code; they are the input the agent runs against. A change
   to the spec re-derives the plan, the contracts, and the tests rather
   than asking a developer to manually propagate it.

DDx's [Spec-First Development](../../principles/spec-first-development/)
principle is the direct expression of this shift.

### What DDx specifically adds

On top of that lineage, DDx contributes a concrete operating model:

- **Bounded-context execution.** Quality decays as a single context
  window fills, well before the hard token limit. DDx runs every bead
  in a fresh, narrowly-scoped context against an explicit contract —
  the Ralph loop. See
  [Bounded Context Execution](../bounded-context-execution/) for the
  mechanics, and the *Lost in the Middle* / *Context Rot* references
  cited there.
- **Evidence on disk.** Every execution leaves an artifact under
  `.ddx/executions/` — prompt, transcript, diff, review verdict. The
  factory's output is not just the merged commit; it is the auditable
  record of how the commit was produced. This is what makes the
  [Audit Trail Required](../../principles/audit-trail-required/)
  principle enforceable rather than aspirational.
- **Project-local install.** `ddx init` and `ddx install <plugin>`
  only touch `<projectRoot>`. The factory's machinery lives next to
  the work it produces, not in a developer's home directory or a
  vendor's cloud. The only global artifact is `ddx-server`.
- **Bead DAG as the work primitive.** Work flows through a dependency
  graph with ready/blocked queues, JSONL interchange, and explicit
  acceptance criteria — the factory's conveyor belt.

## What DDx avoids

A factory metaphor can quickly slide into methodology lock-in. DDx is
deliberate about what it does **not** do:

- **No phase enforcement.** DDx does not own gates, phase transitions,
  or supervisory loops — those are workflow concerns. HELIX provides
  one such opinion; alternative workflows can sit on the same
  primitives. See [Concepts](../) for the platform/workflow split.
- **No proprietary backend.** Specs, beads, and evidence are files in
  Git. The factory floor is inspectable with `ls`, `git log`, and a
  text editor.
- **No required artifact taxonomy.** DDx ships templates and personas,
  but the document types a project uses are the project's choice.
  Spec-Kit, Kiro, HELIX, and bespoke shapes all compose against the
  same primitives.
- **No agent lock-in.** Any harness with a prompt-in / output-out
  contract plugs in. Cheap models do, strong models review — see
  [Right-Size the Model](../../principles/right-size-the-model/).
- **No methodology dressed up as tooling.** The CLI is intentionally
  thin. Opinions live in workflow tools, in personas, and in the
  document library — not in the platform.

## Read next

- [REF-007 — Software Factories (Greenfield & Short, 2004)](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-007-software-factories.md)
- [REF-005 — GitHub Spec Kit](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-005-github-spec-kit.md)
- [REF-006 — AWS Kiro](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/00-discover/references/REF-006-aws-kiro.md)
- [Bounded Context Execution](../bounded-context-execution/)
- [Spec-First Development](../../principles/spec-first-development/)
