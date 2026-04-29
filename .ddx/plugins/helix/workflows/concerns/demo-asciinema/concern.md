# Concern: Demo Reels (Asciinema)

## Category
demo

## Areas
all

## Components

- **Recording tool**: Asciinema (`asciinema rec`) — terminal session recording
- **Playback**: asciinema-player (JavaScript, embedded in microsite)
- **Static export**: GIF via `agg` (asciinema gif generator) for README/social
- **Reproducible capture**: Docker container with pinned dependencies
- **Demo scripts**: Shell scripts that drive the recorded session

## Constraints

- Recordings are `.cast` files (asciinema v2 format, JSON lines)
- Cast files live in `website/static/demos/` for microsite embedding and
  in `docs/demos/<name>/recordings/` as source-of-truth archives
- Demo scripts live in `docs/demos/<name>/demo.sh` with a companion
  `Dockerfile` and `README.md`
- GIF exports go alongside cast files for use in README and social previews
- Recordings must be reproducible — re-running the demo script in the Docker
  container should produce equivalent output
- Do not record interactive sessions manually — always use a scripted demo
- Terminal dimensions: 100 columns x 30 rows (standard for readable recordings)
- Playback speed: 1.5x default in the asciinema-player embed

## Demo Script Requirements

Every demo must include these files in `docs/demos/<name>/`:

| File | Purpose |
|------|---------|
| `demo.sh` | Shell script that drives the demo end-to-end |
| `Dockerfile` | Reproducible environment for recording |
| `README.md` | What it demonstrates, prerequisites, how to run |
| `recordings/` | Directory for `.cast` and `.gif` output |

### demo.sh conventions

- `#!/usr/bin/env bash` with `set -euo pipefail`
- Define helper functions: `narrate()` for section titles, `run()` for
  visible command execution, `show_file()` for file previews
- Use `sleep` between sections for pacing in recordings
- Print visible section headers (`narrate "ACT N: Title"`) so the viewer
  can follow the narrative structure
- Support both Docker and local execution — auto-detect environment
- Include retry logic for network-dependent commands (API calls, installs)
- Exit non-zero on failure — broken demos must not produce recordings

### Narrative structure

A demo reel tells a story. Structure it as acts:

1. **Setup** — Install tools, initialize project, show starting state
2. **Core workflow** — Demonstrate the main capability step by step
3. **Verification** — Show that the result is correct (tests pass, output matches)
4. **Summary** — Recap what happened, show final state

Each act should be self-explanatory to a viewer watching the recording at
1.5x speed without audio. Use section headers and visible command output
as narration.

### What to include in a demo

- The tool's primary workflow from start to finish
- Real commands that a user would actually type
- Visible output that proves the tool works
- File previews that show generated artifacts
- A verification step that confirms correctness
- A final summary showing the end state

### What NOT to include

- Long build times or dependency installation (fast-forward or pre-install)
- Error recovery (unless error handling IS the demo topic)
- Configuration that distracts from the main workflow
- Pauses longer than 3 seconds without visible activity

## Interactive Session Demos (tmux pattern)

The default demo format shows an **interactive Claude Code session**, not
bare CLI commands. Viewers should see the experience they'll actually have:
opening Claude, describing what they want, and watching HELIX work.

This requires automating an interactive terminal application. The pattern
uses **tmux + tmux send-keys** to script keystrokes into an interactive
session while asciinema records the outer terminal.

### How it works

```
asciinema rec (captures the tmux pane output)
  └─ tmux session
       └─ claude (interactive Claude Code session)
            ← tmux send-keys types prompts and commands
```

1. `demo.sh` starts a tmux session and launches `claude` inside it
2. `asciinema rec` records the tmux pane output
3. `tmux send-keys` types user prompts into the Claude session
4. `sleep` controls pacing between interactions
5. The recording captures the full interactive experience — Claude's
   responses, tool calls, and file changes are all visible

### demo.sh interactive pattern

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION="helix-demo"
RECORDING="/recordings/demo-$(date +%Y%m%d-%H%M%S).cast"

# Start tmux session with claude
tmux new-session -d -s "$SESSION" -x 100 -y 30
tmux send-keys -t "$SESSION" "cd /workspace/demo-project" Enter
sleep 1

