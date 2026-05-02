---
title: Right-Size the Model
weight: 6
---

# Right-size the model

> The cheapest tool that can do the job; escalate only when it cannot.

## The principle

Models are not interchangeable, and using the strongest model for every
step is the most expensive way to be mediocre. Cheap models do well on
narrow, well-specified tasks; strong models earn their keep on review,
synthesis, and ambiguous judgment; deterministic checks (linters, type
systems, tests) outperform every model on the things they cover.

A well-run factory tiers the work. The ordering matters: deterministic
checks at the top of the ladder, then review agents, then implementation
agents. This is not just cost optimization — it is a quality strategy.
Deterministic checks catch what models miss because they are exact;
strong reviewers catch what cheap implementers miss because they have
better judgment; cheap implementers handle the bulk of well-specified
work because most beads are not subtle.

The corollary is that the model catalog is not a fixed asset. Vendors
ship new tiers weekly, deprecate old ones quarterly, and prices move
constantly. A factory that hardcodes a single model — or routes by
named provider profile — is locking in tomorrow's drift. Routing by
capability and budget, against live discovery, is what keeps the
ladder current as the catalog churns.

## Evidence

- **REF-015 — FrugalGPT.** A cascade of cheaper-to-more-expensive
  models, gated by a learned confidence check, matches GPT-4-class
  quality on representative workloads at a fraction of the cost. Most
  queries are cheap-model territory; the expensive model is only
  needed for the residual hard tail.
- **REF-029 — Multi-model routing consensus (2026).** OpenRouter,
  LiteLLM, and the major frontier vendors all expose tiered routing,
  fallback chains, live capability discovery, and per-task model
  selection as first-class features. The pattern is now the industry
  standard.
- **RSCH-006.** REF-015 supplies the empirical case for routing;
  REF-029 supplies the operational standardization.

See `docs/helix/00-discover/research/RSCH-006-right-size-the-model.md`.

## DDx response

- **Harness configs declare capability tiers.** `ddx agent run
  --harness=<name>` selects by capability, not by hardwired model;
  harnesses can themselves cascade or quorum across models.
- **Execute-loop escalates on review failure.** Cheap models run
  first; failed reviews reopen the bead and route the retry to a
  stronger model, mirroring FrugalGPT's cascade directly.
- **Deterministic checks gate merge before any model gets a vote.**
  Tests, lints, and build steps must pass; review and implementation
  models do not get to override the linters.
- **Endpoint-first routing** with live model discovery (per the
  ongoing redesign) replaces named-provider profiles, so the catalog
  is a runtime query rather than a checked-in constant.
- **Cost records in `.ddx/executions/`** close the loop: routing
  decisions are evaluated empirically against closed-bead-per-dollar,
  not asserted.
