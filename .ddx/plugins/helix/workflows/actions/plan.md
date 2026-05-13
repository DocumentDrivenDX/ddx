# HELIX Action: Plan

You are creating a comprehensive design plan for a HELIX project scope.

Your goal is to produce a thorough markdown design document through iterative
self-critique and refinement, capturing architecture, interfaces, error
handling, security, testing strategy, and implementation ordering — all before
any code is written or issues are created.

This action is intentionally front-loaded. Invest deeply in planning quality
now to prevent expensive rework during implementation.

## Action Input

You may receive:

- no argument (default: repo-wide plan)
- a scope such as `auth`, `FEAT-003`, `payments`
- `--rounds N` controlling refinement iterations (default: 5)

Examples:

- `helix design`
- `helix design auth`
- `helix design --rounds 8 FEAT-003`

## Authority Order

When artifacts disagree, use this order:

1. Product Vision
2. Product Requirements
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution Designs / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

The plan you produce sits at levels 4-7 and must be consistent with levels 1-3
when they exist.

## PHASE 0 - Context Load

0. **Context Recovery**: Re-read AGENTS.md so project instructions are fresh
   in your working memory. After long sessions, context compaction may have
   dropped critical project rules. This step is cheap insurance against drift.
0a. **Load active design principles** following `.ddx/plugins/helix/workflows/references/principles-resolution.md`.
   These principles guide architectural and design judgment throughout the plan.
0b. **Load active concerns and practices** following `.ddx/plugins/helix/workflows/references/concern-resolution.md`.
   The declared concerns constrain architecture decisions — design within the declared
   technology selections. Reference ADRs that justify concern choices when the
   design depends on them.
1. Read all existing planning artifacts for the scope:
   - product vision, PRD, feature specs, user stories
   - architecture docs, ADRs, technical designs
   - existing test plans and implementation plans
2. Read the current implementation state relevant to the scope.
3. Read the current issue queue state if the tracker is initialized.
4. Identify gaps: what questions does the existing planning stack leave open?
5. **Load design artifact numbering rules** (required before creating any SD or
   TD artifact):
   - Read `.ddx/plugins/helix/workflows/phases/02-design/artifacts/solution-design/meta.yml` to
     understand the SD-{number} format, naming pattern, and no-reuse policy.
   - Read `.ddx/plugins/helix/workflows/phases/02-design/artifacts/technical-design/meta.yml` to
     understand the TD-{number} format.
   - Scan `docs/helix/02-design/solution-designs/SD-*.md` to find the maximum
     existing SD number; **next SD ID = max + 1** (use `001` if none exist).
   - Scan `docs/helix/02-design/technical-designs/TD-*.md` to find the maximum
     existing TD number; **next TD ID = max + 1** (use `001` if none exist).
   - Record both values. Use them exclusively when assigning IDs to new SD or
     TD artifacts in this session; increment by one for each additional artifact
     created. Never guess or reuse an existing number.
   - Before writing any SD or TD artifact, validate each `depends_on` entry in
     its ddx frontmatter: every referenced ID (e.g., `FEAT-XXX`) must resolve
     to an existing artifact on disk. If a target does not exist, stop and
     request guidance before writing the file.

## PHASE 0.5 - Bead Acquisition

Before writing any design content, acquire a governing bead for this design
pass. See `.ddx/plugins/helix/workflows/references/bead-first.md` for the full pattern.

1. Search for an existing open bead governing this work:
   - `ddx bead list --status open --label kind:planning,action:design --json`
   - Filter by scope or `spec-id` if the action was dispatched with a scope.
2. If found, verify it is still relevant and claim it:
   - `ddx bead update <id> --claim`
3. If not found, create one:
   ```bash
   ddx bead create "design: <scope description>" \
     --type task \
     --labels helix,phase:design,kind:planning,action:design \
     --set spec-id=<governing-artifact-if-known> \
     --description "<context-digest>...</context-digest>
   Create comprehensive design document for <scope>.
   Inputs: <list governing artifacts loaded in Phase 0>" \
     --acceptance "Design document converged with all required sections including concern-mandated sections; written to canonical path"
   ```
4. Record the bead ID. All subsequent file modifications are governed by this
   bead.

## PHASE 1 - First Draft

Produce a comprehensive design document covering ALL of the following sections.
Do not skip sections; mark them "N/A" with rationale if genuinely inapplicable.

1. **Problem Statement and User Impact**
   - What problem does this solve? Who benefits? What happens if we don't solve it?
2. **Requirements Analysis**
   - Functional requirements (what the system must do)
   - Non-functional requirements (performance, scalability, reliability)
   - Constraints (technology, timeline, compliance)
3. **Architecture Decisions**
   - For each decision: state the question, list alternatives considered,
     explain the chosen approach and why alternatives were rejected
4. **Interface Contracts**
   - APIs, CLIs, configuration surfaces, file formats
   - Input validation rules and error response formats
