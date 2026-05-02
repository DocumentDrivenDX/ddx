---
title: Executable Specifications
weight: 2
---

# Executable specifications

> A specification is only as strong as the checks it generates.

## The principle

A spec that cannot be checked is a wish. Executable specifications close
the gap between intent and verification by attaching machine-readable
acceptance criteria, contract tests, or schema constraints to every
requirement, so that "done" is a function the system can compute rather
than a judgment a reviewer must render. Acceptance criteria written as
prose are aspirational; acceptance criteria that compile into runnable
scenarios are enforceable.

This matters most in an agent-driven factory, where dozens of attempts
may target the same bead and a human cannot personally adjudicate each
one. An LLM will reliably make any prose spec it reads *look* satisfied,
because plausible-looking code is what LLMs produce by default. The only
durable check is one the spec itself produces — a test, a contract, a
schema — that fails when the implementation is wrong even when the
implementation looks right.

The cost is up-front rigor: writing the check before the code, and
treating "this AC has no concrete check" as a defect in the spec rather
than a tolerance in review. The payoff is that quality scales with
throughput instead of degrading under it. A factory grading dozens of
agent attempts a day cannot rely on human judgment per attempt; it must
rely on checks the spec already produced. DDx anchors this principle to
specs-that-generate-checks: every bead AC item should point to a runnable
test, a file predicate, or a command whose exit code grades the work.

## Evidence

- **REF-019 — BDD/Cucumber.** Two decades of practice binding
  Given-When-Then prose to step definitions, turning the human-readable
  spec into the executable test suite. The pattern predates the LLM era
  and survived because it works.
- **REF-018 — EvalPlus.** Expanded HumanEval and MBPP test suites by
  orders of magnitude; reported pass@1 scores collapsed. The original
  thin specs had been overstating model correctness for years. The fix
  was not better models — it was better executable checks attached to
  the same spec.
- **RSCH-002.** REF-019 supplies the mechanism (specs that compile to
  scenarios); REF-018 supplies the cost of weak mechanisms (benchmarks
  that look passed but aren't).

See `docs/helix/00-discover/research/RSCH-002-executable-specifications.md`.

## DDx response

- **Bead acceptance criteria** are written as concrete, checkable
  statements: command invocations, file predicates, test names, exit
  codes. The bead `<acceptance>` block is intended to be discharged by a
  *specific* test, file path, or evidence artifact — not by an agent's
  self-assessment.
- **The post-merge review pass** re-checks each AC item against the
  working tree. The same AC the executor saw is what the reviewer
  grades, item by item.
- **Failed reviews reopen the bead** with the unmet items surfaced as
  `<review-findings>` for the next attempt, so the spec's checks drive
  the iteration loop directly.
- **Structured AC** (machine-addressable, queryable) over prose AC: the
  AC needs to be addressable so a later review pass — possibly by a
  different model — can verify item-by-item that the spec generated its
  own check.
