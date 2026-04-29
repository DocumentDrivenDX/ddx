# HELIX Concerns Demo

Demonstrates how HELIX concerns prevent technology drift.

A Bun/TypeScript project declares `typescript-bun` as its active concern. The agent reads the concern, builds with Bun-native tools (Bun.serve, bun:test, Biome), then a deliberate drift is introduced (vitest import) and the review catches it.

## Prerequisites

- Docker
- Claude Code credentials (`~/.claude/`, `~/.claude.json`)
- DDx binary (mounted or in PATH)

## Run

```bash
# From the helix repo root:
docker build -t helix-concerns-demo docs/demos/helix-concerns/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-concerns/recordings:/recordings \
  helix-concerns-demo
```

## What It Does

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Setup | Create a Bun project with `typescript-bun` concern declared |
| 2 | Frame | Agent reads concerns, creates PRD with Bun-native requirements |
| 3 | Build | Agent implements with Bun.serve(), bun:test, Biome |
| 4 | Drift | Deliberate Node.js drift introduced (vitest import) |
| 5 | Review | Agent detects concern drift and files a tracker issue |

## Recordings

```bash
asciinema play recordings/helix-concerns-*.cast
```
