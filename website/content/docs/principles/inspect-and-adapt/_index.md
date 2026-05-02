---
title: Inspect and Adapt
weight: 10
---

# Inspect and adapt

> An agent's first output is a draft. Quality comes from a structured
> evidence-review loop.

## The principle

This is not generic agile retrospection. It is a specific claim about
**evidence-bound critique loops** as the unit of agent quality. A draft
is generated, a critic — same model, different model, or deterministic
check — compares the draft against a specific contract, and the draft
is revised until the contract is satisfied.

The mechanism does not require a second model; it requires a structured
prompt for the critique step (what to look for, what would count as
"wrong") and a willingness to iterate. It generalizes because the
failure mode it addresses — first-draft plausibility errors the same
model recognizes on a second pass — is universal to LLMs. Production
agents are workflows of small, evidence-checked steps anchored to
concrete checkable outputs, not autonomous loops with vague goals.
Without an anchor, the agent drifts.

The inverse case is well-documented: when the loop is missing,
multi-agent systems fail in characteristic ways — coordination
breakdowns, role confusion, missing verification, termination failures
— that cluster wherever there is no concrete artifact for one agent to
verify another against. The acceptance criteria are the anchor. Without
them, the loop has nothing to inspect, so it cannot adapt.

DDx commits to running on its own queue, paying its own bills, and
adjusting the harnesses, prompts, and process when the numbers say to.
The metrics that matter are throughput (closed beads per unit time),
cost (dollars per closed bead), quality (review-pass rate, regression
rate), and cycle time (queue to merge). The factory must expose these,
not bury them, and treat unfavorable trends as signals.

## Evidence

- **REF-016 — Self-Refine (Madaan et al.).** A single LLM, asked to
  generate, critique its own output, then revise based on the
  critique, improves on a wide range of tasks versus single-shot
  generation. The smallest demonstrable instance of the loop.
- **REF-027 — Anthropic, "Building Effective Agents."** Operationalizes
  the loop into design patterns: evaluator-optimizer pairs,
  orchestrator-worker patterns, and the explicit recommendation to use
  the *simplest* pattern that works.
- **REF-013 — MAST (multi-agent failure taxonomy, 2025).** Catalogs
  what fails when the loop is absent: failures cluster wherever there
  is no concrete artifact for one agent to verify another against.
- **RSCH-010.** REF-016 shows the loop works at the single-model
  level, REF-027 shows how to compose it into production workflows,
  REF-013 shows what fails when inspection is missing.

See `docs/helix/00-discover/research/RSCH-010-inspect-and-adapt.md`.

## DDx response

- **Generate-then-review for every bead.** The execute-bead pass
  produces a commit; the review pass re-checks against the same
  `<acceptance>` items the executor saw, with each AC item needing a
  specific, addressable piece of evidence.
- **Failed reviews reopen the bead** and escalate to a stronger model,
  with review findings threaded into the retry prompt as a
  `<review-findings>` section — directly mirroring Self-Refine's
  critique-and-revise loop, but with the critic's output preserved as
  durable input to the next attempt.
- **Multi-model `--quorum` and codex-plus-opus plan-review** apply
  the same pattern with independent critics, in line with REF-027's
  evaluator-optimizer pattern.
- **The acceptance criteria are the anchor** that MAST identifies as
  load-bearing. Without them, the loop has nothing to inspect.
- **Execution evidence aggregates into per-bead and per-harness
  metrics**, and routine retros against the queue inform spec,
  harness, and routing changes — the inspect-and-adapt loop applied
  to the factory itself, not just to a single bead.
