---
title: Why DDx
description: "The new reality of agent-driven development, the pain it creates, the physics behind it, and the four primitives DDx provides in response."
weight: 1
---

# Why DDx

Agents now write most of the code. The tools, conventions, and habits we built for human-only teams quietly stop working when the team is half humans and half AI. DDx exists because the substrate underneath agent-driven development is missing — and without it, shipping software with agents degrades into prompt-and-pray.

This page lays out the argument: what changed, what hurts, why it hurts, and what we built in response.

## 1. The New Reality

A few things are true now that weren't true two years ago:

- **Agents execute, humans direct.** A working developer steers, reviews, and approves; the agent writes, edits, and runs the commands. The ratio of agent-authored to human-authored lines keeps climbing.
- **Every invocation starts from zero.** Agents have no persistent memory between runs. Whatever context an agent needs to do good work has to be assembled and handed in every time.
- **Output volume outpaces review capacity.** A single developer can dispatch more work in an hour than they could write in a week. The bottleneck moves from typing to deciding what to keep.
- **Cost is now a first-order constraint.** Token spend is real money on every iteration. Choosing the right model for the job is a daily concern, not a one-time configuration.
- **Work is parallel and interleaved.** Multiple agents drain a queue concurrently; humans pick up some beads, agents pick up others. Coordination has to survive concurrency.

The shape of "writing software" has changed. The tools haven't.

## 2. Six Pain Points

What teams hit, in the order they hit them:

### Constant context re-explanation
Every agent invocation requires re-stating the same project context: what we're building, the conventions, the prior decisions, the specs that govern this change. Without a shared, durable artifact substrate, this overhead repeats forever. Each session starts from zero.

### Specs and code drift apart
A spec is updated; the code isn't. Or the code changes; the spec doesn't. Without explicit relationships between artifacts and a way to detect staleness, drift accumulates silently until a major bug or onboarding session exposes it.

### Work-tracking reinvented per tool
Every workflow tool, every methodology, every project ends up rebuilding the same issue store, dependency tracker, and ready-queue from scratch. The work substrate is local infrastructure that should be shared, not re-implemented per tool.

### Execution evidence evaporates
Agents run, produce output, and that output disappears into a terminal scrollback. There's no durable record of what was run, what model was used, what tokens were spent, what files changed, or what the result was. Without evidence, evaluation, debugging, and learning all degrade to anecdote.

### Routine work runs on premium models
Without capability-aware routing, every bead runs on the same model — usually the strongest one configured. Cheap, reliable, mechanical work pays the price of frontier-model inference. There's no signal on which model is the cheapest one that reliably closes which kind of bead.

### Agents re-learn each project's CLI
Every project has a slightly different shape. Without a discoverable, consistent surface, agents waste tokens probing for commands, conventions, and entry points. The harness keeps re-deriving facts the project already knows.

## 3. The Root Cause

Step back from the symptoms and there are two underlying failure modes, not one:

> **Agents execute in isolation with no shared memory.**
>
> **Within a single run, agents suffer context rot — quality decays as the context window fills.**

The first explains the cross-session pain. Context re-explanation is the absence of shared memory across sessions. Spec/code drift is the absence of shared memory across artifacts. Work-tracking reinvention is the absence of shared memory across tools. Evidence loss is the absence of shared memory across runs. Cost overspend is the absence of shared memory about what worked at what tier. Project re-learning is the absence of shared memory about the project's own surface.

The second explains why the obvious workaround — "just run a longer session" — fails. **Context rot** is the quality decay LLMs exhibit as a single context window fills with transcript, tool output, failed attempts, and partial reasoning. Even within one run, unbounded execution degrades the agent: original instructions get crowded out, retrieved facts blur, and earlier mistakes pollute later reasoning. The agent at hour one is sharper than the agent at hour three, at the same model and the same prompt. Long-running agent sessions trade reliability for the illusion of continuity. So even a perfectly remembered session would still rot from the inside.

These two failure modes need a shared structural fix: **bounded context execution** — also known as the **Ralph loop**. Every unit of agent work runs in a fresh, narrowly-scoped context against an explicit contract, and durable state lives on disk as evidence rather than as transcript carried forward. The next attempt re-enters cold, reads what it needs from the substrate, and executes against the same kind of contract. Iteration becomes reliable because no single iteration has to remember anything, and no single iteration runs long enough to rot. **This is exactly why `ddx work` is designed the way it is**: each bead drains in its own short-lived worktree with a fresh context, evidence is written to disk, and the loop moves on rather than stretching one session past its useful window.

