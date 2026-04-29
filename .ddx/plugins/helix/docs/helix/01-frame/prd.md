---
dun:
  id: helix.prd
---
# Product Requirements Document

## Summary

HELIX is a development control system for AI-assisted software delivery. It
combines collaborative planning with a supervisory autopilot that keeps work
moving across requirements, specifications, designs, tests, implementation,
review, and metrics until human judgment is actually needed.

The primary product experience is not a bag of commands. It is a loop in which
the user can work interactively at any layer, while `helix-run` autonomously
detects downstream implications, selects the least-powerful sufficient next
action, and advances the workflow without requiring repeated manual
orchestration.

## Problem and Goals

### Problem

AI-assisted development tools are good at producing local output, but weak at
maintaining coherent progress across a full software delivery stack. Users must
still tell the system what phase to enter next, remember when requirements
changes imply design updates, decide when stale issues need refinement, and
manually reconnect implementation work to governing artifacts.

That creates three failures:

1. Too much operator effort goes into orchestration instead of judgment.
2. Requirements, designs, tests, issues, and code drift out of alignment after
   iterative changes.
3. The agent behaves too literally unless the user remembers the HELIX method
   and explicitly restates it.

### Goals

1. Make `helix-run` the default supervisory autopilot for HELIX-managed work.
2. Ensure HELIX automatically advances the highest-leverage ready layer that
   does not require human input.
3. Make direct commands and skills available as intervention points inside the
   same loop, not as disconnected parallel interfaces.
4. Preserve bounded execution, authority order, and tracker-first discipline
   while making the system feel fluid in interactive use.

### Success Metrics

| Metric | Target | Measurement Method | Timeline |
|--------|--------|-------------------|----------|
| Autopilot progression | `helix-run` can advance from existing vision/requirements through downstream refinement and bounded execution without explicit phase instructions | End-to-end scenario tests and workflow walkthroughs | Year 1 |
| Reduced orchestration burden | Users rarely need to tell HELIX which phase to enter next when authority is sufficient | Qualitative operator evaluation and session review | Year 1 |
| Trigger correctness | Requirement changes, spec changes, and ready implementation work route to the expected HELIX actions | Deterministic tests and spec-driven examples | Year 1 |
| Safe escalation | HELIX asks for input mainly at real judgment boundaries, not because the control contract is underspecified | Review of intervention points in representative sessions | Year 2 |

### Non-Goals

- Create an unbounded autonomous coding agent.
- Replace product, design, or prioritization judgment with guessed decisions.
- Require autopilot mode for all users or all tasks.
- Flatten all HELIX phases into one generic prompt.

Deferred items tracked in `docs/helix/parking-lot.md`.

## Users and Scope

### Primary Persona: HELIX Operator
**Role**: Developer, tech lead, or staff engineer using AI as a day-to-day development partner  
**Goals**: Express product intent clearly, intervene where judgment matters, and let the system carry forward the rest of the workflow  
**Pain Points**: Too much manual orchestration, repeated re-explaining of workflow expectations, artifact drift after changes, and inconsistent issue quality

### Secondary Persona: HELIX Maintainer
**Role**: Maintainer evolving the HELIX workflow, CLI, skills, and tracker  
**Goals**: Keep the workflow contract coherent across docs, skills, and implementation  
**Pain Points**: The product intent is easy to dilute when surface docs or skill descriptions become too command-literal

### Tertiary Persona: New Team Member
**Role**: Developer or tech lead adopting HELIX for the first time without the creator present  
**Goals**: Understand the artifact hierarchy, create a first project, and become productive quickly  
**Pain Points**: The methodology is sophisticated and requires guided onboarding; without it, users may misuse HELIX as a prompt library or skip the planning artifacts entirely

## Requirements

### Must Have (P0)

1. `helix-run` acts as HELIX's supervisory autopilot rather than a narrow
   command wrapper.
2. `helix-run` continuously selects the highest-leverage next bounded action
   that can be taken safely without user input.
