---
ddx:
  id: RSCH-010
  status: draft
  depends_on:
    - REF-013
    - REF-016
    - REF-027
id: RSCH-010
title: "Inspect and Adapt"
kind: research-synthesis
summary: "Agent quality is built by tight evidence-review loops — generate, critique against concrete evidence, revise — not by single-shot prompting or open-ended autonomy."
tags: [agents, review, self-refine, ddx-principle]
---

# Inspect and Adapt

## Principle

An agent's first output is a draft. Quality comes from a structured
evidence-review loop where a critic — same model, different model, or
deterministic check — compares the draft against a specific contract and
the draft is revised until the contract is satisfied.

## Synthesis

Self-Refine (REF-016) is the smallest demonstrable instance of the loop.
Madaan et al. showed that a single LLM, asked to generate, critique its
own output, then revise based on the critique, improves on a wide range
of tasks versus single-shot generation. The mechanism does not require
a second model — it requires a structured prompt for the critique step
(what to look for, what would count as "wrong") and a willingness to
iterate. The result generalizes because the failure mode it addresses —
first-draft plausibility errors the same model recognizes on a second
pass — is universal to LLMs.

Anthropic's "Building Effective Agents" (REF-027) operationalizes the
loop into design patterns: evaluator-optimizer pairs, orchestrator-worker
patterns, and the explicit recommendation to use the *simplest* pattern
that works. Production-grade agents are workflows of small,
evidence-checked steps — not autonomous loops with vague goals. The
loop must be anchored to concrete checkable outputs at each step;
absent that anchor, the agent drifts.

MAST (REF-013) catalogs what happens when the loop is missing. The 2025
multi-agent failure taxonomy classifies failures across real
benchmarks: coordination breakdowns, role confusion, missing
verification, termination failures. Failures cluster wherever there is
no concrete artifact for one agent to verify another against — the
loop has no evidence to inspect, so it cannot adapt. MAST is the
negative-space proof that REF-016 and REF-027 point at the right
discipline.

The composition: REF-016 shows the loop works at the single-model
level, REF-027 shows how to compose it into production workflows,
REF-013 shows what fails when inspection is absent. This is not
generic agile retrospection — it is a specific claim about
evidence-bound critique loops as the unit of agent quality.

## DDx Implication

DDx runs every bead through a generate-then-review loop. The
execute-bead pass produces a commit; the review pass re-checks against
the same `<acceptance>` items the executor saw, with each AC item
needing a specific, addressable piece of evidence. Failed reviews
reopen the bead and escalate to a stronger model, with the review
findings threaded into the retry prompt as a `<review-findings>`
section — directly mirroring the Self-Refine critique-and-revise loop
but with the critic's output preserved as durable input to the next
attempt. The multi-model `--quorum` and codex-plus-opus plan-review
discipline are the same pattern with independent critics. The
acceptance criteria are the anchor MAST identifies as load-bearing —
without them, the loop has nothing to inspect.
