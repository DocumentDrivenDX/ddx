---
ddx:
  id: FEAT-008
  depends_on:
    - helix.prd
    - FEAT-006
---
# Feature Specification: FEAT-008 — Artifact Template Quality and Completeness

**Feature ID**: FEAT-008
**Status**: Draft
**Priority**: P1
**Owner**: HELIX maintainers

## Overview

HELIX artifact templates (Vision, PRD, Feature Spec, Solution Design,
Technical Design, ADR, Test Plan) should be self-documenting and
machine-interpretable. Each template must include clear instructions,
agent-oriented generation prompts, review checklists, and relationship
declarations that allow the artifact graph to be constructed automatically.

## Problem Statement

- **Current situation**: Templates exist under
  `.ddx/plugins/helix/workflows/phases/*/artifacts/*/template.md` but vary
  in completeness. Some include generation prompts, others don't. Review
  checklists are absent. Relationship declarations are embedded in meta.yml
  but not surfaced to agents or reviewers.
- **Pain points**:
  1. Agents produce artifacts with missing or weak sections because templates
     don't explain what belongs in each section.
  2. Adversarial review has no structured checklist, so reviewers apply
     inconsistent criteria.
  3. The artifact graph (Vision → PRD → Specs → Designs → Tests) cannot be
     constructed automatically because relationship declarations are
     inconsistent across templates.
  4. New HELIX users don't know what a "good" artifact looks like because
     templates lack examples.

## Functional Requirements

### FR-1: Section Instructions

Each artifact template must include a brief instruction block (HTML comment
or frontmatter annotation) for every section, explaining what belongs there
and what does not.

### FR-2: Generation Prompts

Each artifact template must include or reference a generation prompt that
agents can use to draft the artifact from higher-level governing artifacts.
The prompt must specify which upstream artifacts to read and what context to
synthesize.

### FR-3: Review Checklists

Each artifact template must include a review checklist that adversarial
review agents use to evaluate the artifact. The checklist must be specific
to the artifact type (e.g., "Does the PRD define success metrics?" rather
than generic "Is this complete?").

### FR-4: Relationship Declarations

Each artifact template must declare its upstream dependencies and downstream
artifacts in a machine-readable format (frontmatter `depends_on` and
`enables` fields), so the artifact graph can be traversed programmatically.

### FR-5: Exemplar Artifacts

Each artifact type must include at least one exemplar — a fully worked
example of a well-written artifact — either inline in the template or as a
companion file. Exemplars should be drawn from real HELIX projects where
possible.

## Non-Functional Requirements

### NFR-1: Template Size

Templates including instructions and checklists must remain under 200 lines
to avoid overwhelming agents with boilerplate.

### NFR-2: Backward Compatibility

Existing artifacts must remain valid. New template features are additive.

## Acceptance Criteria

1. Every artifact template under `workflows/phases/*/artifacts/*/template.md`
   includes section instructions, a generation prompt reference, and a review
   checklist.
2. Every artifact template declares `depends_on` and `enables` relationships
   in its meta.yml.
3. At least one exemplar artifact exists for each artifact type.
4. `helix review` can load the review checklist for the artifact type being
   reviewed and evaluate against it.

## Constraints

- Templates must work with the existing DDX artifact management system.
- Relationship declarations must be compatible with the existing `ddx`
  frontmatter format.

## Out of Scope

- Automated artifact graph visualization (future work).
- Enforcing template compliance at write time (advisory only for now).
