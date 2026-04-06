---
name: helix-frame
description: Create or refine product vision, PRD, feature specs, and user stories. Use when starting a new project, adding a major feature, or when upstream requirements need definition.
argument-hint: '[scope]'
---

# Frame: Define What to Build

Frame creates the highest-authority artifacts in the HELIX stack — the
documents that govern everything downstream. Use it when you need to define
or refine what the product should do before designing how.

## When to Use

- Starting a new project from scratch
- Adding a major new capability or feature area
- Requirements are unclear or contradictory
- `helix run` stopped with GUIDANCE because upstream authority is missing
- You want to write a PRD, vision document, or feature spec

## What It Produces

Frame-phase artifacts (authority levels 1-3):

- **Product Vision** (`docs/helix/00-discover/product-vision.md`) — mission,
  market, value propositions, principles, success metrics
- **PRD** (`docs/helix/01-frame/prd.md`) — problem, goals, requirements
  (P0/P1/P2), constraints, risks, success criteria
- **Feature Specs** (`docs/helix/01-frame/features/FEAT-*.md`) — per-feature
  requirements, user stories, acceptance criteria
- **User Stories** — captured within feature specs or standalone

## Steps

1. Read existing Frame-phase artifacts if they exist
2. Read the artifact templates **and** `prompt.md` guidance at
   `workflows/phases/00-discover/artifacts/` and
   `workflows/phases/01-frame/artifacts/`
3. Create or update the appropriate documents using the templates
4. Iteratively refine through multiple rounds (challenge assumptions,
   fill gaps, sharpen requirements)
5. Validate all blocking quality checks in `dependencies.yaml` pass before
   committing
6. Create tracker issues for any Design-phase work implied by the framing

## Examples

```bash
helix frame                    # Frame the whole project
helix frame auth               # Frame the auth capability
helix frame "mobile payments"  # Frame a new feature area
```

## References

- Vision template: `workflows/phases/00-discover/artifacts/product-vision/`
- PRD template: `workflows/phases/01-frame/artifacts/prd/`
- Feature spec template: `workflows/phases/01-frame/artifacts/feature-specification/`
- Authority order: `workflows/actions/implementation.md`
