# HELIX Quickstart Demo

A scripted demonstration of the full HELIX lifecycle: install → frame → design → build → review.

Builds a tiny Node.js temperature converter from scratch, driven entirely by HELIX artifacts and the tracker. Uses `ddx agent run` as the agent harness.

## Prerequisites

- Docker
- Claude Code credentials (`~/.claude/`, `~/.claude.json`)
- DDx binary (mounted or in PATH)

## Run

```bash
# From the helix repo root:

# Build the demo container
docker build -t helix-demo docs/demos/helix-quickstart/

# Run with recording (Docker)
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-quickstart/recordings:/recordings \
  helix-demo

# Run locally (no Docker) — uses your existing tools
cd /tmp && bash /path/to/helix/docs/demos/helix-quickstart/demo.sh
```

## What It Does

| Act | Phase | What Happens |
|-----|-------|-------------|
| 1 | Install | Install HELIX skills and CLI, verify ddx agent harness |
| 2 | Setup | Initialize git repo, tracker, and AGENTS.md |
| 3 | Frame | Agent creates product vision, PRD, and feature spec |
| 4 | Design | Agent creates technical design, then tracker issues |
| 5 | Build | Red: write failing tests. Green: implement to pass. |
| 6 | Verify | Run tests, check acceptance criteria |
| 7 | Review | Agent reviews all work for gaps |

## Recordings

Asciinema recordings are saved to `recordings/`. Play them:

```bash
asciinema play recordings/helix-quickstart-*.cast
```

## Run Locally (no Docker)

```bash
cd /tmp
bash /path/to/helix/docs/demos/helix-quickstart/demo.sh
```

Requires: git, jq, node, npm, ddx, helix CLI.