3. HELIX applies the principle of least power when choosing the next action.
   It should refine or reconcile existing artifacts before escalating to larger
   changes.
4. HELIX detects downstream implications of user-driven changes.
   - A functionality change request triggers specification and design work when
     needed.
   - A specification or design change triggers issue refinement when open work
     already exists.
   - A specification or issue refinement made during a live `helix-run`
     session must be observed at the next safe execution boundary rather than
     ignored until a later manual restart.
   - Ready execution work triggers bounded implementation.
5. HELIX escalates to the user when safe progress depends on missing authority,
   unresolved ambiguity, tradeoffs, prioritization, or approval.
6. Users can directly invoke any layer of the workflow interactively without
   breaking the overall control model.
7. Tracker state remains the durable execution layer for refinement, ordering,
   ownership, and completion of work.
8. `helix-run` must support concurrent local operation where an automated
   session advances execution while an operator or another agent refines specs
   and tracker issues interactively.
9. `helix-run` must revalidate issue state before claim and before close so
   concurrent refinement does not lead to stale execution or false completion.
8. HELIX must be distributed and installed as one skill pack, not as isolated
   standalone skills copied without their shared resources.
9. Resources shared by multiple HELIX skills must live in `workflows/`, while
   skill-specific resources must live with the corresponding skill under
   `skills/<skill>/`.
10. Plugin, enterprise, and local installations must preserve package-relative
    access from HELIX skills to the shared `workflows/` resource library.

### Should Have (P1)

1. HELIX documents explicit trigger rules between `run`, `align`, `design`,
   `polish`, `build`, `review`, `check`, `evolve`, and `backfill`.
2. Skills describe not only what each command does, but when it should
   activate based on user intent and repo state.
3. The workflow contract makes it obvious where human attention is expected and
   where autopilot should proceed silently.
4. Deterministic tests cover the most important state transitions in the
   supervisory loop.
5. Packaging and installer rules make incomplete skill-only installs invalid
   rather than silently degrading shared-resource access.
6. Deterministic tests cover queue drift caused by concurrent interactive
   refinement while `helix-run` is active.
7. `helix status` provides a structured lifecycle snapshot: run-controller
   state, current claimed work, blocked issues with reasons, pending human
   decisions, and next recommended action.
8. Cross-model review: when `--review-agent` is configured, reviews and plan
   critiques alternate between the implementation agent and the review agent
   to catch blind spots.
9. Epic focus mode: when `helix run` picks an epic, it stays focused —
   decompose into children, implement them, review the epic on close — before
   moving to the next scope.
10. First-class principles: HELIX ships a small set of default design
    principles and every judgment-making skill loads the active principles
    (project-specific if they exist, HELIX defaults otherwise) to guide
    design trade-offs, implementation choices, and review criteria
    consistently across phases. See [[FEAT-003]].
11. Plugin packaging: HELIX should be installable as a Claude Code plugin so
    that skills, the CLI, and shared workflow resources are discovered
    automatically via the plugin manifest — no manual symlink installation
    required. See [[FEAT-004]].
12. Concerns, practices, and context digests: HELIX ships a library of
    composable cross-cutting concerns (tech stacks, a11y, o11y, i18n) with associated practices. Beads
    created by triage and evolve must carry a compact context digest
    (~1000-1500 tokens) summarizing active principles, concerns, practices,
    relevant ADRs, and governing spec context — making beads self-contained
    execution units that rarely require upstream file reads. See [[FEAT-006]].
13. Artifact template quality: each artifact template (Vision, PRD, Feature
    Spec, Solution Design, Technical Design, ADR, Test Plan) must include
    clear instructions for what goes in each section, prompts for agents to
    generate drafts from higher-level artifacts, review checklists for
    adversarial review, and relationship declarations so the artifact graph
    can be constructed automatically. See [[FEAT-008]].
14. Team onboarding: HELIX must be teachable without its creator present.
    This includes a getting-started guide that walks through initializing a
    project, understanding the artifact hierarchy, creating a first bead,
    and running an agent; a guided first-project template; and
    self-documenting prompts that explain why each bead exists and what
    context it draws from. See [[FEAT-009]].
