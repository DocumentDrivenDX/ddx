---
title: Audit Trail Required
weight: 3
---

# Audit trail required

> The log, not the current state of the working tree, is the truth.

## The principle

Every artifact the factory produces must be traceable to the inputs that
caused it: which spec, which bead, which agent, which model, which
prompt, which commit. Without that chain, debugging becomes guesswork
and compliance becomes impossible. An audit trail is also the substrate
for learning — you cannot improve a process whose decisions are
unrecorded.

In an agent-heavy pipeline this is doubly true. The agents themselves
are non-deterministic and their behavior drifts as models change. The
model that produced a diff is gone the moment it returns; the only thing
that persists is the artifact it emitted. If that artifact is not
captured with its provenance — prompt, harness, model, cost, exit
reason — the system has no contract surface to reason about. The trail
must capture not only what was decided but what was considered and
rejected, so that a future maintainer (human or agent) can reconstruct
the reasoning rather than re-derive it.

DDx treats the audit trail as load-bearing infrastructure, not as
logging. It is part of the product, not an operational courtesy. Every
agent action — prompt, tool call, file edit, model used, cost incurred
— is captured in an append-only, replayable form, and the bead state
machine itself behaves as an event-sourced store.

## Evidence

- **REF-014 — Pat Helland, "Data on the Outside vs. Data on the Inside."**
  Data flowing between independent actors must be immutable, identified,
  and versioned. Agent interactions are exactly this case — the model
  that produced a diff is gone the moment it returns.
- **REF-020 — Kleppmann, *Designing Data-Intensive Applications*.**
  Derived state in caches, replicas, indexes, and search systems is
  reconstructable only if the underlying log of changes is preserved.
- **REF-021 — Fowler, "Event Sourcing."** Record each state change as
  an immutable event; the present is rebuildable from the log at any
  time. Enables temporal queries, debugging, projection rebuilds, and
  regulatory audit.
- **RSCH-003.** REF-014 says cross-actor contracts must be immutable,
  REF-020 says distributed correctness depends on the log, REF-021 says
  the log can be the *primary* artifact rather than a debugging
  afterthought.

See `docs/helix/00-discover/research/RSCH-003-audit-trail-required.md`.

## DDx response

- **`.ddx/executions/<timestamp>-<hash>/`** preserves per-attempt
  evidence: the prompt the agent saw, the diff it produced, the harness
  and model it used, cost, and exit reason.
- **Bead history** records every state transition (created, blocked,
  ready, closed) durably and replayably; the bead tracker is itself an
  event-sourced store.
- **Commits reference the bead id** in the subject (`[ddx-XXXX]`), and
  execute-bead worktrees are merged with `--no-ff` so the history
  contains a tamper-evident record of which agent did which work.
- **Agent-run logs** capture model, harness, prompt, and cost so routing
  decisions can be evaluated empirically rather than asserted.
