---
title: Principles
weight: 2
---

DDx is shaped by a small set of load-bearing principles. They drive the
trade-offs you see across the CLI, the bead tracker, and the agent service.
Each principle here is one we cite in real design and review decisions — if it
weren't, it wouldn't be on the page.

## Platform, Not Methodology

DDx provides primitives. Workflow tools (HELIX) and other consumers
provide opinions. The CLI owns the document library, bead tracker, agent
dispatch, personas, templates, and git sync. Phases, gates, supervisory loops,
and methodology validation belong in plugins.

**In practice:** when someone proposes adding a "frame phase" command or a
"requirements gate" check to `ddx`, the answer is no — those live in the HELIX
plugin. When two workflow plugins need the same primitive (say, content
hashing), it gets lifted into DDx.

## Project-Local by Default

`ddx init` and `ddx install <plugin>` only write under `<projectRoot>`. There
is no home-directory state and no machine-wide opinions. The single global
artifact is `ddx-server`. Per-user concerns (API keys, model preferences) are
the user's responsibility, not DDx's install surface.

**In practice:** a project's plugin tree lives at `<projectRoot>/.ddx/plugins/`,
its skills at `<projectRoot>/.claude/skills/`, its bead store inside the repo.
Cloning the repo gives you the whole picture; nothing on the developer's
laptop changes the answer.

## Documents Are the Product

Code is output. Documents are what you maintain. Prompts, personas, patterns,
templates, and specs are first-class, versioned, agent-consumable artifacts —
not afterthoughts you write once and forget.

**In practice:** when a spec changes, beads are created for the work the
change implies. When implementation reveals the spec was wrong, the spec is
updated and the implementation follows. The artifact hierarchy is a set of
lenses, not a one-way pipeline.

## Beads Are the Unit of Work

Work flows through beads — self-contained items with acceptance criteria and
a dependency DAG. The queue is the interface between intent and execution.
Anything that needs to happen should be expressible as a bead; anything an
agent does should be against a bead.

**In practice:** a developer noticing a bug doesn't fix it inline — they
`ddx bead create`, link dependencies, and either pick it up or let
`ddx work` drain the queue. The bead is the contract; review checks the
acceptance criteria, not the developer's memory of the conversation.

## Cost-Tiered Throughput

Optimize closed beads per dollar, not raw capability. Cheap models do; strong
models review; deterministic checks sit at the top of the ladder catching
what review missed. Routing is by capability and endpoint, not by
hardcoded provider name.

**In practice:** `ddx try` runs implementation on a cheap model in an
isolated worktree, then routes the diff to a stronger reviewer; `ddx work`
drains the queue across many attempts. Failed reviews escalate to a higher
tier on the next attempt. The agent service discovers models from endpoints
rather than wiring named providers.

## Git-Native, File-First

Plain files. Standard git. No lock-in. Documents live in the repo, beads live
in the repo, plugins live in the repo. Sync is `git pull` and `git push` —
not a proprietary protocol.

**In practice:** beads are JSONL on disk; you can read them with `cat`,
diff them, and merge them. Library documents are Markdown and YAML; you can
edit them in any tool. There is no DDx server you must connect to.

## Agent-Agnostic

Any harness with a prompt-in/output-out contract plugs into the agent
service. Claude, Codex, Gemini, local models — all use the same prompt
envelope, the same session logging, the same quorum review.

**In practice:** `ddx agent run --harness claude` and `ddx agent run --harness
qwen-local` differ in cost and capability, not in interface. Routing decisions
are observable, swappable, and don't require touching the CLI.

## Make Intent Explicit

Code, configs, and documents should say what they mean. Avoid implicit
conventions, magic values, and behavior that depends on undocumented ordering.
When intent is ambiguous, name it.

**In practice:** persona bindings are explicit in `.ddx.yml`, not inferred
from filenames. Bead acceptance criteria are checked items, not prose
expectations. A reviewer can tell what was intended without asking.

## Prefer Reversible Decisions

When two options are otherwise equivalent, choose the one easier to undo.
Commit to irreversible choices deliberately, with documented rationale.

**In practice:** endpoint-based agent routing is preferred over hardcoded
provider names because endpoints are swappable. Project-local install is
preferred over global state because it's reversible by deleting `.ddx/`.

## When Principles Conflict

Apply the one whose violation would cause the worse outcome in the current
context. Document the tension in the commit message or design note. Common
tensions:

- **Platform vs. Simplicity** — keeping plugins out of the CLI sometimes
  costs duplication across plugins. Accept the duplication until two plugins
  prove the primitive is shared infrastructure.
- **Cost-Tiered vs. Reversible Decisions** — routing logic that locks in a
  vendor is cheap to ship and expensive to undo. Prefer endpoint abstractions
  even when a single provider would be simpler today.

For the canonical version of these principles (terse, contributor-facing),
see `docs/helix/01-frame/principles.md` in the repository.
