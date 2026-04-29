---
title: Artifacts
weight: 2
prev: /docs/glossary/phases
next: /docs/glossary/actions
---

# HELIX Artifacts

Every HELIX project produces a stack of governing documents — artifacts — that define what to build, how to build it, how to test it, and how to ship it. These artifacts form an authority chain: when they disagree, higher-authority artifacts govern lower ones. This page describes each artifact type, organized by the phase that produces it.

Each artifact has a prompt and template in `workflows/phases/` that guides its creation. The prompt explains the artifact's purpose and principles; the template provides the structure to fill in.

## Authority Order

When artifacts disagree, resolve the conflict using this precedence (highest first):

1. Product Vision
2. Product Requirements (PRD)
3. Feature Specs / User Stories
4. Architecture / ADRs
5. Solution Designs / Technical Designs
6. Test Plans / Tests
7. Implementation Plans
8. Source Code / Build Artifacts

Higher-order artifacts govern lower-order artifacts. Tests govern build execution but do not override requirements or design. Source code is evidence of implementation, not the source of truth for requirements.

---

## Discover (Phase 0)

Discover artifacts validate that an opportunity is worth pursuing before committing to a full development cycle. They are optional — skip this phase for projects where the problem and market are already understood.

### Product Vision

`docs/helix/00-discover/product-vision.md`

A north star document that keeps direction, value, and success criteria clear. Every downstream artifact — PRD, specs, designs, tests — traces back to this document. If the vision is vague, everything built on it drifts. The vision defines the mission, target market, positioning, success metrics, and why now.

### Business Case

`docs/helix/00-discover/`

Answers: **Should we invest in this opportunity?** A quantified, decision-oriented analysis of the financial and strategic justification for the project. Keep it short and focused on the go/no-go decision.

### Competitive Analysis

`docs/helix/00-discover/`

Answers: **How do we win in this market?** A factual comparison of alternatives and differentiation. Keep the comparison compact and focused on positioning rather than exhaustive feature matrices.

### Opportunity Canvas

`docs/helix/00-discover/`

Answers: **Is this the right problem to solve?** A structured one-page opportunity assessment that captures the problem, target users, value proposition, and key risks in a format that supports quick decision-making.

---

## Frame (Phase 1)

Frame artifacts define what the system should do, for whom, and how success will be measured. They are the highest-authority documents below the product vision and must be concrete enough that someone could build the product without further conversation.

### Product Requirements Document (PRD)

`docs/helix/01-frame/prd.md`

The authority document for **what to build and why**. The PRD sits between the product vision (which defines direction) and feature specs (which define detailed requirements). Every design decision and implementation choice should trace back to a PRD requirement. It defines the problem, goals, success metrics, personas, prioritized requirements (P0/P1/P2), constraints, and non-goals.

### Feature Specification

`docs/helix/01-frame/features/FEAT-NNN-*.md`

Defines the **scope and requirements** for one major capability. Feature specs sit between the PRD (which defines what the product needs) and user stories (which define vertical slices for implementation). The feature spec owns requirements; user stories own the user journey. Each spec covers functional and non-functional requirements, constraints, edge cases, and success metrics.

### User Story

`docs/helix/01-frame/user-stories/US-NNN-*.md`

User stories are **governing design artifacts**, not throwaway tickets. Each story defines a complete vertical slice of the application that is independently implementable and testable. Stories use Given/When/Then acceptance criteria concrete enough to drive test-first implementation. Tracker issues reference stories; stories don't reference tracker issues. Stories are more stable than the implementation beads that fulfill them.

### Principles

`docs/helix/01-frame/principles.md`

Cross-cutting design and engineering values that guide judgment calls across all HELIX phases. Not workflow rules or process enforcement — these are lenses applied when choosing between two valid options (e.g., "design for simplicity", "tests first", "prefer composition over inheritance"). Two-layer model: HELIX defaults at `workflows/principles.md`, project overrides at `docs/helix/01-frame/principles.md`.

### Concerns

`docs/helix/01-frame/concerns.md`

Declares the project's active cross-cutting concerns — technology stacks, quality attributes, and conventions. Concerns are composable selections from the library at `workflows/concerns/`. Each concern declares area scope and associated practices that flow into context digests on beads. See the [Concerns glossary page](../concerns) for details.

### Risk Register

`docs/helix/01-frame/`

Comprehensive identification, assessment, and management of project risks. Includes both threats and opportunities with likelihood, impact, mitigation strategies, and ownership.

### Threat Model

`docs/helix/01-frame/`

Systematic identification and analysis of security threats using STRIDE methodology. Maps threats to system components, assesses risk, and defines mitigation strategies.

### Security Requirements

`docs/helix/01-frame/`

Comprehensive security and compliance requirements: security user stories, compliance mapping, risk assessment, and testable acceptance criteria for security controls.

### Feasibility Study

`docs/helix/01-frame/`

A systematic analysis of project viability across technical, business, operational, and resource dimensions. Used to validate that a proposed solution is achievable before committing significant resources.

### Stakeholder Map

`docs/helix/01-frame/`

Identification and analysis of all project stakeholders, their influence, interests, and engagement strategies. Includes RACI matrix for clear accountability.

