# Product Vision

## Mission Statement

HELIX helps teams and agents turn product intent into shipped software through
a supervised autopilot that continuously advances the weakest ready layer of
the development stack until human input is actually needed.

HELIX combines collaborative planning with bounded autonomous execution so
human attention stays focused on judgment, tradeoffs, and approvals rather
than routine orchestration.

## Core Metaphor

Named after the double helix of DNA, HELIX encodes the idea that **planning
and execution are two complementary strands** that happen simultaneously and
feed back into each other. Neither is primary. Neither is sequential. They
intertwine.

- **Planning helix**: review → plan → validate (creates and refines beads)
- **Execution helix**: execute → measure → report (claims beads, does work,
  records results, creates follow-on beads)

## Positioning

For software teams using AI agents for day-to-day development who lose too
much operator effort deciding what the agent should do next and keeping
planning artifacts aligned with implementation, HELIX is a supervisory
autopilot that continuously advances work across specs, designs, tests,
implementation, review, and metrics until human judgment is actually needed.
Unlike ad hoc prompting with manual agent steering, HELIX maintains a durable
control loop where requirements govern code, the tracker steers agents, and
every action traces back to governing intent.

### What HELIX Is Not

- HELIX is **not a platform**. It runs on DDX. It depends on DDX for artifact
  management, bead tracking, agent execution, and review infrastructure.
- HELIX is **not prescriptive about tools**. Technology choices are expressed
  as cross-cutting concerns that vary per project.
- HELIX is **not waterfall**. The artifact hierarchy (vision → PRD → specs)
  is a set of abstractions at different zoom levels, not a sequential pipeline.

## Vision

HELIX becomes the default control system for spec-driven agent software
delivery: a workflow where requirements, design, test, implementation, review,
and metric refinement advance with minimal human intervention, and where
multi-agent and single-agent teams alike adopt HELIX as the safe default for
carrying software from intent to production.

**North Star**: A team should be able to express intent once, collaborate where judgment matters, and let HELIX carry the rest of the workflow forward safely.

## The Methodology

### Rejection of False Dichotomies

- **Waterfall** made the mistake of assuming you could plan completely, then
  execute.
- **Agile** (depending on implementation) makes the mistake of assuming you
  can execute without planning.
- **Vibe coding / ad-hoc agentic development** makes the mistake of assuming
  agents can infer intent from code alone.

HELIX says: **plan and execute simultaneously, at multiple levels of
abstraction, with documentation as the shared context layer.**

### Progressive Abstraction Layers

Each layer is a lens at a different zoom level. Changes can enter at any layer
and propagate up, down, or sideways.

| Layer | Artifact | Purpose |
|-------|----------|---------|
| Discovery | Research, competitive analysis | Understand the problem space |
| Vision | Product Vision | What is the thing? Why does it exist? |
| Requirements | PRD | High-level features and constraints |
| Specification | Feature Specs + Acceptance Criteria | What each feature does, how we know it works |
| Design | Technical Designs, Solution Designs, ADRs | How to build it, what trade-offs we make |
| Test | Test suites (from acceptance criteria) | Executable assertions proving requirements are met |
| Code | Implementation | The software itself |

### Multi-Directional Workflow

HELIX is **not top-down**. Users will:

- Start with a vision and let the agent run the full pipeline.
- See the output and feed corrections back at the implementation level (bottom-up).
- Realize a fundamental direction is wrong and update the vision (top-down).
- Refine a feature spec because implementation revealed a missing requirement
  (middle-out).
- Interact with a running system and provide unstructured feedback that HELIX
  routes to the appropriate abstraction layer.

All of these flows must be natural and low-friction.

### Artifact Graph

HELIX artifacts form a directed graph of relationships:

- Vision → PRD (PRD implements the vision)
- PRD → Feature Specs (specs decompose PRD features)
- Feature Specs → Acceptance Criteria (criteria define "done")
- Feature Specs → Technical Designs (designs explain how to build)
- ADRs ← Cross-cutting concerns (ADRs justify technical concerns)
- Cross-cutting concerns → all execution beads (folded in via context digests)

This graph enables impact analysis (what does a change affect?),
reconciliation (are dependent artifacts still consistent?), and context
synthesis (a bead can pull in its full governance chain).

## User Experience

A developer opens their terminal with a backlog of work tracked in the repo.
They run `helix run`. HELIX picks the highest-leverage ready issue, dispatches
an agent to implement it, verifies the result against acceptance criteria, and
commits. When it finishes, it picks the next one. The developer checks in
periodically — reviewing diffs, approving gates, making product calls when
HELIX surfaces a genuine ambiguity. When they want to work on something
directly, they do — editing a spec, writing a test, closing an issue — and
HELIX picks up the rest. The mental model is an autopilot you can always grab
the wheel from.

## Target Market

| Attribute | Description |
|-----------|-------------|
| Who | Small to medium software teams using AI agents for day-to-day development |
| Pain | Too much operator effort deciding what the agent should do next and keeping planning artifacts aligned with implementation |
| Current Solution | Informal prompting, scattered TODOs, weak issue hygiene, and manual agent steering |
| Why They Switch | As codebases grow, ad hoc prompting breaks down — specs drift from code, agents loop without progress, and the human becomes a full-time dispatcher instead of a decision-maker |

### Adversarial Review as Core Practice

HELIX encourages adversarial review as a default practice, not just a
configurable feature:

- After an agent completes a bead, a second agent (or the same agent with a
  review prompt) examines the work against the artifact hierarchy.
- Reviews check: Does the implementation match the spec? Does the spec still
  align with the PRD? Are cross-cutting concerns respected?
