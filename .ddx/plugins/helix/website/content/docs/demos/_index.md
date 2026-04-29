---
title: Demos
weight: 7
prev: /docs/glossary
---

# Demo Reels

Scripted terminal recordings of HELIX in action. Each demo runs in a Docker container for reproducibility and uses `ddx agent run` as the agent harness.

---

## Quickstart: Full Lifecycle

The complete HELIX onboarding experience: install, frame, design, build, and review. Builds a Node.js temperature converter from scratch, driven entirely by HELIX artifacts and the tracker.

{{< asciinema src="helix-quickstart" >}}

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Install | Install HELIX skills and CLI, verify ddx agent harness |
| 2 | Setup | Initialize git repo, tracker, and AGENTS.md |
| 3 | Frame | Agent creates product vision, PRD, and feature spec |
| 4 | Design | Agent creates technical design, then tracker issues |
| 5 | Build | Red: write failing tests. Green: implement to pass. |
| 6 | Verify | Run tests, check acceptance criteria |
| 7 | Review | Agent reviews all work for gaps |

{{< tabs >}}

{{< tab name="Docker" >}}
```bash
docker build -t helix-demo docs/demos/helix-quickstart/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-quickstart/recordings:/recordings \
  helix-demo
```
{{< /tab >}}

{{< tab name="Local" >}}
```bash
cd /tmp
bash /path/to/helix/docs/demos/helix-quickstart/demo.sh
```

Requires: git, jq, node, npm, ddx, helix CLI.
{{< /tab >}}

{{< /tabs >}}

---

## Concerns: Preventing Technology Drift

Demonstrates how [concerns](/docs/glossary/concerns) keep agents on the declared technology stack. A Bun/TypeScript project declares `typescript-bun` as its concern. The agent builds with Bun-native tools, then a deliberate drift (vitest import) is introduced and caught by concern-aware review.

{{< asciinema src="helix-concerns" >}}

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Create Bun project with `typescript-bun` concern |
| 2 | Frame | Agent reads concerns, uses Bun-native requirements |
| 3 | Build | Agent implements with Bun.serve(), bun:test, Biome |
| 4 | Drift | Deliberate Node.js drift introduced (vitest import) |
| 5 | Review | Agent detects drift and files a tracker issue |

{{< tabs >}}

{{< tab name="Docker" >}}
```bash
docker build -t helix-concerns-demo docs/demos/helix-concerns/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-concerns/recordings:/recordings \
  helix-concerns-demo
```
{{< /tab >}}

{{< tab name="Local" >}}
```bash
cd /tmp
bash /path/to/helix/docs/demos/helix-concerns/demo.sh
```

Requires: git, jq, bun, ddx, helix CLI.
{{< /tab >}}

{{< /tabs >}}

---

## Evolve: Threading Requirements Through the Stack

Demonstrates how `helix evolve` propagates a new requirement through every layer of the artifact stack. Starting from a working temperature converter, a "Add Kelvin support" requirement is threaded through the PRD, feature spec, and technical design, then implemented via TDD.

{{< asciinema src="helix-evolve" >}}

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Working v1 project with existing artifacts and code |
| 2 | Evolve | Thread "Add Kelvin" through PRD, feature spec, design, tracker |
| 3 | Build | TDD: failing Kelvin tests (Red), then implementation (Green) |
| 4 | Verify | All tests pass, new CLI flags work |

{{< tabs >}}

{{< tab name="Docker" >}}
```bash
docker build -t helix-evolve-demo docs/demos/helix-evolve/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-evolve/recordings:/recordings \
  helix-evolve-demo
```
{{< /tab >}}

{{< tab name="Local" >}}
```bash
cd /tmp
bash /path/to/helix/docs/demos/helix-evolve/demo.sh
```

Requires: git, jq, node, npm, ddx, helix CLI.
{{< /tab >}}

{{< /tabs >}}

---

## Experiment: Metric-Driven Optimization

Demonstrates how `helix experiment` optimizes code through measured iteration. A deliberately slow string processing function is improved through hypothesis-driven changes, with correctness tests enforced at every step.

{{< asciinema src="helix-experiment" >}}

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Project with slow function, correctness tests, and benchmark |
| 2 | Iterate | Agent hypothesizes, edits, tests, benchmarks (iteration 1) |
| 3 | Iterate | Agent finds next bottleneck, optimizes further (iteration 2) |
| 4 | Results | Final benchmark shows improvement, all tests still passing |

{{< tabs >}}

{{< tab name="Docker" >}}
```bash
docker build -t helix-experiment-demo docs/demos/helix-experiment/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-experiment/recordings:/recordings \
  helix-experiment-demo
```
{{< /tab >}}

{{< tab name="Local" >}}
```bash
cd /tmp
bash /path/to/helix/docs/demos/helix-experiment/demo.sh
```

Requires: git, jq, node, npm, ddx, helix CLI.
{{< /tab >}}

{{< /tabs >}}
