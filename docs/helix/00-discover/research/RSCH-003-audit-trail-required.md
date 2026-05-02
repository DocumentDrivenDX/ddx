---
ddx:
  id: RSCH-003
  status: draft
  depends_on:
    - REF-014
    - REF-020
    - REF-021
id: RSCH-003
title: "Audit Trail Required"
kind: research-synthesis
summary: "When AI agents modify a system, the immutable record of what they did — inputs, outputs, decisions, and timing — is not optional instrumentation; it is the system's contract surface."
tags: [audit, event-sourcing, observability, ddx-principle]
---

# Audit Trail Required

## Principle

Every agent action — prompt, tool call, file edit, model used, cost incurred —
must be captured in an append-only, replayable log. The log, not the current
state of the working tree, is the truth.

## Synthesis

Pat Helland's "Data on the Outside vs. Data on the Inside" (REF-014) draws
the foundational distinction: data flowing between independent actors must be
immutable, identified, and versioned, because the receiver has no way to
reach back and re-read the sender's mutable internal state. Agent
interactions are exactly this case — the model that produced a diff is gone
the moment it returns; the only thing that persists is the artifact it
emitted. If that artifact is not captured with its provenance, the system
has no contract surface to reason about.

Kleppmann's *Designing Data-Intensive Applications* (REF-020) generalizes
the pattern across distributed systems: derived state in caches, replicas,
indexes, and search systems is reconstructable only if the underlying log
of changes is preserved. Stream-processing architectures, CDC pipelines,
and consensus systems all converge on the same pattern — an ordered,
append-only log is the primary artifact, and current state is a projection.

Fowler's Event Sourcing essay (REF-021) makes the architectural pattern
concrete: rather than mutating in place, record each state change as an
immutable event; the present is rebuildable from the log at any time. The
benefits Fowler enumerates — temporal queries, debugging, rebuilding
projections after a bug fix, regulatory audit — are exactly the properties
needed when an autonomous agent has been modifying production code on its
own initiative.

The composition is forceful: REF-014 says cross-actor contracts must be
immutable, REF-020 says distributed correctness depends on the log, REF-021
says the log can be the *primary* artifact rather than a debugging
afterthought. Apply this to agentic systems and the conclusion is that the
agent's action stream must be event-sourced. Anything less leaves teams
unable to answer "what did the agent do, why, and what did it cost" — which
is the bare minimum for trusting agent-modified code in a shared codebase.

## DDx Implication

DDx records every execution under `.ddx/executions/<timestamp>-<hash>/` —
the prompt the agent saw, the diff it produced, the harness and model it
used, cost, and exit reason. The bead tracker itself behaves as an
event-sourced store: bead state changes (created, blocked, ready, closed)
are durable, addressable, and replayable. The execute-bead worktree is
named, preserved on failure, and merged with `--no-ff` so the history
contains a tamper-evident record of which agent did which work. None of
this is optional polish; it is the contract surface that lets humans
trust, review, and roll back agent output.
