---
title: Why DDx
description: "Ten domain principles for running a document-driven software factory with humans and agents."
weight: 1
---

# Why DDx

DDx exists to run software development as a [software factory](/docs/concepts/software-factory/): a system where specifications, evidence, and tracked work — not transcripts and tribal knowledge — are the durable substrate, and where humans and agents both operate against the same contracts. The ten domain principles below are the load-bearing claims behind that factory. Each one is cited in real design and review decisions; each one has a deep page that walks through what it means and how DDx applies it.

## 1. Spec-First Development

Specifications, not code, are the durable artifact in a document-driven factory. Code is one rendering of intent; the spec is the source. When the spec changes, work is created to reconcile the code; when implementation reveals the spec was wrong, the spec is corrected first and the code follows. Beads, reviews, and acceptance criteria all trace back to a written contract.

[Read more →](/docs/principles/spec-first-development/)

## 2. Executable Specifications

A specification is only as strong as the checks it generates. Prose alone rots; tests, schemas, and machine-checkable acceptance criteria don't. DDx beads carry explicit acceptance items, and review verifies them rather than the conversation. Wherever possible, "the spec says X" should be expressible as a check that fails when X is not true.

[Read more →](/docs/principles/executable-specifications/)

## 3. Audit Trail Required

The log, not the current state of the working tree, is the truth. Every agent run captures evidence — model, tokens, files changed, exit metadata — to disk. Outcomes are reconstructable after the fact, debugging doesn't depend on memory, and trust scales because what happened is observable rather than asserted.

[Read more →](/docs/principles/audit-trail-required/)

## 4. Context Is King

The model is not the bottleneck; context selection and ordering are. Agents start every invocation cold, so what gets handed in — and what gets left out — decides the quality ceiling. DDx treats context assembly as a first-class concern: artifacts are discoverable, beads are self-contained, and a single skill teaches the agent the toolkit's shape.

[Read more →](/docs/principles/context-is-king/)

## 5. Work Is a DAG

Work is not a list. It is a directed acyclic graph. Beads have dependencies; the ready queue is computed from the graph, not maintained by hand. Parallel agents can drain independent branches without stepping on each other, and blocked work surfaces as a structural fact rather than a status field someone forgot to update.

[Read more →](/docs/principles/work-is-a-dag/)

## 6. Right-Size the Model

The cheapest tool that can do the job; escalate only when it cannot. Routine work runs on cheap models; review and hard problems escalate to stronger ones; deterministic checks sit at the top of the ladder catching what review missed. Throughput is measured in closed beads per dollar, not in raw model capability.

[Read more →](/docs/principles/right-size-the-model/)

## 7. Avoid Vendor Lock-In

Standardize the contract; let implementations compete underneath. Harnesses plug in via a prompt-in/output-out interface; routing is by endpoint and capability, not by hardcoded provider name; storage is plain files and standard git. Switching a model, a harness, or a host is a configuration change, not a migration.

[Read more →](/docs/principles/avoid-vendor-lock-in/)

## 8. Drift Is Debt

Documentation that lives separately from the code it describes will drift out of correctness. DDx makes the artifact graph explicit: relationships between specs, code, and beads are tracked, and staleness becomes detectable rather than discovered the hard way during onboarding or an incident. Closing drift is treated as work, not hygiene.

[Read more →](/docs/principles/drift-is-debt/)

## 9. Least Privilege for Agents

An agent is treated as an untrusted process that happens to be useful, not as a trusted teammate. Each run executes in an isolated worktree with a narrow contract; access to credentials, networks, and shared state is scoped to what the bead actually needs. The blast radius of a bad run stays bounded.

[Read more →](/docs/principles/least-privilege-for-agents/)

## 10. Inspect and Adapt

An agent's first output is a draft. Quality comes from a structured evidence-review loop. Runs produce diffs and evidence; review checks them against acceptance criteria; failures escalate to a higher tier with the prior findings threaded forward. Iteration is the norm, not a fallback — and it is bounded so context rot doesn't compound.

[Read more →](/docs/principles/inspect-and-adapt/)

---

Ready to try it? Start with [Get Started](/docs/getting-started/) or browse the [Features](/features/).
