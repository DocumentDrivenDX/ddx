# HELIX Experiment Demo

Demonstrates metric-driven optimization with `helix experiment`.

A project has a deliberately slow string processing function with correctness tests. The agent runs experiment iterations: hypothesize a change, implement it, verify tests still pass, benchmark, then keep or discard based on the metric.

## Prerequisites

- Docker
- Claude Code credentials (`~/.claude/`, `~/.claude.json`)
- DDx binary (mounted or in PATH)

## Run

```bash
# From the helix repo root:
docker build -t helix-experiment-demo docs/demos/helix-experiment/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-experiment/recordings:/recordings \
  helix-experiment-demo
```

## What It Does

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Create a project with slow string processor and correctness tests |
| 2 | Iterate | Agent hypothesizes, edits, tests, benchmarks (iteration 1) |
| 3 | Iterate | Agent finds next bottleneck, optimizes further (iteration 2) |
| 4 | Results | Final benchmark, all tests still passing |

## Key Points

- Tests MUST pass — if they don't, the change is discarded regardless of metric improvement
- One change per iteration — isolate what works from what doesn't
- Keep/discard decisions are based on measured metric, not intuition
- The benchmark outputs `METRIC runtime=<ms>` for structured tracking

## Recordings

```bash
asciinema play recordings/helix-experiment-*.cast
```
