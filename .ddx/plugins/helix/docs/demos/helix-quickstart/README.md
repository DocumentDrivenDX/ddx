# HELIX Quickstart Demo

A scripted demonstration of the HELIX onboarding path from sparse intent to
queue drain. New users should prefer `helix input` to shape the request and
`ddx agent execute-loop` to drain execution-ready work.

The current recording still shows the older phase-by-phase compatibility flow
inside the demo harness. It builds a tiny Node.js temperature converter from
scratch, driven entirely by HELIX artifacts and the tracker, with
`ddx agent run` used only to capture scripted agent responses during recording.

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
| 3 | Input | `helix input` would shape the request into governed work |
| 4 | Queue Drain | `ddx agent execute-loop` is the default execution path |
| 5 | Build | The recording visualizes the older phase-by-phase compatibility flow |
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