# Start asciinema recording of the tmux pane
asciinema rec --cols 100 --rows 30 --command "tmux attach -t $SESSION" "$RECORDING" &
ASCIINEMA_PID=$!
sleep 1

# ACT 1: Open Claude and describe the project
tmux send-keys -t "$SESSION" "claude" Enter
sleep 3  # wait for Claude to initialize

tmux send-keys -t "$SESSION" "I want to build a CLI that converts temperatures. Use /helix-frame to get started." Enter
sleep 30  # wait for Claude to complete framing

# ACT 2: Run the autopilot
tmux send-keys -t "$SESSION" "/helix-run" Enter
sleep 60  # wait for build cycle

# ACT 3: Verify
tmux send-keys -t "$SESSION" "Show me the test results and the final code." Enter
sleep 15

# Exit Claude and stop recording
tmux send-keys -t "$SESSION" "/exit" Enter
sleep 2
tmux kill-session -t "$SESSION"
wait "$ASCIINEMA_PID" || true
```

### Key conventions for interactive demos

- **Type at human speed**: Use `tmux send-keys -l` with delays or break
  long prompts into chunks with short sleeps. A wall of text appearing
  instantly looks robotic.
- **Wait for completion**: Each Claude response takes time. Use generous
  `sleep` values (30-60s for framing, 60-120s for build cycles). Check
  for completion markers if possible.
- **Show the conversation**: The viewer should see both the human prompt
  and Claude's response. This is the natural tmux capture behavior.
- **Use slash commands**: Show `/helix-run`, `/helix-review`, etc. as the
  primary interaction — this is how users will actually invoke HELIX.
- **Natural language too**: Show at least one natural-language prompt
  ("add OAuth support") to demonstrate that Claude understands context
  without slash commands.

### When to use interactive vs. CLI demos

| Demo type | Use when |
|-----------|----------|
| **Interactive (tmux)** | Showing the user experience, onboarding, new features |
| **CLI (direct)** | Showing automation, CI integration, scripting patterns |

Interactive demos are the **default**. CLI demos are for the automation
section of the docs.

## Dockerfile Pattern (interactive)

The interactive demo Dockerfile adds tmux to the base image:

```dockerfile
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates jq tmux <project-deps> \
    && rm -rf /var/lib/apt/lists/*
```

## Dockerfile Pattern (CLI)

```dockerfile
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl ca-certificates jq <project-deps> \
    && rm -rf /var/lib/apt/lists/*

# Install asciinema
RUN pipx install asciinema
ENV PATH="/root/.local/bin:$PATH"

# Install project tools (including the agent CLI)
RUN npm install -g @anthropic-ai/claude-code

# Git identity for commits inside the container
RUN git config --global user.name "Demo" \
    && git config --global user.email "demo@project.dev" \
    && git config --global init.defaultBranch main

ENV SHELL=/bin/bash TERM=xterm-256color

WORKDIR /workspace

COPY demo.sh /usr/local/bin/demo.sh
RUN chmod +x /usr/local/bin/demo.sh

ENTRYPOINT ["/usr/local/bin/demo.sh"]
```

## Agent Credential Mounting

Demo containers that invoke AI agents (via `ddx agent run`)
need the user's Claude CLI credentials mounted into the container. The Claude
CLI stores authentication in two locations:

| Host path | Container path | Mode | Purpose |
|-----------|---------------|------|---------|
| `~/.claude.json` | `/root/.claude.json` | `ro` | CLI config and session metadata |
| `~/.claude/` | `/root/.claude/` | `rw` | Auth tokens, session state, cache |

The `~/.claude/` mount must be **read-write** because the Claude CLI writes
session state during execution. The `~/.claude.json` config file can be
read-only.

Additional optional mounts:

| Host path | Container path | Mode | Purpose |
|-----------|---------------|------|---------|
| Project repo | `/helix` (or `/project`) | `ro` | Source repo for skill/workflow access |
| DDx binary | `/usr/local/bin/ddx` | `ro` | DDx CLI (if not installed in image) |
| Recordings dir | `/recordings` | `rw` | Asciinema output extraction |

### Complete `docker run` command

```bash
docker run --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude \
  -v $(pwd):/helix:ro \
  -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
  -v $(pwd)/docs/demos/<name>/recordings:/recordings \
  <project>-demo
```

This pattern is fully autonomous — an agent or CI job can build and run the
container without interactive authentication. The user's existing Claude CLI
session is reused.

### Agent harness

Demo scripts must use `ddx agent run` as the harness:

```bash
ddx agent run \
  --harness claude \
  --text "$prompt"
```

This routes through DDx's agent abstraction, which provides output capture,
token tracking, and session logging.

### Deterministic replay with virtual harness

For reproducible demos that produce identical output every time (no tokens
consumed, no network dependency), use DDx's virtual agent harness.

**Step 1 — Record responses** by adding `--record` to a live agent run:

```bash
ddx agent run \
  --harness claude \
  --record \
  --text "$prompt"
```

This executes the prompt with the real agent and saves the prompt→response
pair to `.ddx/agent-dictionary/<hash>.json` (hash is a truncated SHA-256 of
the prompt text).

**Step 2 — Replay** by switching the harness to `virtual`:

```bash
ddx agent run \
  --harness virtual \
  --text "$prompt"
```

The virtual harness looks up the prompt hash in the dictionary and returns
the recorded response. No agent binary is invoked, no tokens are consumed,
and timing is simulated from the original run.

**Demo script pattern:**

```bash
HARNESS="${DEMO_HARNESS:-claude}"

ddx agent run \
  --harness "$HARNESS" \
  --text "$prompt"
```

First run: `DEMO_HARNESS=claude ./demo.sh` (live, optionally with `--record`).
Subsequent runs: `DEMO_HARNESS=virtual ./demo.sh` (deterministic replay).

**Notes:**
- The `.ddx/agent-dictionary/` directory should be committed to git so
  recordings are shared and versioned.
- Re-record when prompts change — the hash is prompt-exact.
- The virtual harness is always available (`ddx agent list` shows it
  regardless of installed agent binaries).

### Permissions

Demos need file and command permissions. Do **not** use `--permissions
unrestricted` — it is unreliable across DDx versions. Instead, the demo
script should create `.claude/settings.json` with pre-approved permissions
in the demo project directory before any agent calls:

```json
{
  "permissions": {
    "allow": ["Bash(*)", "Read(*)", "Write(*)", "Edit(*)"]
  }
}
```

This grants the Claude CLI all necessary permissions via its settings
file, which is the supported mechanism.

## Microsite Embedding

Use the `asciinema` Hugo shortcode (see `hugo-hextra` concern):

```markdown
{{</* asciinema src="demo-name" cols="100" rows="30" */>}}
```

The shortcode loads asciinema-player from CDN and plays `static/demos/<src>.cast`
with monokai theme, autoplay, and 1.5x speed.

Copy the `.cast` file to `website/static/demos/` after recording.

## Recording Workflow

1. Build the Docker image:
   ```bash
   docker build -t <project>-demo docs/demos/<name>/
   ```
2. Run with credential and recording mounts:
   ```bash
   docker run --rm \
     -v ~/.claude.json:/root/.claude.json:ro \
     -v ~/.claude:/root/.claude \
     -v $(pwd):/helix:ro \
     -v $(pwd)/../ddx/ddx:/usr/local/bin/ddx:ro \
     -v $(pwd)/docs/demos/<name>/recordings:/recordings \
     <project>-demo
   ```
3. Review the cast file: `asciinema play recordings/<file>.cast`
4. Generate GIF: `agg recordings/<file>.cast recordings/<file>.gif`
5. Copy cast to microsite: `cp recordings/<file>.cast website/static/demos/`
6. Embed in content with `asciinema` shortcode

Steps 1-2 are fully autonomous and can be run by an agent with access to
the user's home directory. Steps 3-6 are post-processing that can also
be automated.

## When to use

Any project that needs to demonstrate a CLI workflow, developer tool, or
terminal-based process. Demo reels are more effective than screenshots for
showing multi-step workflows and more maintainable than screen recordings
because they can be regenerated from scripts.

## ADR References
