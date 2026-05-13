# Product Vision

## Mission Statement

HELIX is a methodology and artifact catalog for AI-assisted software teams. It
packages decades of document discipline — PRDs, design documents, test plans,
alignment reviews — as portable templates plus one skill that keeps everything
in sync as the work moves. Like the double helix of DNA, two strands run
together: the document strand holds intent, design, and tests; an execution
runtime turns those into code.

## Positioning

For software teams using AI agents to build production systems but losing time
to inconsistent specifications, drifting designs, and ad-hoc prompting, HELIX
is a portable development methodology delivered as artifact templates and a
single planning skill. Unlike one-off prompt libraries or vendor-locked
workflow tools, HELIX is a methodology you adopt, not a platform you join —
its content runs on any agent runtime that reads and writes markdown.

## Vision

HELIX becomes the default discipline for AI-assisted software development.
Teams adopt it the way they adopted test-driven development: not because of a
tool, but because the practice produces durably better software. Agents
authoring against HELIX's templates produce specifications and designs at a
level human reviewers actually trust. The alignment skill keeps the governing
artifacts coherent as priorities and code evolve.

**North Star**: A team writes its product intent once, and the rest of the
governing artifacts — features, designs, tests, code — stay aligned with that
intent through every change, without the team rewriting documents by hand.

## User Experience

A team wants to add OAuth login. They describe the intent to their agent. The
agent invokes the HELIX alignment skill against the project's existing
governing artifacts.

The skill walks the artifacts and produces a structured finding: the new
feature affects three feature specs, two solution designs, and the security
architecture; one existing ADR conflicts with the OAuth pattern; four user
stories need to be added; one test plan must be revised before any of that.
Output is a plan ordered by authority — start with the security-architecture
revision, then update the affected feature specs, then create the stories,
then update the designs.

The team reviews the plan, adjusts scope, approves it. The runtime (DDx,
Databricks Genie, whatever they're using) creates work items from the plan.
As work happens, the alignment skill runs periodically against the artifacts
to catch drift before it accumulates.

The team never invoked a HELIX command, because there isn't one. They invoked
their agent, and the agent invoked HELIX's skill.

## Target Market

| Attribute | Description |
|-----------|-------------|
| Who | Software teams using AI agents (Claude Code, Cursor, Databricks Genie, Copilot Workspace) for non-trivial development |
| Pain | Artifact drift across sessions; agents make local decisions that conflict with global intent; specs and code diverge silently; reviews catch problems too late |
| Current Solution | Ad hoc prompting, manual document maintenance, hoping for the best |
| Why They Switch | HELIX gives agents and humans the same methodology — every change traces back to a governing artifact, and the alignment skill catches drift early |

## Key Value Propositions

| Capability | Customer Benefit |
|---|---|
| Authority-ordered artifact catalog | Every document has a clear governing relationship; changes propagate predictably |
| Single alignment skill | Catches drift early without walking the artifact tree by hand |
| Portable content | Run on any runtime that reads markdown — DDx, Databricks Genie, Claude Code, anything |
| Document-driven reviews | Audits work on artifacts, not chat transcripts |
| Methodology, not platform | Adopt incrementally; no vendor lock-in |

## Success Definition

| Metric | Target |
|--------|--------|
| Runtime breadth | 3 documented runtime deployments (DDx, Databricks Genie, one other) within 12 months |
| Adoption | 50+ public repos using HELIX templates as their artifact catalog within 18 months |
| Alignment quality | Healthy artifact sets average < 3 alignment findings per skill run |
| Authoring quality | PRDs and feature specs authored from HELIX templates pass first-pass review at measurably higher rates than free-form equivalents |
| Skill portability | Skill body contains zero runtime-specific commands; portability check passes on every release |

## Why Now

AI-assisted development has crossed from novelty to default practice. Most
professional developers ship AI-touched code daily. But the practice is
uneven: agents produce work that is locally good but globally inconsistent,
because the disciplines human teams developed over decades — PRDs, design
documents, test plans, alignment reviews — have not been packaged into
something agents can apply uniformly.

The agents are ready. The methodology has been ad hoc. The Genie / Cursor /
Claude Code era will have its TDD-equivalent practice. HELIX is the bet on
what it looks like: document-driven, agent-friendly, portable.

