#!/usr/bin/env bun
// Profile current d3-force layout used by D3Graph.svelte.
//
// Mirrors the simulation parameters in
// cli/internal/server/frontend/src/lib/components/D3Graph.svelte:
//   - forceLink distance 160, strength 0.4
//   - forceManyBody strength -600
//   - forceCenter(width/2, height/2)
//   - forceCollide 48
//   - default alpha 1, alphaMin 0.001, alphaDecay ~0.0228, velocityDecay 0.4
//
// Measures, for a 128-node fixture:
//   1. Label widths (approx, for the truncated 32-char title) using a
//      monospace-friendly average advance for the 13px text-body-sm class.
//   2. Settle time: synchronous tick loop until alpha < alphaMin.
//   3. Degree distribution: max in-degree, max out-degree, mean / median.
//
// Run from cli/internal/server/frontend with:
//   bun ../../../../.ddx/executions/<run-id>/profile-layout.mjs
// or:
//   cd cli/internal/server/frontend && bun /abs/path/profile-layout.mjs

import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  forceCollide
} from 'd3-force'

const NODE_COUNT = 128
const VIEW_W = 1280
const VIEW_H = 800

// 1. Build a fixture that mimics a real DDx doc-graph: a frame doc tree (vision
// → PRD → features → user stories) plus a moderate web of cross-links. This
// gives us a realistic mix of high-fan-out hubs and leaf nodes.
function buildFixture(n) {
  const nodes = []
  const links = []
  const titles = [
    'Product Vision',
    'Product Requirements Document',
    'Feature Spec: doc-graph viewer',
    'Feature Spec: bead tracker UX',
    'Feature Spec: agent execute-loop dashboard',
    'Feature Spec: persona binding UI',
    'User Story: graph node click opens document',
    'User Story: graph supports zoom and pan',
    'User Story: graph fits 128 nodes without clipping',
    'User Story: stale documents render distinctly',
    'ADR-0007 Document graph layout strategy',
    'ADR-0011 Bead identifier format',
    'Runbook: drain the queue safely',
    'Runbook: recover a corrupted bead store',
    'Runbook: rotate harness credentials',
    'Test plan: graph contrast and accessibility'
  ]
  for (let i = 0; i < n; i++) {
    const base = titles[i % titles.length]
    nodes.push({
      id: `n${i}`,
      title: i < titles.length ? base : `${base} (variant ${Math.floor(i / titles.length)})`,
      // mimic the truncation rule in D3Graph.svelte
      label: ''
    })
  }

  // Tree backbone: parent index = floor((i-1)/3) — average out-degree 3 near
  // the top, with a long tail of leaves.
  for (let i = 1; i < n; i++) {
    const parent = Math.floor((i - 1) / 3)
    links.push({ source: `n${parent}`, target: `n${i}` })
  }

  // Cross-links: every 7th node depends on its index-7 neighbor; every 13th
  // pulls a back-edge to a hub. Produces a couple of high in-degree hubs.
  for (let i = 7; i < n; i += 7) {
    links.push({ source: `n${i}`, target: `n${i - 7}` })
  }
  for (let i = 13; i < n; i += 13) {
    links.push({ source: `n${i}`, target: `n0` })
  }
  for (let i = 25; i < n; i += 25) {
    links.push({ source: `n${i}`, target: `n1` })
  }

  return { nodes, links }
}

// 2. Approximate label widths for text-body-sm (13px, default sans-serif stack
// in the SvelteKit app). Measured against system sans on macOS / Linux: the
// average advance for a mixed-case ASCII label is ~6.6–7.0 px per character at
// 13px. We use 6.8 as the central estimate. Numeric/uppercase content widens
// slightly; this is a conservative *under*-estimate for ALL-CAPS strings.
const PX_PER_CHAR_BODY_SM = 6.8
const TITLE_MAX_CHARS = 32 // matches the slice(0,32)+'…' rule in D3Graph.svelte