5. **Data Model**
   - Entities, relationships, storage, migration strategy
6. **Error Handling Strategy**
   - Error categories, retry policies, fallback behavior, user-facing messages
7. **Security Considerations**
   - Attack surfaces, authentication, authorization, data protection
   - Threat model for new surfaces introduced
8. **Test Strategy**
   - What to test at each level (unit, integration, e2e)
   - Critical paths that must have coverage before shipping
9. **Implementation Plan with Dependency Ordering**
   - Work breakdown into issue-sized slices
   - Dependency graph showing what must be built first
   - Parallel tracks that can proceed independently
10. **Risk Register**
    - Known risks, likelihood, impact, mitigation strategy
11. **Observability**
    - Logging, metrics, alerting, dashboards

### Concern-Mandated Sections

If active concerns require specific design coverage, those sections are
mandatory (not just "recommended"):

- `security-owasp` active → Section 7 (Security Considerations) must include
  OWASP-aligned threat model, not just a placeholder.
- `o11y-otel` active → Section 11 (Observability) must include OpenTelemetry
  instrumentation plan with specific spans, metrics, and trace propagation.
- `a11y-wcag-aa` active → Add a **12. Accessibility** section covering WCAG AA
  compliance strategy for all user-facing surfaces.
- Other active concerns → Check each concern's `practices.md` for design-phase
  requirements and ensure the plan addresses them.

The governing bead's acceptance criteria include concern-mandated section
completeness.

## PHASE 2 through N - Iterative Refinement

For each subsequent round:

1. Re-read AGENTS.md to refresh project context.
2. **Assume there are 80+ elements you have missed or underspecified.** This is
   not a suggestion — actively search for gaps in every section.
3. Challenge every assumption, interface, error path, and edge case.
4. For each section, ask:
   - What happens when this fails?
   - What happens at 10x scale?
   - What happens when the input is malformed?
   - What happens when a dependency is unavailable?
   - What would a security reviewer flag?
   - What would an oncall engineer need at 3am?
5. Add missing:
   - Error handling paths and recovery procedures
   - Concurrency considerations and race conditions
   - Migration strategies and backward compatibility
   - Rollback procedures
   - Monitoring and observability hooks
   - Performance constraints and benchmarks
   - Security attack surfaces
6. Track changes between rounds in a **Refinement Delta** section at the end:
   - Round number
   - Count of substantive changes
   - Summary of what changed and why

## Convergence Detection

Track a refinement velocity metric: count of substantive changes per round.
A substantive change is one that affects behavior, interfaces, error handling,
security, or architecture — not formatting or wording.

When velocity drops below 5 substantive changes for two consecutive rounds,
declare convergence and stop refinement.

## PHASE N+1 - Finalize

1. Remove the Refinement Delta section (it served its purpose during iteration).
2. Write the final plan to:
   `docs/helix/02-design/plan-YYYY-MM-DD[-scope].md`
   where YYYY-MM-DD is today's date and scope is the input scope (omit if repo-wide).
3. Ensure the plan is self-contained: a reader should understand the full design
   without reading other documents, though it should cross-reference governing
   artifacts by path.

## PHASE N+2 - Measure

Verify the design document against the governing bead's acceptance criteria.
See `.ddx/plugins/helix/workflows/references/measure.md` for the full pattern.

1. **Acceptance criteria**: Verify the design document exists at the canonical
   path and contains all required sections (including concern-mandated ones).
2. **Convergence**: Confirm refinement velocity dropped below threshold or
   explain why it did not.
3. **Concern coverage**: Verify each active concern's design-phase requirements
   are addressed in the plan.
4. **Record results** on the governing bead:
   `ddx bead update <id> --notes "<measure-results>...</measure-results>"`

## PHASE N+3 - Report

Close the design cycle and feed back into the planning helix.
See `.ddx/plugins/helix/workflows/references/report.md` for the full pattern.

1. If measurement passed, close the governing bead with evidence summary.
2. If measurement identified gaps, create follow-on beads for:
   - Missing concern-mandated sections
   - Sections that need further refinement
   - Guidance-dependent items
3. Note: The design document itself is not an execution bead — it must go
   through `helix polish` to be decomposed into implementable beads before
   `helix build` can execute against it.

## Output

Report these trailer lines at the end of your output:

```
PLAN_STATUS: CONVERGED|IN_PROGRESS|GUIDANCE_NEEDED
PLAN_DOCUMENT: docs/helix/02-design/plan-YYYY-MM-DD[-scope].md
PLAN_ROUNDS: N
MEASURE_STATUS: PASS|FAIL|PARTIAL
BEAD_ID: <governing-bead-id>
FOLLOW_ON_CREATED: N
```

- `CONVERGED`: refinement velocity dropped below threshold
- `IN_PROGRESS`: max rounds reached but velocity still high
- `GUIDANCE_NEEDED`: ambiguity that requires user input before the plan can converge