15. Testing strategy templates: HELIX should provide structured guidance on
    testing approaches — acceptance test generation from acceptance criteria,
    property-based testing patterns, integration/flow testing, and
    performance benchmarking templates. These are structured approaches and
    prompts, not test frameworks. See [[FEAT-010]].
16. Hypothesis-driven optimization: the `helix experiment` command must
    support metric definition, hypothesis generation, hypothesis testing
    (apply change, re-measure), selection (keep improvements, discard
    regressions), and iteration until target or diminishing returns. Each
    hypothesis is a bead. Failed hypotheses must roll back cleanly.

### Nice to Have (P2)

1. Operator-facing examples show common transitions such as requirements change
   -> evolve -> polish -> build -> review.
2. The tracker supports richer lifecycle metadata: phase labels for all 6
   phases (`phase:frame`, `phase:test`), per-scope phase gates, and structured
   blocker descriptions.
3. Phase-aware `helix run` that drives the full lifecycle (Frame → Design →
   Test → Build → Deploy → Iterate) using explicit per-scope phase state
   rather than inference from labels and deps.
4. `helix frame`, `helix test`, and `helix deploy` as phase-specific commands
   for manual control over phases currently handled implicitly.
5. `helix reject` as a compound operation: structured rejection reason, mark
   execution-ineligible, create corrective follow-on issue.
