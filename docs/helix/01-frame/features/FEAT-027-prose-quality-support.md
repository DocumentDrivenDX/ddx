---
ddx:
  id: FEAT-027
  depends_on:
    - helix.prd
---
# Feature: Prose Quality Support

**ID:** FEAT-027
**Status:** Implemented
**Priority:** P2
**Owner:** DDx Team

## Overview

DDx Prose Quality Support defines deterministic, explainable prose checks for
governing artifacts and user-facing docs. The goal is clearer, more specific,
voice-preserving writing in DDx materials, not AI-detection evasion.

This feature introduces a default skill surface, a deterministic checker/rules
surface, and a docs command surface for reviewing changed prose. It produces
structural findings that can be reviewed, tracked, and fed into review
workflow integration.

## Problem Statement

**Current situation:** Governing artifacts and documentation can drift into
vague, generic, high-level prose that reads fine on a first pass but weakens
document-driven development. When prose is too generic, maintainers spend more
time inferring intent than executing it.

**Pain points:**
- Technical docs hide concrete behavior behind broad claims and filler phrasing
- Planning docs lose actionable detail because the prose stops short of naming
  the actual decision, constraint, or boundary
- Public prose becomes polished but imprecise, which makes DDx harder to trust
  as a source of truth
- Reviewers have no deterministic prose-quality signal to point at when the
  problem is specific wording rather than missing requirements

**Desired outcome:** DDx can identify common AI-writing tropes, generic prose
patterns, and recognizable LLM-default constructions with deterministic rules,
explain each finding, and suggest a concrete edit without rewriting the
author's voice away.

## Users and Content Modes

### Primary Users

- DDx maintainers
- Project maintainers using DDx governing artifacts
- Agents writing or reviewing docs

### Content Modes

Prose quality checks apply to three modes:

- **Technical** prose: feature specs, design docs, reference docs, and command
  descriptions
- **Planning** prose: beads, plans, roadmaps, and implementation notes
- **Public** prose: website copy, release notes, and externally visible docs

The rules may vary by mode, but they share the same deterministic output shape
and advisory default behavior.

## Product Surfaces

### 1. Default Skill

DDx ships a default skill surface that can guide agents toward concise,
specific prose and surface the prose-quality workflow when appropriate. The
skill is advisory: it helps authors improve prose, but it does not change the
meaning of the authored artifact.

### 2. Deterministic Checker and Rules

DDx defines prose-quality checks as deterministic rules over text. The checker
does not attempt to classify human vs. AI authorship and is not an AI detector.
It evaluates observed text against named rules and explains each match.

### 3. `ddx doc prose --changed`

DDx adds a docs surface for checking changed prose only. The command is meant
for pre-review and pre-merge use cases where maintainers want a focused,
diff-based advisory report instead of a full repository scan.

### 4. Review Integration

`ddx bead review <id> --prose` uses the same checker and emits prose findings
as advisory review evidence. Prose findings stay separate from correctness
findings and do not change the review verdict by default.

## Requirements

### Functional

1. **Deterministic rule evaluation** — prose checks must run as rule-based
   analysis over the selected text and return repeatable results for the same
   input.
2. **Mode awareness** — the checker must support technical, planning, and
   public prose modes, with rule application appropriate to the selected mode.
3. **Advisory default** — prose findings are advisory by default and do not
   block docs operations unless a later policy explicitly opts into blocking.
4. **Structural findings** — each finding must include:
   - file path
   - line or line range
   - rule id
   - severity
   - rationale
   - suggested edit
5. **Project vocabulary** — findings and suggestions must preserve project
   terminology when possible instead of rewriting terms into generic language.
6. **Changed-only review surface** — `ddx doc prose --changed` evaluates only
   changed prose by default and reports findings for the touched lines.
7. **Explainable output** — the checker must describe why a rule fired using
   concrete textual evidence from the input.
8. **Review compatibility** — the finding format is stable enough for
   `ddx bead review <id> --prose` to consume without changing the core rule
   model.
9. **AI-slop avoidance** — prose checks must explicitly target common
   LLM-default constructions: unsupported polish words, vague benefit claims,
   inflated transition phrases, generic summary sentences, and wording that
   sounds fluent while omitting actor, action, artifact, boundary, or evidence.

### Measurable Acceptance Criteria

The feature is considered successful when it can produce deterministic,
command-verifiable findings with the following structure for a changed prose
sample:

- `file`: the path of the changed document
- `line` or `line_range`: the affected location
- `rule_id`: a stable deterministic identifier
- `severity`: advisory severity value
- `rationale`: a short explanation tied to the observed text
- `suggested_edit`: a concrete replacement or rewrite suggestion

The feature must also be able to flag generic prose patterns such as vague
claims, filler transitions, uncoupled abstractions, and common AI-slop
constructions while preserving the document's own vocabulary and intended
voice.

Findings must be structural rather than subjective. Each result must trace to a
specific file and line span, and the rationale must explain the triggered rule
in terms of observed text instead of a broad style judgment.

## Non-Goals

- No AI detector
- No detector bypass
- No default blocking behavior
- No automatic rewriting that strips authorial voice
- No AI-based rewrite engine
- No semantic judgment of correctness; prose findings stay separate from
  acceptance and implementation review

## Rule Model

The prose-quality checker should reason in terms of named rules, not opaque
scores. A rule may detect one or more common prose tropes, but each emitted
finding must remain explainable and reviewable on its own.

### Example Rule Families

- Generic claim without specific subject or consequence
- Overly abstract sentence that omits the concrete artifact or action
- Repetition of empty emphasis phrases
- Passive or indirect phrasing where the responsible actor is known
- Voice drift that replaces project-specific terminology with generic wording
- LLM-default polish that uses fluent confidence in place of concrete evidence

These families are examples of the target shape, not a final implementation
catalog.

## Command and Skill Boundaries

- The default skill helps authors and agents notice prose issues during writing
  and review
- The deterministic checker owns the actual finding generation
- `ddx doc prose --changed` is the primary command surface for reviewing only
  changed prose
- `ddx bead review <id> --prose` is an advisory consumer of the same
  structured findings

## Out of Scope

- Detector scoring heuristics that try to infer authorship
- Content transformation that rewrites style by default
- Blocking docs operations by default
- Automatic semantic rewrites
- Treating prose findings as correctness failures by default