Patching symptom-by-symptom doesn't fix either failure mode. What's needed is a substrate that gives agent work a place to stand — durable, file-based, agent-discoverable, and shared across every invocation — plus a loop shape that respects context rot instead of fighting it.

DDx answers with four primitives:

1. **Artifacts** — versioned, related, discoverable project knowledge. Documents, diagrams, prompts, personas, and other media live in the repo with explicit identity and relationships. The artifact graph tracks staleness when an upstream artifact changes.
2. **Beads** — the unit of work. Self-contained items with acceptance criteria and a dependency DAG. The ready/blocked queue is the interface between intent and execution. Beads make intent legible to humans and agents alike.
3. **Tracked execution (`ddx work`)** — drain the bead queue. Each attempt runs in an isolated worktree, captures evidence (model, tokens, files changed, exit metadata), and either merges back on success or preserves the result for inspection. The history persists.
4. **Skills** — a single, consolidated agent-facing surface. One DDx skill teaches an agent how to operate the toolkit: bead triage, agent dispatch, plugin installation, evidence inspection. Agents stop probing; they read the skill once.

These four primitives are the shared memory the agent-driven workflow was missing.

## 4. The Physics

Underneath the pain points and the response are six load-bearing claims about how software gets built and how generative AI behaves. We call these the *physics* — facts you can either respect or pay the cost of ignoring.

### Physics of Software

1. **Abstraction is the lever.** Multi-level artifact stacks with maintained relationships are how intent propagates without being lost. True for human teams; load-bearing for agents because they don't carry tacit knowledge between invocations. If the artifact stack isn't there, every invocation rebuilds context from scratch and gets it slightly wrong.

2. **Software is iteration over tracked work.** Repeated trials over an explicit work substrate — beads, queues, dependency DAGs — is how software gets built. This pre-existed agents; agents make the substrate non-optional. Untracked work cannot be drained, evaluated, or coordinated across multiple agents.

3. **Methodology is plural.** Different teams, projects, and problem domains demand different workflows — waterfall, agile, kanban, ad-hoc. No tool that bakes one in survives the rest. The platform must provide primitives that any methodology composes; opinions belong in optional plugins.

### Physics of Generative AI

4. **LLMs are stochastic, unreliable, and costly.** Quality degrades as the context window fills. Cost-tier ladders, ensemble verification, and "cheapest model that reliably closes the bead" are the operating shape of agentic work, not optimizations to add later. Treating LLM calls as if they were deterministic function calls produces brittle workflows and surprise bills.

5. **Evidence provides memory.** Agents carry no state between invocations and outputs aren't bit-reproducible. The only thing that survives a run is what was captured as it happened. That captured evidence is the substrate for evaluation, trust, debugging, and learning — without it, every other principle degrades to anecdote.

### The Intersection

6. **Human-AI collaboration is the fulcrum.** Abstraction levers intent across the artifact stack, but only collaboration converts leverage into shipped software. Humans supply intent and accountability; AI supplies volume and execution. Handoffs run in both directions, at every level of the stack — and the toolkit at the seam decides whether those handoffs are smooth or expensive.

The four primitives are the choices DDx makes in response to these six. Artifacts honor #1. Beads honor #2 and #3. Tracked execution honors #4 and #5. Skills honor #6.

## 5. DDx as the Response

Put it together and the operating model is straightforward:

- **Project knowledge lives in the repo as artifacts.** Plain files, plain git, agent-discoverable. Identity and relationships are explicit, not inferred. Staleness is detected, not discovered the hard way.
- **Work flows through beads.** Anything an agent does is against a bead. The bead carries acceptance criteria; review checks the criteria, not memory of the conversation. The queue is the contract between intent and execution.
- **Agents run via `ddx work`.** The queue drains in isolated worktrees, evidence is captured automatically, and successful results merge back. Failed or timed-out runs are preserved for inspection. Token cost, model choice, and outcome are all recorded.
- **Agents learn the toolkit through one skill.** One consolidated DDx skill replaces a fleet of project-specific instructions. Every project that uses DDx looks the same to the agent.
- **Everything is local-first.** `ddx init` and `ddx install <plugin>` only touch the project root. There's no home-directory state, no machine-wide opinions, no proprietary backend. Cloning the repo gives you the whole picture.
- **Every primitive is workflow-agnostic.** DDx ships no methodology. Phases, gates, and supervisory loops belong in plugins, not in the platform.

The result: agents stop starting from zero. Work has a place to live. Evidence persists. The substrate that agent-driven development was missing is on disk, in the repo, in plain files — exactly where everything else already is.

That's why DDx exists.

---

Ready to try it? Start with [Get Started](/docs/getting-started/) or browse the [Features](/features/).
