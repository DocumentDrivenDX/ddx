---
title: Context Is King
weight: 4
---

# Context is king

> The model is not the bottleneck; context selection and ordering are.

## The principle

An agent's output is bounded by the context it receives. The same model,
given a sharper brief, produces dramatically better work — and the same
model, given a bloated or stale brief, produces confidently wrong work.
A smaller model with the right 4K of context will routinely beat a
larger model given 200K of irrelevant tokens.

In a document-driven factory, context engineering is not a tuning
detail; it is the primary lever on quality and cost. That means
investing in the mechanisms that select, shape, and bound what each
agent sees: the spec excerpts, the relevant code, the prior attempts,
the review findings, the persona. It also means resisting the
temptation to dump everything into the prompt "just in case," because
irrelevant context degrades performance and inflates cost.

For multi-agent systems the stakes compound. Agents call themselves and
each other, and each call inherits or assembles its own context. A
loop that ladles full transcripts forward at each step is manufacturing
degradation at every hop. The factory's job is to assemble the right
brief for the right step, every time, and to make that assembly visible
and tunable.

## Evidence

- **REF-008 — Liu et al., "Lost in the Middle."** Across multiple
  frontier models and task types, recall follows a U-shaped curve —
  information at the start and end of the prompt is reliably retrieved;
  information in the middle is dropped or hallucinated. Position of
  evidence in the context window matters as much as whether the evidence
  is there at all.
- **REF-009 — Chroma, "Context Rot" (2025).** Even when relevant
  information is well-placed, performance degrades as raw input length
  grows, well within advertised context windows. Larger windows are
  real capacity but not free capacity.
- **RSCH-004.** Composes the two: more context is not better context.
  Retrieve aggressively, prune ruthlessly, place load-bearing material
  at the start or end of the window.

See `docs/helix/00-discover/research/RSCH-004-context-is-king.md`.

## DDx response

- **The document library** is structured retrieval surface: typed
  artifacts (specs, personas, templates, patterns), addressable IDs,
  and explicit `depends_on` edges so a bead can be assembled with
  exactly the context it needs and no more.
- **Beads carry only their description and AC** into the execute-bead
  prompt; governing references are pulled by ID, not inlined wholesale.
- **Personas** keep system prompts short and role-specific rather than
  catch-all, so the per-call context budget is spent on the task, not
  on instructions about being an assistant.
- **Harness configs and review-finding threading** shape the brief at
  retry time: only the unmet AC items and their review notes carry
  forward, not the full prior transcript.
- **`.ddx/executions/`** captures what was actually sent, so the brief
  itself is observable and tunable rather than an opaque side-effect of
  the loop.
