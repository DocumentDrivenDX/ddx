---
ddx:
  id: helix.workflow.principles
  depends_on:
    - RSCH-001
    - RSCH-002
    - RSCH-003
    - RSCH-004
    - RSCH-005
    - RSCH-006
    - RSCH-007
    - RSCH-008
    - RSCH-009
    - RSCH-010
---
# DDx Domain Principles

> For internal engineering principles, see [docs/dev/engineering-principles.md](../../dev/engineering-principles.md).

## Preface: Factory-Floor Design Choices

DDx is a document-driven software factory. The product it ships is not a
single application — it is the machinery that lets specifications, beads,
agents, and harnesses combine to produce software at a predictable cost and
quality. The principles below are factory-floor design choices: they describe
how the line is laid out, where quality is inspected, which inputs are
trusted, and how the work moves from intent to merged code.

Each principle has a headline, a supporting paragraph that explains the
choice and the constraint behind it, and a "DDx response" that names the
mechanism in this codebase that enacts it. The principles are grounded in
the research syntheses RSCH-001 through RSCH-010, which capture the
evidence and prior art behind each choice.

## Principles

### Spec-first development

Specifications, not code, are the durable artifact in a document-driven
factory. Code is a perishable rendering of a spec into a particular
language, framework, and runtime — regenerable, replaceable, and often
shorter-lived than the requirement it serves. When the spec is the source
of truth, AI agents and human contributors share a stable target, reviews
focus on intent rather than syntax, and refactors stop being existential.
The alternative — code-first prompting where requirements live in chat
history and PR descriptions — produces software no one can confidently
change, because the "why" decays the moment the conversation scrolls. A
factory built on specs treats every implementation as a re-pour from the
mold; a factory built on code treats every change as archaeology. DDx
chooses the mold.

**DDx response:** HELIX frame artifacts (vision, PRD, feature specs,
principles) are first-class documents in `docs/helix/`, versioned with the
code, and referenced by every downstream bead via `ddx.depends_on`.

### Executable specifications

A spec that cannot be checked is a wish. Executable specifications close
the gap between intent and verification by attaching machine-readable
acceptance criteria, contract tests, or schema constraints to every
requirement, so that "done" is a function the system can compute rather
than a judgment a reviewer must render. This matters most in an
agent-driven factory, where dozens of attempts may target the same bead
and a human cannot personally adjudicate each one. Executable specs let
the factory grade its own output: tests pass or fail, contracts hold or
break, schemas validate or reject. The cost is up-front rigor — writing
the check before the code — and the payoff is that quality scales with
throughput instead of degrading under it. DDx treats unverifiable
acceptance criteria as a defect in the spec, not a tolerance in review.

**DDx response:** Bead acceptance criteria are written as concrete,
checkable statements (commands, file predicates, test names); the
post-merge review step grades each AC item against the working tree.

### Audit trail required

Every artifact the factory produces must be traceable to the inputs that
caused it: which spec, which bead, which agent, which model, which
prompt, which commit. Without that chain, debugging becomes guesswork and
compliance becomes impossible. An audit trail is also the substrate for
learning — you cannot improve a process whose decisions are unrecorded.
In an agent-heavy pipeline this is doubly true, because the agents
themselves are non-deterministic and their behavior drifts as models
change. The trail must capture not only what was decided but what was
considered and rejected, so that a future maintainer (human or agent) can
reconstruct the reasoning rather than re-derive it. DDx treats the audit
trail as load-bearing infrastructure, not as logging — it is part of the
product, not an operational courtesy.

**DDx response:** `.ddx/executions/` preserves per-attempt evidence
(prompts, diffs, review findings); bead history records every state
transition; commits reference the bead id; agent runs log model, harness,
and cost.

### Context is king

An agent's output is bounded by the context it receives. The same model,
given a sharper brief, produces dramatically better work — and the same
model, given a bloated or stale brief, produces confidently wrong work.
In a document-driven factory, context engineering is not a tuning detail;
it is the primary lever on quality and cost. That means investing in the
mechanisms that select, shape, and bound what each agent sees: the spec
excerpts, the relevant code, the prior attempts, the review findings,
the persona. It also means resisting the temptation to dump everything
into the prompt "just in case," because irrelevant context degrades
performance and inflates cost. The factory's job is to assemble the
right brief for the right step, every time, and to make that assembly
visible and tunable.

**DDx response:** Personas, harness configs, governing-artifact
resolution, and review-finding threading shape the brief; bead
descriptions are the contract; `.ddx/executions/` captures what was
actually sent.

### Work is a DAG

Work is not a list. Tasks have prerequisites, blockers, and downstream
consequences, and pretending otherwise produces either false serial
ordering (slow) or chaotic parallelism (broken). A directed acyclic
graph is the honest data structure: each unit of work names what must
exist before it starts and what becomes possible once it finishes. In a
factory where multiple agents run in parallel, the DAG is also the
scheduler — the queue dispatches what is ready and holds what is
blocked, without anyone having to remember the order. Modeling work as
a DAG forces dependencies into the open, where they can be inspected,
broken, or parallelized; modeling work as a list hides them, where they
re-emerge as merge conflicts and rework. DDx commits to the graph.

