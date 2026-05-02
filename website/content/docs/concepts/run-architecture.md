---
title: Run Architecture
weight: 3
---

DDx owns three explicit, layered run primitives. Each higher layer composes
the layer beneath it. There are no other run kinds beyond these three. See
**FEAT-010** in `docs/helix/01-frame/features/FEAT-010-executions.md` for the
full specification.

| Layer | CLI | What it owns |
|---|---|---|
| 1 | `ddx run` | one agent invocation — prompt in, structured output and side effects out |
| 2 | `ddx try <bead>` | one bead attempt in an isolated worktree — bead → prompt resolution, merge or preserve |
| 3 | `ddx work` | one drain of the bead queue — iterate `ddx try` until a stop condition is met |

`ddx try` wraps `ddx run`. `ddx work` iterates `ddx try`. One on-disk
substrate; layer metadata distinguishes records.

```
┌──────────────────────────────────────────────────────────┐
│  Layer 3 — ddx work    (queue drain)                     │
│  ┌────────────────────────────────────────────────────┐  │
│  │  Layer 2 — ddx try <bead>   (bead attempt)         │  │
│  │  ┌──────────────────────────────────────────────┐  │  │
│  │  │  Layer 1 — ddx run     (invocation atom)     │  │  │
│  │  └──────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

## Layer 1 — `ddx run`

A layer-1 run is one agent invocation. Inputs are a prompt, requested
`MinPower` (and optional `MaxPower`), optional agent passthrough constraints,
and non-routing execution config. Outputs are the structured response (text or
bytes), any side-effects the agent performed via tools, and run metadata
(tokens, model, actual power, duration, exit status, session pointer).

Layer 1 calls the upstream `ddx-agent` service contract directly. DDx does not
reimplement agent routing or the invocation loop; it wraps one `Execute` call
with provenance capture. Routing — model choice within power bounds, provider
fallback, retry — lives in `ddx-agent`. DDx contributes a layer-1 record per
invocation.

`ddx artifact regenerate <id>` is sugar for layer 1 (or layer 2, when the
generator edits the repo directly) with `produces_artifact: <id>` metadata. It
is not a fourth layer.

## Layer 2 — `ddx try <bead>`

A layer-2 run is one bead attempt in an isolated worktree. It owns:

- Worktree creation from a base revision and worktree finalization (merge or
  preserve).
- Bead → prompt resolution: description, acceptance criteria, governing
  artifacts, persona, project config.
- Side-effect bundling: commits, evidence, no-changes rationale.
- One or more layer-1 invocations, recorded as children of the attempt.

Layer 2 owns bead-attempt success classification. DDx determines success from
artifacts it owns: commit presence, merge or preserve result, no-changes
rationale, post-run checks, review verdicts, and cooldown policy. The agent's
exit status and actual model/power are inputs to that decision, not the whole
decision.

A layer-2 record references its child layer-1 records by run id.

## Layer 3 — `ddx work`

A layer-3 run is one drain of the bead queue. It iterates `ddx try` across
ready beads until a stop condition is met. It owns:

- Queue iteration order.
- No-progress and stop-condition evaluation.
- A loop-level record that references its child layer-2 records by attempt id
  and reports terminal disposition (drained, blocked, deferred, no-progress).

Stop conditions evaluated between attempts include queue exhaustion, budget
exhaustion, an explicit deferral, and consecutive no-progress attempts. See
FEAT-010 for the full list.

Content-aware supervisory decisions — for example, "comparison failed →
enqueue reconciliation beads" — are not layer-3 concerns. Those are skill or
plugin compositions on top of the three layers.

## Why Three Layers, Not Two or Four

The three layers correspond to three real composition boundaries:

- **Atom:** one prompt in, one response out. This is the unit a model
  provider charges for and a session log records.
- **Attempt:** one bead, one worktree, one merge-or-preserve decision. This
  is the unit reviews grade and beads close against.
- **Drain:** one queue pass with a budget and a stop condition. This is the
  unit operators schedule and observers watch.

A two-layer model conflates atom and attempt or attempt and drain. A
four-layer model invents a band that does not correspond to a real seam
(typically by promoting a particular skill, like comparison or replay, to a
top-level run kind). DDx will not introduce additional run kinds beyond the
three layers — new flavors emit ordinary layer-1, layer-2, or layer-3 records
with skill-specific metadata.

## Cost Tiering Across the Layers

The three layers are cost-tiered by design:

- **Layer 1** asks the cheapest agent that meets the requested `MinPower` to
  do the work.
- **Layer 2** routes review of the resulting diff to a stronger model when
  the bead's acceptance criteria warrant it. Failed reviews thread their
  findings into the next attempt's prompt and may raise `MinPower` on retry.
- **Layer 3** sits above both, watching the loop drain and stopping when the
  queue, the budget, or progress runs out.

Deterministic checks (Dun, project test suites, lints) sit above review,
catching what slipped through. The cheap model implements; the strong model
reviews; deterministic checks have the final word.

## On-Disk Shape

All three layers persist into a single substrate under `.ddx/runs/` (or the
legacy `.ddx/executions/` location, for layer-2 attempts in the current
codebase). Each record carries a layer tag (1, 2, or 3) and references its
parents and children by run id. `ddx runs list --layer N` queries across all
three layers; `ddx runs show <id>` returns the record plus its layer-specific
extension. See FEAT-010 for the canonical schema.

## See Also

- [Architecture](../architecture/) — bead lifecycle, persona binding, and the
  project-local install model.
- [Bounded-context execution](../bounded-context-execution/) — why DDx wraps
  agents in this loop instead of trusting longer-lived sessions.
- [Principles](../principles/) — the load-bearing decisions behind the
  three-layer model.
- [FEAT-010 — Three-Layer Run Architecture](https://github.com/DocumentDrivenDX/ddx/blob/main/docs/helix/01-frame/features/FEAT-010-executions.md)
  — the full feature specification.
