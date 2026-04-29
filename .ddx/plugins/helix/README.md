# HELIX

A supervisory autopilot for AI-assisted software delivery. HELIX enforces
specification-first, test-first discipline through structured phases — framing
the problem, designing the solution, writing failing tests, building the
implementation, and iterating with real feedback. Humans set direction and make
judgment calls; AI agents do the heavy lifting under HELIX's supervision.

HELIX is built on [DDx](https://documentdrivendx.github.io/ddx/) (Document-Driven
Development eXperience), a platform for AI-assisted development. DDx
provides the foundation that HELIX runs on: a document library for governing
artifacts, a work tracker for issue management, an agent harness for dispatching
AI models, and an execution engine for recording what happened. HELIX adds the
methodology layer — the phases, the authority order, the bounded execution loop,
and the skills that turn governing documents into working software.

**[Documentation](https://documentdrivendx.github.io/helix/)** · **[Demo Reels](https://documentdrivendx.github.io/helix/docs/demos/)** · **[Getting Started](https://documentdrivendx.github.io/helix/docs/getting-started/)**

![HELIX Quickstart Demo](docs/demos/helix-quickstart/recordings/helix-quickstart.gif)

## Install

First, install [DDx](https://documentdrivendx.github.io/ddx/):

```bash
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
```

Then install HELIX:

```bash
ddx install helix
```

You'll also need [Claude Code](https://claude.ai/claude-code) (or another
agent CLI like `codex`), plus `bash` and `git`.

## Quick Start

Open Claude Code in your project and tell it what you want to build:

```
> I want to build a REST API for managing bookmarks. Use /helix-frame to get started.
```

Claude loads the HELIX skills automatically and begins the structured
workflow — creating a product vision, PRD, and feature specs. From there:

```
> /helix-run
```

HELIX takes over: it designs the solution, writes failing tests, implements
the code, reviews the work, and iterates — stopping only when human judgment
is needed or the queue is empty.

### Interactive commands

Inside a Claude Code session, HELIX skills are available as slash commands:

| Command | What it does |
|---------|-------------|
| `/helix-run` | Autopilot: build → review → check → repeat |
| `/helix-frame` | Create vision, PRD, and feature specs |
| `/helix-build` | Execute one ready issue end-to-end |
| `/helix-design auth` | Design a subsystem through iterative refinement |
| `/helix-review` | Fresh-eyes review of recent work |
| `/helix-evolve "add OAuth"` | Thread a new requirement through the artifact stack |
| `/helix-check` | What should happen next? |
| `/helix-align` | Top-down reconciliation review |
| `/helix-triage "Fix login bug"` | Create a well-structured tracker issue |
| `/helix-status` | Queue health and lifecycle snapshot |

### CLI (automation and scripting)

For background execution, CI integration, or scripting, the same commands
are available as a shell CLI:

```bash
helix run --agent claude --summary    # Background autopilot
helix start                           # Daemon mode with PID file
helix status                          # Check progress
helix stop                            # Stop the daemon
```

## Phases

HELIX guides work through six phases. Each phase produces governing
artifacts that drive the next:

0. **Discover** (optional) — Validate the opportunity
1. **Frame** — Define the problem and establish context
2. **Design** — Architect the solution approach
3. **Test** — Write failing tests that define behavior
4. **Build** — Implement code to make tests pass
5. **Deploy** — Release to production with monitoring
6. **Iterate** — Learn and improve for the next cycle

## How it works

1. You describe what you want to build in a Claude Code session
2. HELIX creates governing artifacts (vision → requirements → design → tests)
3. The tracker breaks work into issues with acceptance criteria
4. Agents claim issues, implement them, and verify against the tests
5. Reviews catch bugs that implementation blindness misses
6. The loop continues until the queue drains or human input is needed

The key insight: **documents drive the agents**. Requirements define what to
build. Designs define how. Tests define done. The tracker sequences the work.
HELIX orchestrates the agents to follow this chain — and stops them when they
drift.

## Workflow Contract

The HELIX methodology lives in `workflows/` and covers:

- [Workflow Overview](workflows/README.md) — phases, authority order, execution model
- [Execution Guide](workflows/EXECUTION.md) — operator flow, queue control, loop behavior
- [Reference Card](workflows/REFERENCE.md) — quick lookup for actions and concepts
- Tracker: `ddx bead --help` — issue management conventions

## DDx Platform

HELIX is built on [DDx](https://documentdrivendx.github.io/ddx/) — a
platform for AI-assisted development. DDx provides the document library, work
tracker, agent harness, and execution engine. HELIX provides the methodology,
phases, authority order, and supervisory skills.

See [DDx Methodology](workflows/DDX.md) for the artifact graph, authority
hierarchy, evolution model, and agent context model.