function approxLabelWidthPx(title) {
  const truncated = title.length > TITLE_MAX_CHARS ? title.slice(0, TITLE_MAX_CHARS) + '…' : title
  return Math.round(truncated.length * PX_PER_CHAR_BODY_SM)
}

function summarize(arr) {
  const sorted = [...arr].sort((a, b) => a - b)
  const sum = sorted.reduce((s, v) => s + v, 0)
  return {
    min: sorted[0],
    max: sorted[sorted.length - 1],
    mean: +(sum / sorted.length).toFixed(2),
    median: sorted[Math.floor(sorted.length / 2)],
    p95: sorted[Math.floor(sorted.length * 0.95)]
  }
}

function profileOnce(fixture) {
  const sim = forceSimulation(fixture.nodes.map((n) => ({ ...n })))
    .force(
      'link',
      forceLink(fixture.links.map((l) => ({ ...l })))
        .id((d) => d.id)
        .distance(160)
        .strength(0.4)
    )
    .force('charge', forceManyBody().strength(-600))
    .force('center', forceCenter(VIEW_W / 2, VIEW_H / 2))
    .force('collide', forceCollide(48))
    .stop() // run synchronously below

  const t0 = performance.now()
  let ticks = 0
  // Mirror d3-force's internal driver: tick until alpha < alphaMin.
  while (sim.alpha() >= sim.alphaMin()) {
    sim.tick()
    ticks++
    if (ticks > 5000) break // safety
  }
  const elapsedMs = performance.now() - t0
  return { ticks, elapsedMs }
}

function main() {
  const fixture = buildFixture(NODE_COUNT)

  const labelWidths = fixture.nodes.map((n) => approxLabelWidthPx(n.title))
  const inDeg = new Map()
  const outDeg = new Map()
  for (const n of fixture.nodes) {
    inDeg.set(n.id, 0)
    outDeg.set(n.id, 0)
  }
  for (const l of fixture.links) {
    outDeg.set(l.source, (outDeg.get(l.source) || 0) + 1)
    inDeg.set(l.target, (inDeg.get(l.target) || 0) + 1)
  }
  const inVals = [...inDeg.values()]
  const outVals = [...outDeg.values()]

  // Three runs to reveal CPU jitter; settle time is deterministic in tick
  // count (alphaDecay is fixed) but wall-clock varies.
  const runs = [profileOnce(fixture), profileOnce(fixture), profileOnce(fixture)]
  const tickCounts = runs.map((r) => r.ticks)
  const wallTimes = runs.map((r) => +r.elapsedMs.toFixed(2))

  const inHubs = [...inDeg.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5)
  const outHubs = [...outDeg.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5)

  const result = {
    fixture: {
      nodes: NODE_COUNT,
      links: fixture.links.length,
      viewport: { width: VIEW_W, height: VIEW_H }
    },
    labels: {
      truncationRule: `slice(0, ${TITLE_MAX_CHARS}) + '…'`,
      pxPerCharEstimate: PX_PER_CHAR_BODY_SM,
      fontClass: 'text-body-sm (13px, default sans-serif stack)',
      widths: summarize(labelWidths),
      anchorOffsetPx: 24, // text x=24 in D3Graph.svelte
      circleRadiusPx: 18
    },
    degree: {
      in: { ...summarize(inVals), top: inHubs },
      out: { ...summarize(outVals), top: outHubs }
    },
    settle: {
      tickCount: tickCounts,
      wallClockMs: wallTimes,
      // d3-force defaults: alpha 1, alphaMin 0.001, alphaDecay = 1 -
      // alphaMin^(1/300) ≈ 0.0228, so ~300 ticks to settle.
      defaults: {
        alpha: 1,
        alphaMin: 0.001,
        alphaDecayApprox: 0.0228,
        velocityDecay: 0.4
      }
    }
  }

  console.log(JSON.stringify(result, null, 2))
}

main()