6. Unstructured feedback routing: when a user provides unstructured feedback
   about a running system ("I don't like how this looks", "the ships should
   be green"), HELIX should determine which artifact layer the feedback
   applies to and generate the appropriate bead to address it.
7. Real-project feedback integration: lessons from validated HELIX projects
   should be captured and folded into improved templates, better prompts,
   new cross-cutting concerns, and updated getting-started materials.

## Phases

| Phase | Description | Key Activities |
|-------|-------------|----------------|
| **Discover** | Explore the problem space | Market analysis, competitive review, stakeholder research |
| **Frame** | Decompose the problem into structured artifacts | Vision, PRD, Feature Specs, Acceptance Criteria |
| **Plan** | Encode requirements as testable assertions | Convert acceptance criteria into executable tests |
| **Build / Iterate** | Red-green-refactor against tests | Implement code to pass tests, respecting cross-cutting concerns |
| **Polish** | Systematic optimization against metrics | Hypothesis-driven improvement, adversarial review, gap analysis |

## Command Surface

The command surface is organized around how users interact with HELIX:

### Execution (how agents work)

- `helix run` — supervisory autopilot; reads tracker, selects the
  highest-leverage next action, executes it, repeats until human input is
  needed or no work remains
- `helix worker` — launch `helix run` as a background process and monitor
  progress via summary output and log files
- `helix status` — structured lifecycle snapshot for human observation

### Steering (how humans direct agents)

The tracker is the primary steering interface. Users typically interact with
the tracker via an agent conversation while `helix run` grinds in the
background. Structured commands ensure artifact consistency:

- `ddx bead` — raw CRUD for issues (create, show, update, close, list,
  ready, blocked, dep, status)
- `helix evolve` — thread a requirement change through the artifact stack;
  updates governing docs and creates/modifies tracker issues
- `helix design` — create or extend the design stack (architecture, ADRs,
  solution designs, test strategy) through iterative multi-model refinement
- `helix review` — fresh-eyes review of recent work
- `helix align` — top-down audit of the full planning stack vs implementation;
  also handles reconstruction of missing docs (backfill mode)
- `helix polish` — iterative issue refinement before implementation
- `helix experiment` — metric-driven optimization loop

### Internal (dispatched by run, also directly invocable)

- `helix build` — one bounded implementation pass (currently: `implement`)
- `helix check` — queue-health decision: what action should run take next

### Deferred (Phase 2 — requires tracker lifecycle primitives)

- `helix frame` — Phase 1: requirements, feature specs, user stories
- `helix test` — Phase 3: write failing tests from design artifacts (Red gate)
- `helix deploy` — Phase 5: release, monitoring, verification
- `helix reject` — compound rejection: structured reason, mark ineligible,
  create corrective follow-on
- Per-scope phase gates on tracker epics/scopes

### Command naming principle

Commands that map to HELIX phases use the phase name: `frame`, `design`,
`test`, `build`, `deploy`. Commands that are control-plane verbs use their
verb: `run`, `status`, `evolve`, `review`, `align`, `polish`, `experiment`.

## Functional Requirements

### Supervisory Run Loop

1. `helix run` must own autonomous forward progress for HELIX-managed work.
2. `helix run` must treat the companion HELIX actions as triggered subroutines
   inside the loop, while still allowing them to be invoked directly.
3. `helix run` must stop when human input is required rather than continuing
   through uncertainty.
4. `helix run` must support epic focus: when an epic is selected, stay on it
   (decompose → implement children → review on close) before moving on.
5. `helix run` must use exponential backoff on difficult issues rather than
   immediately skipping them. Only declare an issue intractable after
   escalating effort across multiple attempts.
6. `helix run` must absorb small adjacent work (same file, related manifest
   updates) into the current issue rather than creating separate tickets for
   every observation.
7. `helix run` must produce a structured blocker report when it finishes,
   identifying every skipped issue with its reason and marking them in the
   tracker for human triage.

### Trigger Rules

1. When the user asks for a functionality change, HELIX must evaluate whether
   existing requirements, feature specs, or designs need reconciliation or
   expansion before implementation work continues.
2. When requirements or design artifacts change and open issues already exist,
   HELIX must refine the issue queue before implementation resumes.
3. When issues are ready and adequately governed by upstream artifacts, HELIX
   must prefer bounded implementation over further speculative planning.
4. When tracker or governing-artifact state changes during a live run, HELIX
   must re-check at the next safe boundary instead of closing or claiming work
   from stale assumptions. Drift on a single issue should skip that issue,
   not stop the entire loop.
5. After implementation, HELIX must review the recent work using a different
   AI model than the one that implemented it when cross-model review is
   configured.
6. After an epic closes, HELIX must run a scoped review against the epic's
   governing spec to verify acceptance coverage and catch gaps.
7. When canonical artifacts are missing or too incomplete for safe progress,
   HELIX must stop for guidance or run a bounded reconstruction path rather
   than guessing.
8. When `helix evolve` invalidates in-flight work, it must use explicit
   tracker primitives (`superseded-by`, `execution-eligible=false`, or
   blocking deps on new design issues) rather than vague "re-evaluation"
   flags.

### Interactive Operation

1. Users must be able to work directly in vision, PRD, specs, designs, issues,
   tests, implementation, review, or metrics.
2. Direct invocation of a single HELIX command or skill must not break the
   broader model that `helix run` uses to resume supervision later.
3. The typical interaction pattern is: user talks to an agent that manipulates
   the tracker and governing artifacts, while `helix run` executes in the
   background on clearly defined work. The tracker is the shared state between
   the human-facing agent and the execution agent.

### Observability

1. `helix run` must persist run-controller state (last check result, stop
   reason, current claimed issue, last action timestamp, cumulative token
   usage) to a structured state file.
2. `helix status` must surface this state file plus tracker health in a
   machine-readable format.
3. Long-running codex sessions must have timeout enforcement and periodic
   heartbeat output.
4. Each cycle must log elapsed time and token usage.
5. Every HELIX skill invocation that produces structured output must create a
   DDx execution run record linking the output to the governing artifact.
   See [[FEAT-005]].
6. `helix status` must be able to query recent execution history to report
   what actions were taken and their outcomes.

### Quality Assurance

1. Tracker validation must enforce required fields (helix label, phase label,
   spec-id for tasks, acceptance criteria for tasks and epics) at issue
   creation time.
2. Design documents and critical artifacts should go through multi-round
   iterative refinement with convergence detection.
3. When cross-model review is configured, alternating AI models critique each
   other's work to catch implementation blindness.

### Packaging and Resource Access

1. HELIX skills must be installed as one package that preserves both
   `skills/` and `workflows/`.
2. `workflows/` must be treated as the shared resource library for assets used
   by multiple HELIX skills.
3. Assets used by only one skill should live in that skill's directory instead
   of `workflows/`.
4. HELIX skills must reference shared resources through stable package-relative
   paths and must fail clearly if the shared library is missing.

## Constraints, Assumptions, Dependencies

### Constraints

- **Technical**: HELIX must preserve bounded actions, authority-ordered
  decision-making, tracker-first execution, and package-relative access from
  skills to shared workflow resources.
- **Product**: The interface must stay comprehensible; new supervisory behavior
  cannot devolve into hidden magical state changes.
- **Testing**: The control loop should be explainable and testable through
  deterministic scenarios, not only anecdotal chat transcripts.

### Assumptions

- Repositories using HELIX have or will produce a usable authority stack:
  vision, requirements, specs, designs, tests, and issues.
- Users want the ability to intervene anywhere in the workflow even when
  autopilot exists.
- The least-power heuristic can be documented tightly enough to avoid arbitrary
  agent behavior.

### Dependencies

- Workflow contract documents under `workflows/`
- **DDx bead tracker** (`ddx bead`) as the canonical work-item backend. HELIX
  configures `bead.id_prefix: hx` and layers HELIX-specific labels/fields via
  validation hooks. HELIX no longer owns storage directly.
- **DDx agent service** (`ddx agent run`) for harness dispatch, output capture,
  token tracking, and session logging. HELIX no longer implements its own
  agent invocation; it calls DDx.
- **DDx execution framework** (`ddx exec`) for durable, artifact-linked
  execution run records. HELIX skill outputs are captured as DDx execution
  runs. See DDx FEAT-010 and [[FEAT-005]].
- HELIX tracker conventions (labels, spec-id, queue semantics) remain
  HELIX-owned, enforced via DDx validation hooks, and operate on DDx beads.
- Skill surfaces under `skills/`
- CLI execution surface in `scripts/helix`
- Plugin packaging via `.claude-plugin/plugin.json` manifest that preserves
  the HELIX package layout (see [[FEAT-004]])
- Legacy installer (`scripts/install-local-skills.sh`) as development
  convenience

## Risks

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Autopilot remains too vague and agents continue behaving literally | High | High | Define trigger rules and escalation boundaries explicitly in the PRD and workflow contract |
| The system overreaches and makes product decisions without authority | Medium | High | Preserve bounded actions, least-power rules, and escalation-on-ambiguity requirements |
| Interactive commands and autopilot drift into separate mental models | Medium | High | Define companion commands as intervention points inside one shared control system |
| Existing docs and skills lag behind the product contract | High | Medium | Follow the PRD with explicit workflow and skill-spec updates plus tracker issues |
| Testing and validation unsolved industry-wide | High | High | Progressive abstraction + TDD + hypothesis optimization; track Meta JIT testing and similar research |
| Documentation staleness (classic doc-driven failure) | Medium | High | Reconciliation beads, adversarial review, `helix align` |
| DDX/HELIX boundary confusion | Medium | Medium | Explicit separation: DDX owns platform (beads, agents, exec); HELIX owns methodology (phases, templates, concerns) |
| Transferability — new users can't adopt without creator | High | High | Team onboarding workflow ([[FEAT-009]]), guided first-project template, self-documenting prompts |
| Rate of change in agentic best practices | High | Medium | Flexible template system, concern library extensibility, no breaking changes to artifact shapes |

## Success Criteria

- `helix-run` is defined as HELIX's supervisory autopilot in the product docs.
- The PRD makes clear when HELIX should continue autonomously and when it must
  stop for user input.
- The PRD defines the required transitions among requirement changes, design
  reconciliation, issue refinement, bounded implementation, and review.
- The PRD makes direct commands and skills subordinate to one coherent control
  model rather than describing them as isolated wrappers.
