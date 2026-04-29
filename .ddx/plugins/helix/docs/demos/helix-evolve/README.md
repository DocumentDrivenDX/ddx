# HELIX Evolve Demo

Demonstrates how `helix evolve` threads a new requirement through the entire artifact stack.

Starting from a working temperature converter (Fahrenheit/Celsius), a new requirement arrives: "Add Kelvin support." The agent updates the PRD, feature spec, and technical design, creates tracker issues, then implements the change via TDD.

## Prerequisites

- Docker
- Claude Code credentials (`~/.claude/`, `~/.claude.json`)
- DDx binary (mounted or in PATH)

## Run

```bash
# From the helix repo root:
docker build -t helix-evolve-demo docs/demos/helix-evolve/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-evolve/recordings:/recordings \
  helix-evolve-demo
```

## What It Does

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Create a working v1 project with existing artifacts and code |
| 2 | Evolve | Thread "Add Kelvin" through PRD, feature spec, design, tracker |
| 3 | Build | TDD: failing Kelvin tests (Red), then implementation (Green) |
| 4 | Verify | All tests pass, new CLI flags work |

## Recordings

```bash
asciinema play recordings/helix-evolve-*.cast
```
