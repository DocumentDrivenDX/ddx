# Doc-graph layout baseline (Story 3.B input)

Bead: `ddx-0e6ff1c5` — *doc-graph: profile current layout — label widths, settle time, degree distribution*.

This document captures measurement-anchored facts about the current
`D3Graph.svelte` d3-force layout so the Story 3.B redesign can express its
acceptance criteria as deltas against numbers (not vibes).

## What was profiled

- Component: `cli/internal/server/frontend/src/lib/components/D3Graph.svelte`
- Simulation parameters lifted verbatim into the profiler:
  - `forceLink` distance `160`, strength `0.4`
  - `forceManyBody` strength `-600`
  - `forceCenter(width/2, height/2)` at viewport `1280×800`
  - `forceCollide(48)`
  - default `alpha = 1`, `alphaMin = 0.001`, `alphaDecay ≈ 0.0228`,
    `velocityDecay = 0.4`
- Label rule: `text-body-sm` (13px, default sans-serif stack), text anchor at
  `x = 24` next to a `r = 18` circle, title truncated at 32 chars + `…`.

## Reproducer

```bash
cd cli/internal/server/frontend
bun install                                                    # one-time
bun ../../../../.ddx/executions/20260502T123549-d9f9c327/profile-layout.mjs
```

The script (`profile-layout.mjs` in the same directory as this report)
constructs a 128-node fixture that mirrors a realistic DDx doc graph: a
ternary tree backbone (parent index `floor((i-1)/3)`) modelling Vision → PRD →
Feature Spec → User Story, plus three families of cross-links that produce a
small number of high in-degree hubs (every 7th, 13th, 25th node back-edges to
an older node). The result is `128 nodes, 159 links` — close to the density
the real `docGraph` query returns on this repo today.

It then runs the d3-force simulation synchronously to settling, three times,
and prints a JSON report.

## Results (128-node fixture)

### Degree distribution

| Direction | min | median | mean | max | p95 |
|-----------|----:|-------:|-----:|----:|----:|
| **in-degree**  | 1 | 1 | 1.24 | **10** | 2 |
| **out-degree** | 0 | 1 | 1.24 | **4**  | 4 |

Top in-degree hubs (id, in-degree): `n0:10, n1:6, n7:2, n14:2, n21:2`.
Top out-degree hubs: `n7:4, n13:4, n14:4, n21:4, n25:4`.

The graph is sparse and tree-dominated. The two roots (`n0`, `n1`) are
visually meaningful hubs; everything else has degree ≤ 2 in either direction.
This matches the spec-tree shape DDx tracks.

### Label widths (text rendered to the right of each node)

Approximated from `text-body-sm` (13px sans-serif, `~6.8 px/char` average
advance for mixed-case ASCII; this is an under-estimate for ALL-CAPS strings).

| metric | value |
|--------|------:|
| min width  |  95 px |
| median     | 224 px |
| mean       | 219.59 px |
| p95        | 224 px |
| **max**    | **224 px** |

The `slice(0, 32) + '…'` rule clamps any title at 32 visible characters,
which is the cause of the 224 px ceiling (`32 × 6.8 ≈ 218` plus the ellipsis
glyph). Most node titles in the realistic fixture saturate this cap — which
means in current production, virtually every label hits the cap on common
DDx titles like *"Feature Spec: doc-graph viewer"* and *"User Story: graph
fits 128 nodes without clipping"*. The 32-char truncation is the dominant
information-loss event in the layout, not the layout itself.

A label box therefore occupies roughly `224 + 24 = 248 px` of horizontal
real estate per node, anchored at the node centre. With `forceCollide(48)`
collision happens at the *circle*, not the *label* — so labels overlap each
other freely whenever nodes are within ~250 px horizontally and ~13 px
vertically.

### Settle time

Tick count and wall-clock for three back-to-back synchronous runs in Bun:

| run | ticks | wall-clock (compute only) |
|----:|------:|--------------------------:|
| 1   | 300 | 121.71 ms |
| 2   | 300 |  92.84 ms |
| 3   | 300 |  88.39 ms |

Tick count is deterministic at 300, governed by the default `alphaDecay`
(`alphaMin^(1/300)`). In the browser, the simulation drives ticks via
`d3.timer` / `requestAnimationFrame`, so wall-clock settle time on a 60 Hz
frame budget is **≈ 300 × 16.67 ms ≈ 5.0 s**, not the ~100 ms compute time
above. The compute number is the floor: any browser slower than 60 fps will
take longer.

The "fit-to-bounds" pan/zoom transition at `simulation.on('end')` adds
another **400 ms** (`d3.transition().duration(400)`), so the user-perceived
"layout finished" event is **~5.4 s** after mount on a typical desktop.

## Implications for Story 3.B acceptance criteria

These numbers turn vague goals into testable thresholds. Suggested wording:

1. **Label budget.** A node label must fit inside a `≤ 200 px` horizontal box
   without truncation for the median DDx title (current p95 = 224 px, all
   capped). Story 3.B should either widen the truncation cap, switch to a
   wrapping label, or pick a tighter font — and assert the rendered bounding
   box stays inside the chosen budget.
2. **Label collision.** No two labels may overlap when the layout settles on
   the 128-node fixture. Today, `forceCollide(48)` only protects circles,
   so this is currently violated whenever two non-leaf nodes settle within
   ~250 px horizontally — a near-certain occurrence on the tree backbone.
3. **Settle time.** Time-to-stable layout on the 128-node fixture must be
   **≤ 2.0 s** (60% reduction from the ~5.0 s baseline). Achievable by
   raising `alphaDecay` to ~0.05, dropping ticks to ~120, or by switching to
   a deterministic layout (dagre/elk) that does not iterate.
4. **Hub legibility.** The two highest in-degree nodes (degree ≥ 6) must
   remain non-overlapping with their neighbours. Today the `-600` charge
   spreads them adequately *only* because the graph is sparse; any new
   layout must explicitly cover the hub case.
5. **Determinism.** Re-mounting the graph with the same data must produce
   the same final positions (today it is non-deterministic — `d3-force`
   randomly initialises positions on every rebuild). Story 3.B should seed
   or replace the layout to make screenshot diffs stable.

## Limitations

- Label widths use a per-character approximation, not browser
  `getBBox`. Cross-checked against the truncation cap (`32 × 6.8 ≈ 218`)
  and consistent within ±5 % of native rendering for the default
  font stack. Re-measure with `canvas` or a Playwright probe if Story 3.B
  needs sub-pixel accuracy.
- Wall-clock settle time is compute-only (no `requestAnimationFrame`
  pacing). The 60 Hz extrapolation (`300 × 16.67 ms`) is a lower bound for
  real-device behaviour.
- The fixture is synthetic but shape-matched to the real `docGraph` query;
  swap in a snapshot of `library/`-derived nodes if a perfectly faithful
  baseline is required.
