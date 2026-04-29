---
title: Phases
weight: 1
prev: /docs/glossary
next: /docs/glossary/artifacts
---

# HELIX Phases

HELIX structures software delivery into six sequential phases. Each phase has input gates that validate the previous phase's outputs before allowing progression.

## Phase 0: Discover

**Optional.** Validate the opportunity before committing to Frame.

| | |
|---|---|
| **Purpose** | Research the market, users, and technical feasibility |
| **Location** | `docs/helix/00-discover/` |
| **Key artifacts** | Product Vision, Business Case, Competitive Analysis, Opportunity Canvas |
| **Output** | A validated opportunity worth pursuing |

## Phase 1: Frame

Define the problem, establish requirements, and scope the work.

| | |
|---|---|
| **Purpose** | Turn a validated opportunity into actionable requirements |
| **Location** | `docs/helix/01-frame/` |
| **Key artifacts** | PRD, Feature Specs, User Stories, Principles, Concerns |
| **Command** | `helix frame` |
| **Output** | Requirements clear enough to design against |

Frame also selects the project's active [concerns](/docs/glossary/concerns) and establishes [design principles](/docs/glossary/concepts#principles).

## Phase 2: Design

Architect the solution approach.

| | |
|---|---|
| **Purpose** | Make technology and architecture decisions before writing code |
| **Location** | `docs/helix/02-design/` |
| **Key artifacts** | Architecture, ADRs, Solution Designs (SD-NNN), Technical Designs (TD-NNN) |
| **Command** | `helix design [scope]` |
| **Output** | Design contracts that tests can verify |

Solution designs are feature-level. Technical designs are story-level slices that inherit from a solution design.

## Phase 3: Test

Write failing tests that define system behavior. This is the **Red** phase of TDD.

| | |
|---|---|
| **Purpose** | Turn requirements and designs into executable specifications |
| **Location** | `docs/helix/03-test/`, `tests/` |
| **Key artifacts** | Test Plans, Test Procedures, Test Suites, Security Tests |
| **Output** | Failing tests that define what "done" means |

Tests are the contract between design and implementation. Code must satisfy tests, not the other way around.

## Phase 4: Build

Implement code to make tests pass. This is the **Green** phase of TDD.

| | |
|---|---|
| **Purpose** | Write the minimum code needed to satisfy the tests |
| **Location** | `docs/helix/04-build/`, source code |
| **Key artifacts** | Implementation Plan |
| **Command** | `helix build [issue-id]` |
| **Output** | Passing tests, committed code, closed tracker issues |

Build cannot start until tests are written and failing. Implementation is complete when all tests pass.

## Phase 5: Deploy

Release to production with monitoring.

| | |
|---|---|
| **Purpose** | Safely deliver the built software to users |
| **Location** | `docs/helix/05-deploy/` |
| **Key artifacts** | Project-specific checklists and runbooks as needed |
| **Output** | Running production system with observability |

Deploy cannot start until all tests pass (Green phase complete).

## Phase 6: Iterate

Learn and improve for the next cycle.

| | |
|---|---|
| **Purpose** | Review what was built, capture learnings, plan the next cycle |
| **Location** | `docs/helix/06-iterate/` |
| **Key artifacts** | Alignment Reviews, Backfill Reports, Metric Definitions |
| **Commands** | `helix align`, `helix review`, `helix experiment` |
| **Output** | Actionable follow-up work feeding back into Frame |

Iterate closes the loop. Alignment reviews and experiments produce tracker issues that drive the next cycle.