### PR/FAQ

`docs/helix/01-frame/`

Working-backwards press release and FAQ. Forces clarity about the end-user experience by writing the announcement before building the product.

### Research Plan

`docs/helix/01-frame/`

A structured plan for investigating unknown requirements, exploring problem spaces, or validating assumptions before committing to detailed specifications.

### Parking Lot

`docs/helix/01-frame/`

Registry for deferred and future work kept out of the main PRD flow. Parked items remain in their normal HELIX locations and are flagged with `dun.parking_lot: true`. This prevents scope creep while preserving ideas for future cycles.

### Feature Registry

`docs/helix/01-frame/`

Central registry tracking all features, their status, dependencies, and ownership. Single source of truth for feature identification across the project lifecycle.

### Validation Checklist

`docs/helix/01-frame/`

Checklist to validate Frame phase completeness, quality, and readiness for Design. Ensures all deliverables meet standards and stakeholders are aligned before proceeding.

---

## Design (Phase 2)

Design artifacts define **how** the system will be built. They translate requirements into architecture, contracts, and technical decisions that implementation can follow without guesswork.

### Architecture

`docs/helix/02-design/architecture.md`

System-level architecture: component relationships, module boundaries, data flow, and deployment topology. This is the structural blueprint that all solution and technical designs must respect.

### Architecture Decision Record (ADR)

`docs/helix/02-design/adr/ADR-NNN-*.md`

Documents a significant architectural decision: the context that drove it, the alternatives considered, the chosen approach, and the consequences. Each ADR covers exactly one decision. ADRs are immutable once accepted; new decisions that revise them create a new ADR marked as superseding the old one.

### Solution Design

`docs/helix/02-design/solution-designs/SD-NNN-*.md`

Feature-level design that explains the chosen approach for a feature or cross-component capability. Maps requirements to a concrete system shape, evaluates alternatives, and defines the decomposition that story-level technical designs should inherit. Solution designs are the bridge between "what" (feature spec) and "how" (technical design).

### Technical Design

`docs/helix/02-design/technical-designs/TD-NNN-*.md`

Story-level design that details **how** to implement a single user story within the broader solution architecture. Enables vertical slicing by providing implementation details for one bounded piece of work at a time.

### Proof of Concept

`docs/helix/02-design/`

A minimal working implementation that validates the feasibility and approach of a key technical concept before full development. More substantial than a tech spike, focusing on end-to-end validation of technical approaches.

### Tech Spike

`docs/helix/02-design/`

Time-boxed technical investigation to explore unknowns, validate approaches, or reduce technical risk before committing to implementation. Produces concrete technical insights to inform architecture and design decisions. Spikes are evidence-gathering — they feed into ADRs and concern selections.

---

## Test (Phase 3)

Test artifacts define **what to verify** before implementation begins. In HELIX, tests are written before code — they are executable specifications that drive the build phase.

### Test Plan

`docs/helix/03-test/test-plans/TP-*.md`

The project-level test strategy: test levels and scope, framework choices, coverage targets, critical paths, test data strategy, infrastructure requirements, and sequencing. Drives failing tests before implementation and provides traceability from requirements to test execution.

### Test Suites

`tests/`

Executable test code organized by level: unit, integration, E2E, property-based, and performance.

### Security Tests

`docs/helix/03-test/`

Security-specific test procedures and automation: penetration testing plans, vulnerability scanning, and compliance validation.

### Test Procedures

`docs/helix/03-test/`

Step-by-step test execution guides for manual or semi-automated testing scenarios.

---

## Build (Phase 4)

Build artifacts guide the implementation work. In HELIX, code is written to make failing tests pass — build artifacts describe **how** to organize and sequence that work.

### Implementation Plan

`docs/helix/04-build/implementation-plan.md`

Work breakdown, dependency ordering, and parallel tracks. Maps the path from "tests are failing" to "tests are passing" with clear sequencing so agents and developers know what to build in what order.

---

## Deploy (Phase 5)

Deploy artifacts are project-specific checklists and runbooks. Rather than prescribing a fixed set, HELIX recommends creating deploy artifacts as needed for your project's deployment model. Common examples include deployment checklists, runbooks, monitoring setup, and release notes.

---

## Iterate (Phase 6)

Iterate artifacts capture what was learned and feed it back into the next cycle. They close the loop between production reality and the planning stack.

### Alignment Review

`docs/helix/06-iterate/alignment-reviews/AR-*.md`

Top-down reconciliation of plan vs. implementation: gap analysis, acceptance criteria validation, traceability matrix, and execution issues for any drift between governing documents and the codebase.

### Backfill Report

`docs/helix/06-iterate/backfill-reports/BF-*.md`

Documentation reconstruction from evidence: when governing documents are missing or incomplete, a backfill report conservatively reconstructs them from code, tests, and commit history.

### Metric Definition

`docs/helix/06-iterate/metrics/*.yaml`

Individual metric specification: name, unit, direction (higher-is-better or lower-is-better), measurement command, tolerance band, and ratchet floor. See [Quality Ratchets](/docs/workflow/#quality-ratchets) for how metrics become enforced floors.
