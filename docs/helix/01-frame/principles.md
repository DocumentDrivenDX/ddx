---
ddx:
  id: helix.workflow.principles
  depends_on:
    - helix.workflow
---
# DDx Design Principles

## Purpose

These principles guide judgment calls during design, implementation, and review
in the DDx repository. They drive decisions; they are not workflow rules.
Process enforcement belongs in phase enforcers and ratchets, not here.

A principle earns its place by changing actual decisions. If a principle below
isn't cited in a spec or review note, it should be cut.

## Principles

### Platform, Not Methodology

DDx provides primitives. Workflow tools (HELIX) and quality tools (Dun) provide
opinions. The CLI owns: document library, bead tracker, agent dispatch,
personas, templates, git sync. Phases, gates, supervisory loops, and
methodology validation belong in workflow plugins.

Test: if a feature request would put a phase, gate, or methodology-specific
prompt into `cli/`, redirect it to a plugin. If a primitive is duplicated in
two plugins, lift it into the platform.

### Project-Local by Default

`ddx init` and `ddx install <plugin>` only write under `<projectRoot>`. No
home-directory state, no machine-wide opinions. The single global artifact is
`ddx-server`. Per-user config (API keys, model preferences) is the user's
responsibility, not DDx's install surface.

### Beads Are the Unit of Work

Work flows through beads — self-contained items with acceptance criteria and a
dependency DAG. The queue is the interface between intent and execution.
Anything that needs to happen should be expressible as a bead; anything an
agent does should be against a bead.

### Cost-Tiered Throughput

Optimize closed beads per dollar, not raw capability. Cheap models do; strong
models review; deterministic checks sit at the top of the ladder. Routing is
by capability and endpoint, not by hardcoded provider name.

### Design for Simplicity

Start with the minimal structure that could work. Additional components,
layers, or abstractions require documented justification. Complexity that
serves no current requirement is waste — remove it.

### Validate Your Work

Decisions should be testable. If you cannot describe how you would know whether
a choice is working, the choice is not complete. Prefer designs that surface
their own health.

### Make Intent Explicit

Code, configs, and documents should say what they mean. Avoid implicit
conventions, magic values, and behavior that depends on undocumented ordering.
When intent is ambiguous, name it.

### Prefer Reversible Decisions

When two options are otherwise equivalent, choose the one easier to undo.
Commit to irreversible choices deliberately, with documented rationale.
Reversibility buys options; irreversibility spends them.

## Tension Resolution

When principles conflict, apply the one whose violation would cause the worse
outcome in context. Document the tension in the commit message or design note.

Common tensions:

- **Platform vs. Simplicity**: keeping plugins out of the CLI sometimes costs
  duplication across plugins. Accept the duplication until two plugins prove
  the primitive is shared infrastructure.
- **Simplicity vs. Validate Your Work**: the simplest design may not expose
  good observability. Prefer validation unless the cost is disproportionate.
- **Cost-Tiered vs. Reversible Decisions**: routing logic that locks in a
  vendor is cheap to ship and expensive to undo. Prefer endpoint abstractions
  even when a single provider would be simpler today.
