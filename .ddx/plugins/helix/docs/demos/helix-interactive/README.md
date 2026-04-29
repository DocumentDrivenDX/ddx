# Interactive HELIX Demo

Shows a complete HELIX session inside Claude Code: framing a project,
running the autopilot, reviewing results, and evolving requirements.

This is the **default demo format** — it shows the experience users will
actually have when working with HELIX through an interactive Claude session.

## What it demonstrates

1. Opening Claude Code and describing a project
2. Using `/helix-frame` to create governing artifacts
3. Using `/helix-run` to execute the build loop
4. Reviewing test results through natural language
5. Using `/helix-evolve` to thread a new requirement

## How it works

The demo uses **tmux + tmux send-keys** to automate an interactive Claude
Code session while **asciinema** records the terminal output. The viewer
sees exactly what a real user sees — Claude's responses, tool calls, file
changes, and test results.

## Run with Docker

```bash
docker build -t helix-interactive-demo docs/demos/helix-interactive/
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/helix-interactive/recordings:/recordings \
  helix-interactive-demo
```

## Run locally

```bash
RECORDING_DIR=$(pwd)/docs/demos/helix-interactive/recordings \
  bash docs/demos/helix-interactive/demo.sh
```

Requires: tmux, asciinema, claude CLI, ddx, helix, node, npm, git, jq.

## Timing

The demo takes 5-10 minutes to complete depending on model response times.
The `wait_for_idle` function polls the tmux pane content and waits for Claude
to finish before sending the next prompt.
