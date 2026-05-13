# Status — Queue State and Health

## Mode note

This reference covers **direct queue queries and health checks** (`queue_steward`
and `interactive-steward` orient phase). Use it for:

- "What's on the queue?" "What's ready?" "Is DDx healthy?" "Sync status?"

If the user is asking a **broad planning or orientation question** — "what should
I work on next?", "what's blocking the queue?" — see `reference/interactive.md`
first. The interactive steward uses these commands to build `QueueFacts` but
does not instruct manual ready-bead implementation; `ddx work` is the queue
worker.

---

Three distinct intents live here. They answer different questions
and have different commands; call them by intent, not
interchangeably.

## What's on the queue?

"What's ready to work on?" "What's blocked?"

```bash
ddx bead ready      # execution-ready open beads; use --execution for worker view
ddx bead blocked    # status=blocked external/recheckable blockers
ddx bead status     # aggregate lifecycle and derived queue counts
ddx bead list       # all beads (filter with --status, --label, --where)
```

Typical answers:

- "What should I work on next?" → See `reference/interactive.md` for the
  interactive-steward planning workflow. The steward uses `ddx bead ready` to
  build `QueueFacts` but will not instruct you to manually implement ready beads.
- "Why isn't bead X moving?" → `ddx bead show <id>` for full state;
  `ddx bead dep tree <id>` to see what it depends on.
- "How many beads are open right now?" → `ddx bead status`.
- "What is waiting on dependencies?" → `ddx bead ready` / `ddx bead status`;
  dependency waits are derived queue state, not `status=blocked`.

## Are workers running for this project?

"Is the worker still working beads for this project?" "How is the queue
progressing?"

```bash
ddx work status            # live ddx work/ddx try processes for THIS project
ddx work status --json     # machine-readable
ddx work status --all-projects   # explicit cross-project escape hatch
```

`ddx work status` defaults to the current project root (CLI flag → env
`DDX_PROJECT_ROOT` → git root of CWD). The default view never reports
workers from another repository — surfacing a worker that belongs to a
different project as evidence that this project is "still working" is the
exact failure this command was added to prevent.

**Required when answering "is the worker still running for project X?":**

1. Resolve the requested project root before answering. Pass `--project`
   if you are not already in that root.
2. Use `ddx work status` (or `ddx work status --json`) for the live-worker
   evidence. Never substitute a raw global `ps aux | grep -E 'ddx work|ddx try'`
   scan: it returns workers from every checkout on the host and produces
   a misleading "yes, working" signal for whichever project happens to be
   asked.
3. Always name the project root the worker status applies to ("for
   `<project>`: …"). A bare "yes, a worker is running" answer is unsafe
   because the reader cannot tell which project it refers to.
4. If a live worker belongs to a *different* project, mention it only as
   explicitly out-of-scope context (`--all-projects` view), not as
   evidence that the asked-about project is progressing.

## Am I healthy?

"Is DDx installed and working?" "Are the harnesses available?"

```bash
ddx doctor          # environment, config, install validation
# Fizeau diagnostics, if exposed by the project, remain separate from DDx.
ddx version         # CLI version
```

`ddx doctor` checks:

- DDx binary is reachable and recent.
- `.ddx/config.yaml` is valid (schema + required fields).
- `.ddx/beads.jsonl` is readable and parses.
- Git is available and the repo is in a usable state.
- Shell integration (where applicable) is set up.
- Plugins declared in config are actually installed.

Fizeau diagnostics go further into the execution service: which
harnesses are discoverable, whether provider credentials are present,
and whether routing metadata loads.

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
- Fizeau diagnostics — **harness health** (are the routes reachable?).
  Call when `ddx run` reports an availability failure.
- `ddx status` — **upstream drift** (is this project in sync with
  the library registry?). Call periodically to catch stale
  plugins.
- `ddx bead status` — **work queue state** (how many beads, in
  what states?). Call to understand project progress.

A user asking "how's the project doing?" probably means `ddx
doctor` or the Fizeau diagnostics surface (am I set up correctly) —
follow up with `ddx bead status` if they want work-queue state too.

A user asking "what's ready to work on?" is clearly asking for
`ddx bead ready`.

## Anti-patterns

- **Running `ddx status` to check if routes work.** Use
  the Fizeau diagnostics surface for that; `ddx status` is about upstream sync.
- **Running `ddx doctor` to see the queue.** Use `ddx bead status`.
- **Closing stale beads based on "status" alone.** `ddx bead
  status` shows counts, not quality. Use `ddx bead list
  --status open` + `ddx bead show <id>` to actually read each bead.
- **Answering "is the worker running?" with a global `ps aux | grep -E
  'ddx work|ddx try'` scan.** Without project-root filtering this surfaces
  workers from other repositories and falsely signals progress on the
  project that was asked about. Use `ddx work status` (project-scoped by
  default) and only fall back to `--all-projects` for an explicit
  cross-project view.

## CLI reference

```bash
# Queue state
ddx bead ready
ddx bead blocked
ddx bead status
ddx bead list [--status open|closed] [--label <l>]
ddx bead show <id>

# Live workers for this project (defaults to current project root)
ddx work status
ddx work status --json
ddx work status --all-projects   # opt-in cross-project view

# Environment health
ddx doctor
ddx version

# Agent/harness health
# Fizeau diagnostics, if exposed by the project, remain separate from DDx.

# Upstream sync
ddx status
ddx doc stale
```

Full flag list: `ddx doctor --help`,
`ddx bead --help`.
