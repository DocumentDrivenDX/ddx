# Status — Queue State and Health

Three distinct intents live here. They answer different questions
and have different commands; call them by intent, not
interchangeably.

## What's on the queue?

"What's ready to work on?" "What's blocked?"

```bash
ddx bead ready      # beads with all deps satisfied (actionable)
ddx bead blocked    # beads with at least one unclosed dep
ddx bead status     # aggregate counts: Open / Closed / Ready / Blocked
ddx bead list       # all beads (filter with --status, --label, --where)
```

Typical answers:

- "What should I work on next?" → `ddx bead ready` (top of list).
- "Why isn't bead X moving?" → `ddx bead show <id>` for full state;
  `ddx bead dep tree <id>` to see what it depends on.
- "How many beads are open right now?" → `ddx bead status`.

## Am I healthy?

"Is DDx installed and working?" "Are the harnesses available?"

```bash
ddx doctor          # environment, config, install validation
ddx agent doctor    # harness health (binaries, providers, auth)
ddx version         # CLI version
```

`ddx doctor` checks:

- DDx binary is reachable and recent.
- `.ddx/config.yaml` is valid (schema + required fields).
- `.ddx/beads.jsonl` is readable and parses.
- Git is available and the repo is in a usable state.
- Shell integration (where applicable) is set up.
- Plugins declared in config are actually installed.

`ddx agent doctor` goes further into the agent service: which
harnesses are discoverable (`claude`, `codex`, `gemini`, embedded
`agent`), whether provider credentials are present, whether the
model catalog loads.

## Is the project drifted from upstream?

"Has the DDx library changed since I last updated?" "Do I need to
pull new plugin content?"

```bash
ddx status          # sync state: upstream revs, local drift, stale docs
ddx doc stale       # documents that reference outdated artifacts
```

`ddx status` is the CLI-wide status: binary version, plugin
versions, document-graph freshness. `ddx doc stale` specifically
lists documents that depend on artifacts that have moved or
changed.

## Don't use these interchangeably

- `ddx doctor` — **environment health** (is DDx working on this
  machine?). Call after `ddx upgrade` or when things seem broken.
- `ddx agent doctor` — **harness health** (are the agents
  reachable?). Call when `ddx run` reports an agent setup or availability
  failure.
- `ddx status` — **upstream drift** (is this project in sync with
  the library registry?). Call periodically to catch stale
  plugins.
- `ddx bead status` — **work queue state** (how many beads, in
  what states?). Call to understand project progress.

A user asking "how's the project doing?" probably means `ddx
doctor` or `ddx agent doctor` (am I set up correctly) — follow up
with `ddx bead status` if they want work-queue state too.

A user asking "what's ready to work on?" is clearly asking for
`ddx bead ready`.

## Anti-patterns

- **Running `ddx status` to check if agents work.** Use
  `ddx agent doctor` for that; `ddx status` is about upstream sync.
- **Running `ddx doctor` to see the queue.** Use `ddx bead status`.
- **Closing stale beads based on "status" alone.** `ddx bead
  status` shows counts, not quality. Use `ddx bead list
  --status open` + `ddx bead show <id>` to actually read each bead.

## CLI reference

```bash
# Queue state
ddx bead ready
ddx bead blocked
ddx bead status
ddx bead list [--status open|closed] [--label <l>]
ddx bead show <id>

# Environment health
ddx doctor
ddx version

# Agent/harness health
ddx agent doctor
ddx agent list
ddx agent capabilities <harness>

# Upstream sync
ddx status
ddx doc stale
```

Full flag list: `ddx doctor --help`, `ddx agent doctor --help`,
`ddx bead --help`.
