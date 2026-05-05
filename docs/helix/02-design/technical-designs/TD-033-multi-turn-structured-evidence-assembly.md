---
ddx:
  id: TD-033
  depends_on:
    - FEAT-022
    - FEAT-006
    - FEAT-008
  status: draft
---
# Technical Design: Multi-Turn Structured Evidence Assembly

## Purpose

This TD defines the architecture for assembling large DDx prompts in a
structured, multi-turn way. FEAT-022 defines the evidence caps and
bounded-assembly invariants; this TD defines how callers stage evidence,
resolve the assembly mode, and render a final prompt without each surface
inventing its own topology.

The design exists for three callers that need consistent behavior:

- the agent service boundary in FEAT-006
- the UI-triggered review flow in FEAT-008
- the bounded review and grading prompts in FEAT-022

The goal is not to introduce a single monolithic builder. The goal is to
standardize the shape of the assembly process so the review path, grading
path, and primary invocation path can share the same evidence vocabulary
while still choosing different top-level ordering and budgets.

## Current State

Today FEAT-022 already requires bounded prompt assembly, minimum evidence
floors, and a no-unbounded-prompts invariant. What is still missing is a
documented topology for how a caller moves from raw evidence to the final
rendered prompt when the request is review-like and tool-free.

That gap shows up in three places:

- FEAT-006 owns the request boundary but does not yet name the review-mode
  assembly shape.
- FEAT-008 exposes an "Agent review" action but does not yet say that the
  resulting session is tool-free by convention.
- FEAT-022 defines the caps and the section content rules but intentionally
  stops short of mandating a repo-wide `EvidenceAssembler` object.

## Design

### 1. Structured evidence envelope

Every assembly run begins with a small structured envelope that records:

- intent: `primary`, `review`, or `grading`
- mode: `ref-only` or `inline-with-cap`
- target budget: the applicable cap for the chosen intent
- evidence source set: bead metadata, docs, diffs, runs, or prompt bodies

The envelope is not the final prompt. It is the contract that lets the caller
plan the assembly without knowing the full content shape up front.

### 2. Multi-turn assembly phases

The assembly proceeds in three phases:

1. Plan the evidence set and section priority order.
2. Materialize the sections with the FEAT-022 cap primitives.
3. Render the final prompt and record the byte accounting.

This can happen in one process call, but it is still a multi-turn
architecture conceptually because the caller first chooses the evidence plan
and then performs the final render against that plan.

### 3. No-tool reviewer mode

Review-mode sessions do not receive tools. The caller hands the reviewer a
structured evidence envelope plus the rendered prompt, and the reviewer is
expected to reason over the supplied content only.

That convention matters because the review prompt is meant to be stable and
auditable. If a reviewer needs additional evidence, the caller must assemble it
up front rather than relying on a later tool call.

### 4. Caller-specific ordering

The shared envelope does not force a single assembly order across all call
sites.

- FEAT-022 review and grading paths keep their own section priorities.
- FEAT-006 keeps request-level intent and evidence capture ownership.
- FEAT-008 surfaces the review action and dispatches the session in the
  no-tool reviewer mode described here.

## Non-Goals

- This TD does not replace FEAT-022's cap primitives.
- This TD does not define new routing policy.
- This TD does not introduce map-reduce per-file review or any other
  parallel fan-out architecture.
- This TD does not require a new repository-wide assembler type.

