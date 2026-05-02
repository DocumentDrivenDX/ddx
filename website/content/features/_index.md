---
title: Features
description: "DDx platform capabilities — stable, beta, and planned."
---

DDx is a focused set of platform primitives for AI-assisted development. Each capability below carries a maturity badge so you can tell what is solid today from what is still in motion.

## Artifact Graph {#artifact-graph} {{< maturity "beta" >}}

![Artifact graph view](/ui/feature-artifact-graph.png "Artifact graph — planned UI")

Documents are not the only thing a project ships. Diagrams, screenshots, schema files, fixtures, generated assets — all of them carry meaning, and all of them drift. DDx treats every artifact as a first-class node in a single graph: markdown documents directly, and non-markdown files (graphics, diagrams, binaries) through a `.ddx.yaml` sidecar that records kind, provenance, and links.

The graph exists to answer two questions:

- **Drift visibility** — which artifacts have moved out from under the documents that reference them, and which references now point at stale or missing targets?
- **Impact analysis** — if you change this spec, which features, beads, diagrams, and downstream artifacts are implicated?

```
ddx doc graph         # show the artifact graph
ddx doc audit         # surface drift and broken references
```

This is the spine the rest of the platform hangs from. Beads reference artifacts; executions produce artifacts; reviews assess artifacts. Keeping the graph honest keeps everything else honest.

## Beads & DAG {#beads-dag} {{< maturity "stable" >}}

![Bead list with dependency tree](/ui/feature-beads-dag.png "Beads and dependency DAG — planned UI")

Beads are the atomic unit of work. Each one carries a title, description, acceptance criteria, labels, state history, and a typed dependency edge to other beads. The dependency graph is a DAG, not a list: `ddx bead ready` returns only beads whose prerequisites are closed, so the queue is always a correct-by-construction work front.

```
ddx bead create "Add login endpoint" --ac "returns 200 on valid credentials"
ddx bead dep add <child> --on <parent>
ddx bead ready
ddx bead tree <id>
```

## Execute-Loop (`ddx work`) {#execute-loop} {{< maturity "stable" >}}

![Execute-loop draining the queue](/ui/feature-execute-loop.png "ddx work draining the queue — planned UI")

`ddx work` drains the bead queue. It picks the next ready bead, runs the configured agent harness inside an isolated git worktree, captures evidence, and merges the result back when acceptance criteria pass. Failing or timed-out runs are preserved as refs for inspection rather than discarded.

```
ddx work                              # drain the queue
ddx agent execute-bead <id> --from HEAD
```

Because each iteration runs in its own worktree, parallel runs are safe and a failed run never poisons the main branch.

### Why a loop, not a long session — bounded context execution

The execute-loop is the operational shape of **bounded context execution** — sometimes called the **Ralph loop**. LLM output quality decays as a single context window fills: transcript, tool output, and failed attempts accumulate and compete with the original instructions. We call this decay **context rot**, and it is why long-running agent sessions are unreliable even on frontier models.

`ddx work` answers context rot structurally. Each bead runs in a fresh agent invocation with a clean context, scoped tightly to the bead's description, acceptance criteria, and the files it touches. When the attempt ends — merged, preserved, or abandoned — the context is discarded. Persistent state lands on disk as evidence, code, an updated bead, or a `<review-findings>` block threaded into the next attempt's prompt. The next iteration starts cold and reads what it needs from the substrate.

The result: drain throughput stays steady across hundreds of beads. The agent never has to remember what attempt 47 was thinking. The loop does the remembering, on disk, in plain files.

See [Bounded Context Execution](/docs/concepts/bounded-context-execution/) for the full treatment.

## Evidence Capture {#evidence-capture} {{< maturity "stable" >}}

![Execution evidence bundle](/ui/feature-evidence-capture.png "Evidence capture bundle — planned UI")

Every execution writes a bundle under `.ddx/executions/<timestamp>-<hash>/` with the prompt, the agent transcript, token and cost telemetry, the diff produced, and the merge outcome. Evidence stays in the repository, attached to the commit, so future reviewers (human or model) can reconstruct what happened without rerunning the agent.

```
ddx agent log
ls .ddx/executions/
```

## Multi-Model Review {#multi-model-review} {{< maturity "framing" >}}

![Multi-model review consensus](/ui/feature-multi-model-review.png "Multi-model review — planned UI")

A single model is a single point of view. DDx can dispatch the same prompt — a plan, a diff, a spec — to multiple harnesses (Claude, Codex, Gemini, local models) and aggregate their findings. Quorum modes let you require majority or unanimous agreement before a result is treated as approved.

```
ddx agent run --quorum=majority --harnesses=claude,codex,gemini --prompt review.md
```

The pattern works for plan review before implementation, diff review before merge, and spec sanity-checks before breakdown into beads.

## Skills {#skills} {{< maturity "beta" >}}

![Skill listing](/ui/feature-skills.png "Skills — planned UI")

Skills are reusable, agent-invocable capabilities packaged alongside the project. They install under `.agents/skills/` and `.claude/skills/` and become available to any harness that supports skill discovery. Plugins ship skills the way they ship templates and prompts — versioned, project-local, no global state.

```
ddx install <plugin-name>     # plugins can carry skills
ls .claude/skills/
```

## Agent-Agnostic Dispatch {#agent-agnostic-dispatch} {{< maturity "stable" >}}

![Agent harness selection](/ui/feature-agent-dispatch.png "Agent-agnostic dispatch — planned UI")

DDx talks to agents through a uniform harness interface. Swap Claude for Codex for a local model by changing configuration, not commands. The same `ddx agent run` invocation works across providers; cost, tokens, and latency are recorded the same way regardless of which harness answered.

```
ddx agent list                # show available harnesses
ddx agent run --harness claude --prompt prompts/implement.md
ddx agent run --harness codex --prompt prompts/implement.md
ddx agent doctor              # harness health check
```

This is what makes cost-tiered routing and multi-model review possible: the rest of the platform never has to care which model is on the other end of the wire.
