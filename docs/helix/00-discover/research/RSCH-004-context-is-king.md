---
ddx:
  id: RSCH-004
  status: draft
  depends_on:
    - REF-008
    - REF-009
id: RSCH-004
title: "Context Is King"
kind: research-synthesis
summary: "LLM performance is dominated by what is in the context window and where; assembling small, well-ordered, task-specific context outperforms dumping the repository in."
tags: [context, retrieval, ddx-principle]
---

# Context Is King

## Principle

The model is not the bottleneck; context selection and ordering are.
A smaller model with the right 4K of context will routinely beat a larger
model given 200K of irrelevant tokens.

## Synthesis

Liu et al.'s "Lost in the Middle" (REF-008) established the canonical
empirical result: across multiple frontier models and task types, recall
follows a U-shaped curve — information at the start and end of the prompt
is reliably retrieved; information in the middle is dropped or
hallucinated over. The effect is robust and reproduced across model
families. The position of evidence in the context window matters as much
as whether the evidence is there at all.

Chroma's 2025 "Context Rot" study (REF-009) extended the finding past
position effects: even when relevant information is well-placed, model
performance degrades as raw input length grows, well within advertised
context windows. Frontier models that report 200K or 1M token windows show
measurable accuracy decay long before those limits are approached. The
larger window is real capacity, but it is not free capacity — every token
spent on irrelevant material is taxing the same attention budget that the
relevant material has to compete in.

The two studies compose into a clear operating constraint: more context is
not better context. The right move is to retrieve aggressively, prune
ruthlessly, and place the load-bearing material at the start or end of the
window. "Just give the model the whole repo" is a strategy that gets
worse, not better, as context windows grow — because the rot effect
scales with how much chaff the model has to sift.

For agent systems this matters compoundingly: agents call themselves and
each other, and each call inherits or assembles its own context. A
multi-agent loop that ladles full transcripts forward at each step is
manufacturing degradation at every hop.

## DDx Implication

DDx is built on the assumption that *what an agent reads* is the unit of
quality. The document library is structured retrieval surface — typed
artifacts (specs, personas, templates, patterns), addressable IDs, and
explicit `depends_on` edges — so a bead can be assembled with exactly the
context it needs and no more. Beads carry only their description and AC
into the execute-bead prompt; governing references are pulled by ID, not
inlined wholesale. Personas (REF-008/REF-009-aware) keep system prompts
short and role-specific rather than catch-all. The architectural bet is
that small, sharp, well-ordered context beats large, loose context — and
the empirical literature backs the bet.
