---
ddx:
  id: RSCH-006
  status: draft
  depends_on:
    - REF-015
    - REF-029
id: RSCH-006
title: "Right-Size the Model"
kind: research-synthesis
summary: "Single-model strategies are economically and operationally dominated by tiered, task-aware routing across multiple models — a result confirmed by both 2023 research and 2026 industry consensus."
tags: [routing, multi-model, cost, ddx-principle]
---

# Right-Size the Model

## Principle

There is no one model. Different tasks have different cost/quality
tradeoffs, and the right answer is to route each call to the cheapest
model that can satisfy it, escalating only when needed.

## Synthesis

FrugalGPT (REF-015) provided the foundational empirical result: a cascade
of cheaper-to-more-expensive models, gated by a learned confidence check,
matches GPT-4-class quality on representative workloads at a fraction of
the cost. Chen, Zaharia, and Zou framed three composable techniques —
prompt adaptation, LLM approximation, and model cascades — but the
load-bearing one is the cascade. The empirical claim was strong: most
queries are cheap-model territory, the expensive model is only needed for
the residual hard tail, and a routing policy that recognizes which is
which captures the bulk of the savings.

The 2026 multi-model routing consensus (REF-029) shows the industry has
arrived at the same answer through independent paths. OpenRouter,
LiteLLM, and the major frontier vendors all expose tiered routing,
fallback chains, live capability discovery, and per-task model selection
as first-class features. The shared pattern across vendors and OSS
projects is not coincidence — it is the operationalization of FrugalGPT's
research finding into production tooling. Live discovery matters because
the model catalog churns weekly: hardcoding a model name into a workflow
ages out faster than the workflow does.

The composition: REF-015 says routing wins on cost/quality, REF-029 says
the industry has standardized on the routing surface. The conclusion is
that any AI-native platform that hardcodes a single model — or routes by
named provider profile — is locking in tomorrow's drift.

## DDx Implication

DDx treats the agent service as a routing layer, not a model wrapper.
`ddx agent run --harness=<name>` selects a harness by capability, not by
hardwired model; harnesses can themselves cascade or quorum across
models. The execute-loop runs cheap models first and escalates failed
beads to stronger models on retry — directly mirroring FrugalGPT's
cascade and the user's standing "cost-tiered work" goal. Endpoint-based
discovery (vs. named provider profiles) is the implementation of REF-029's
live-discovery requirement: the catalog is a runtime query, not a
checked-in constant. The cost record kept in `.ddx/executions/` closes
the loop by letting routing decisions be evaluated empirically rather
than asserted.
