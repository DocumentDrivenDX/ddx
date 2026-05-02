---
ddx:
  id: RSCH-002
  status: draft
  depends_on:
    - REF-018
    - REF-019
id: RSCH-002
title: "Executable Specifications"
kind: research-synthesis
summary: "Specs that generate their own checks — BDD scenarios, contract tests, and rigorously expanded suites — close the gap between intent and verification that prose specs leave open."
tags: [executable-specs, testing, bdd, ddx-principle]
---

# Executable Specifications

## Principle

A specification is only as strong as the checks it generates. Acceptance
criteria written as prose are aspirational; acceptance criteria that compile
into runnable scenarios are enforceable.

## Synthesis

Behavior-Driven Development (REF-019) was the first widely adopted attempt to
collapse the gap between "what the system should do" and "the test that
proves it does." Cucumber and its descendants take Given-When-Then prose
written by product stakeholders and bind it directly to step definitions,
turning the human-readable spec into the executable test suite. The lineage
matters because it shows the pattern is not new to the LLM era — the
discipline of *writing specs that are also tests* predates AI agents by two
decades and survived because it works.

EvalPlus (REF-018) supplies the empirical counterweight: the consequence of
*not* doing this. The authors took HumanEval and MBPP, the two benchmarks
the LLM-code-generation field had been optimizing against, and expanded
their test suites by orders of magnitude. Reported pass@1 scores collapsed —
in some cases models that "passed" 80% of HumanEval problems failed the
expanded suite on the same code. The original specs, expressed as a thin
test suite, had been overstating model correctness for years. The fix was
not better models; it was better executable checks attached to the same
spec.

Together these two references frame the principle. REF-019 shows the
mechanism (specs that compile to scenarios). REF-018 shows the cost of
weak mechanisms (benchmarks that look passed but aren't). For AI-generated
code this is acute: an agent will reliably make any prose spec it reads
look satisfied, because plausible-looking code is what LLMs produce by
default. The only durable check is one the spec itself produces.

## DDx Implication

DDx beads carry an `<acceptance>` block that is intended to be discharged by
a *specific* test, file path, or evidence artifact, not by an agent's
self-assessment. The execute-bead loop and review pipeline both treat AC
items as gates: each item must point to concrete evidence, and the review
agent re-checks against the same AC the implementing agent saw.

This is why DDx stores AC inside beads (structured, queryable) rather than
inside free-form prose: the AC needs to be machine-addressable so a later
review pass — possibly by a different model — can verify item-by-item that
the spec generated its own check. Bead AC plays the same role in DDx that
Cucumber scenarios play in BDD and that the expanded test suite plays in
EvalPlus.