- Reviews can trigger research for best practices or alternative approaches.
- Review findings become new beads in the tracker.

### Interface with Working Systems

A key workflow is: agent builds something → human interacts with the running
system → human provides feedback → HELIX routes feedback to the appropriate
abstraction layer. This means HELIX must support:

- Accepting unstructured feedback ("I don't like how this looks").
- Determining which artifact layer the feedback applies to.
- Generating the appropriate bead to address it.

## Key Value Propositions

| Value Proposition | Customer Benefit |
|-------------------|------------------|
| Supervisory autopilot | HELIX keeps work moving across specs, designs, tests, implementation, review, and metrics until human judgment is actually needed |
| Least-power execution | HELIX chooses the smallest sufficient next action instead of overreaching, reducing unnecessary churn and speculative changes |
| Authority-ordered reconciliation | When artifacts disagree, HELIX resolves the conflict by escalating to the governing source instead of guessing from code alone |
| Tracker as steering wheel | The tracker is the primary human interface for steering agents. Users create issues, set priorities, approve gates, and reject work through tracker state; agents read it and execute |
| Cross-model quality | Critical artifacts are reviewed by alternating AI models, catching blind spots that self-review misses |
| Interactive intervention points | Users can step into any layer of the workflow directly without losing the benefits of autopilot orchestration |

## Product Principles

1. **Autopilot by default**
   `helix run` is HELIX's supervisory autopilot. It continuously selects and
   executes the highest-leverage next bounded action that does not require
   human input.

2. **Human intervention by exception**
   HELIX should escalate only when ambiguity, missing authority, tradeoffs, or
   product judgment block safe forward progress. When it stops, it should tell
   the user exactly what decision is needed and why.

3. **Least powerful next action**
   HELIX should restore progress with the smallest sufficient action: refine a
   spec before redesigning a system, sharpen issues before implementing, and
   reconcile artifacts before inventing new ones.

4. **Authority before implementation**
   Requirements, designs, tests, and plans govern code. Implementation is
   evidence of current behavior, not the source of truth for what should exist.

5. **Tracker as the steering wheel**
   The tracker is the primary interface between humans and agents. Users steer
   by creating issues, setting priorities, approving phase gates, and rejecting
   work — all through tracker state. Agents read tracker state and execute.
   The mental model is: `User <-> Tracker <-> helix run (background)`.

6. **Do hard things**
   Agents should attack problems, not defer them. If the toolchain doesn't
   compile, try to fix it. If a spec is ambiguous, make the best-effort
   interpretation and document the reasoning. Only genuinely contradictory
   governing artifacts or intractable technical problems after escalating
   effort should result in stopping.

7. **Cross-model verification**
   Critical artifacts and implementations should be reviewed by a different AI
   model than the one that produced them. Different models have different blind
   spots; alternating reviewers catches errors that self-review misses.

8. **Interactive entry at any layer**
   The user should be able to work directly on vision, PRD, specs, tests,
   issues, implementation, review, or metrics while still benefiting from
   HELIX's overall control system.

9. **Continuous useful work**
   The system should always be making forward progress. When one issue is
   blocked, move to the next. When an epic's children are done, review the
   epic before moving on. Absorb small adjacent work instead of creating
   ticket churn. The goal is net progress, not activity.

## Success Definition

| Metric | Target |
|--------|--------|
| Autonomous forward progress | From an established vision and PRD, `helix run` advances the repo through downstream refinement and bounded execution until input is required |
| Reduced orchestration overhead | Users spend materially less time telling the agent what phase to enter next |
| Artifact alignment | Specs, issues, tests, implementation, and follow-on work remain traceable and mutually consistent after iterative changes |
| Safe escalation | HELIX asks for user input primarily at real judgment boundaries, not because the workflow contract is underspecified |

## Why Now

AI coding tools are already useful, but teams still lack a reliable supervisory
layer that keeps complex software work coherent over time. The repo already
contains the workflow method, tracker, CLI, and skill surfaces needed to make
the control loop explicit — the gap is connecting them into a single autopilot
that humans can trust and steer.

## Known Risks

1. **Testing and validation is unsolved industry-wide.** HELIX's approach
   (progressive abstraction + TDD + hypothesis optimization) is the best
   available strategy, but it is not proven at scale. HELIX should track
   developments in this space and adapt.

2. **Documentation staleness.** The classic failure mode of document-driven
   development. HELIX mitigates with reconciliation beads and adversarial
   review, but this mitigation is only as good as the prompts and the
   discipline of running reviews.

3. **Agent boundary violations.** Agents don't naturally respect module
   boundaries or abstraction layers. HELIX's approach of folding cross-cutting
   concerns into beads helps, but semantic validation of agent work remains an
   open research question.

4. **DDX/HELIX boundary.** Some capabilities (feedback loops, measurement,
   optimization) could live in either DDX or HELIX. If the boundary isn't
   resolved cleanly, users will be confused about what's platform vs.
   methodology.

5. **Transferability.** HELIX must be teachable without its creator present.
   The test of maturity is whether a new team member can pick it up and be
   productive without the creator's guidance.

6. **Rate of change.** Agentic development best practices evolve weekly. HELIX
   must be flexible enough to incorporate new ideas without breaking changes to
   artifact templates or phase definitions.

## Explicit Non-Goals

- HELIX is not a generic "do everything" agent that replaces product judgment.
- HELIX is not an unbounded autonomous coding loop.
- HELIX is not a chat-only prompt library disconnected from durable tracker
  state.
- HELIX should not force users to remain in autopilot mode when they want to
  intervene directly in a specific workflow layer.
