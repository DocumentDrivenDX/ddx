---
title: Getting Started
weight: 1
prev: /docs
next: /docs/workflow
---

Get HELIX installed and running your first supervised build session.

## DDx and HELIX

HELIX is built on [DDx](https://documentdrivendx.github.io/ddx/) (Document-Driven
Development eXperience), a platform for AI-assisted development. DDx
provides the foundation: a **document library** for managing governing artifacts
like specs and designs, a **work tracker** (`ddx bead`) for issue management with
dependencies and claims, an **agent harness** (`ddx agent`) for dispatching AI
models with token tracking, and an **execution engine** (`ddx exec`) for recording
structured evidence of what happened.

HELIX adds the methodology on top — the development phases
(Frame → Design → Test → Build → Deploy → Iterate), the authority order that
resolves conflicts between artifacts, the bounded execution loop that decides
what to do next, and the skills that turn all of this into agent instructions.
You install DDx first, then install HELIX as a DDx package.

## Install

First, install DDx:

```bash
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
```

Then install HELIX:

```bash
ddx install helix
```

You'll also need [Claude Code](https://claude.ai/claude-code) (or another
agent CLI like `codex`), plus `bash` and `git`.

## Start Building

Open Claude Code in your project directory and describe what you want to build:

```
> I want to build a REST API for managing bookmarks. Use /helix-frame to get started.
```

Claude loads the HELIX skills automatically and begins the structured
workflow — creating a product vision, PRD, and feature specs based on your
description. The governing artifacts it creates will drive everything
downstream.

## Understand the Artifact Hierarchy

After framing, your project has governing artifacts at different zoom levels:

```
Product Vision          "What is this and why?"
  └── PRD               "What must it do?"
       └── Feature Spec  "What exactly does this feature do?"
            └── Bead     "One unit of work to implement it"
```

Higher levels govern lower levels. If a feature spec contradicts the PRD,
the PRD wins. HELIX enforces this automatically — you don't need to remember
the hierarchy, but understanding it helps you steer effectively.

## Add Work to the Tracker

HELIX works from a tracker queue. After framing, you can add specific work
items:

```
> /helix-triage "Add user authentication with OAuth"
```

This creates a well-structured bead with acceptance criteria, spec references,
and a context digest that tells the implementing agent everything it needs to
know. You can also add beads directly:

```bash
ddx bead create "Add OAuth login flow" --type task \
  --labels helix,phase:build --set spec-id=FEAT-001 \
  --acceptance "OAuth login redirects to provider and returns a session token"
```

## Run the Autopilot

Once framing is complete and beads exist, start the autopilot:

```
> /helix-run
```

HELIX takes over from here. It reads the governing artifacts, designs the
solution, writes failing tests, implements the code to make them pass, reviews
the work for bugs, and iterates — stopping only when human judgment is needed
or the work queue is empty.

## Interactive Commands

Inside a Claude Code session, HELIX skills are available as slash commands.
You can invoke them at any time to steer the work:

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
| `/helix-experiment` | Metric-driven optimization loop |
| `/helix-polish` | Refine issues before implementation |

You can also just describe what you want in natural language — Claude
understands HELIX context and will invoke the right skills:

```
> The auth module needs OAuth support. Thread that through the specs and design.
> Review the last commit for security issues.
> What should we work on next?
```

## Background Execution (CLI)

For long-running work, CI integration, or scripting, the same commands are
available as a shell CLI:

```bash
helix start                           # Daemon mode with PID file
helix status                          # Check progress
helix stop                            # Stop the daemon
```

Or run directly:

```bash
helix run --agent claude --summary    # Background autopilot with concise output
helix run --max-cycles 10             # Stop after 10 completed build cycles
```

## Next Steps

- Read about the [HELIX workflow](../workflow) and how phases work
- See the full [CLI reference](../cli) for automation and scripting
- Watch the [demo reels](../demos) of HELIX in action