**DDx response:** Beads have explicit `depends_on` edges; the queue
surfaces ready/blocked partitions; `ddx bead dep tree` renders the DAG;
execution respects topological order.

### Right-size the model

Models are not interchangeable, and using the strongest model for every
step is the most expensive way to be mediocre. Cheap models do well on
narrow, well-specified tasks; strong models earn their keep on review,
synthesis, and ambiguous judgment; deterministic checks (linters, type
systems, tests) outperform every model on the things they cover. A
well-run factory tiers the work: the cheapest tool that can do the job,
escalating only when it cannot. This is not just cost optimization — it
is also a quality strategy, because deterministic checks catch what
models miss, and strong models catch what cheap models miss. The
ordering matters: deterministic at the top of the ladder, then review
agents, then implementation agents. DDx routes by capability and budget,
not by brand loyalty.

**DDx response:** Harness configs declare capability tiers; execute-loop
escalates on review failure; deterministic checks (tests, lints) gate
merge before any model gets a vote.

### Avoid vendor lock-in

Model vendors change pricing, deprecate models, and rewrite APIs on
their own schedule. A factory that bakes a single vendor's name into
its routing logic, prompts, or contracts hands that vendor a veto over
the factory's roadmap. The defense is abstraction at the right seam:
endpoints, capabilities, and protocols rather than provider names; live
discovery rather than hardcoded model lists; portable prompt formats
rather than vendor-specific control tokens. The cost is a layer of
indirection and the discipline to resist convenient shortcuts. The
payoff is the freedom to swap, mix, and tier providers as the market
moves — and to run local models alongside hosted ones without rewriting
the harness. DDx treats portability as a non-negotiable, even when a
single-vendor path would be cheaper to ship this quarter.

**DDx response:** Endpoint-first routing (no named-provider profiles),
fuzzy model match against live discovery, harness abstraction over
provider SDKs, MCP for tool surfaces.

### Drift is debt

Specifications, code, configs, and documentation drift apart whenever
one is changed without the others. Each instance of drift is a small
debt: the next reader has to reconcile the conflict, the next agent has
to guess which is authoritative, the next change has to pay interest on
both. Left untreated, drift compounds until the documents are decorative
and the truth lives only in the code — at which point spec-first
development has quietly collapsed into code-first development. The
factory's response is not heroic vigilance; it is mechanical detection.
Drift checks belong in CI, in review, and in the alignment step, so
that divergence shows up as a failing build rather than a future
surprise. DDx invests in alignment tooling because the alternative is
slow rot, and slow rot is the failure mode that kills document-driven
projects.

**DDx response:** `helix align` reconciles spec and implementation;
ratchets and lints detect drift in CI; the review step flags AC items
the implementation no longer satisfies.

### Least privilege for agents

Agents are powerful, fallible, and fast — a combination that punishes
permissive defaults. An agent with shell access, network access, and
write access to the entire repo can do enormous damage in seconds, and
the damage may not be obvious until much later. Least privilege treats
agent permissions the way a security-conscious system treats user
permissions: grant the minimum needed for the task, scope it to the
artifact at hand, and audit what was actually used. In practice that
means isolated worktrees, explicit allowlists for shell and network,
bounded tool surfaces, and merge gates that re-validate the diff a
human would re-validate. The goal is not to make agents weak; it is to
make their blast radius proportional to the trust we have actually
earned with them.

**DDx response:** execute-bead runs in an isolated worktree; harnesses
declare permitted tools; settings.json allowlists gate Bash and MCP
calls; merge to base requires review.

### Inspect and adapt

A factory that does not measure itself cannot improve, and one that
measures the wrong things will optimize itself into a corner. Inspect
and adapt is the discipline of closing the loop: define what good
looks like, measure it on real runs, surface the result, and let the
measurement drive concrete changes to specs, harnesses, prompts, and
process. In an agent-heavy pipeline the metrics that matter are
throughput (closed beads per unit time), cost (dollars per closed bead),
quality (review-pass rate, regression rate), and cycle time (queue to
merge). The factory must expose these, not bury them, and must treat
unfavorable trends as signals rather than noise. DDx commits to running
on its own queue, paying its own bills, and adjusting when the numbers
say to.

**DDx response:** Execution evidence aggregates into per-bead and
per-harness metrics; routine retros against the queue inform spec,
harness, and routing changes; the project dogfoods its own pipeline.

## Tension Resolution

When principles conflict, apply the one whose violation would cause the
worse outcome in context. Document the tension in the commit message or
design note.

Common tensions:

- **Right-size the model vs. Avoid vendor lock-in**: the cheapest model
  for a task is sometimes the one with the most proprietary surface.
  Prefer portability unless the cost gap is structural.
- **Context is king vs. Least privilege for agents**: richer context
  often means broader read access. Scope reads to the bead's governing
  artifacts; do not hand agents the whole repo by default.
- **Spec-first development vs. Inspect and adapt**: measurements
  sometimes contradict the spec. Treat that as a signal to evolve the
  spec, not to silently diverge from it.
