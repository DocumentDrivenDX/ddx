---
ddx:
  id: REF-015
  status: published
id: REF-015
title: "FrugalGPT: How to Use Large Language Models While Reducing Cost and Improving Performance"
kind: reference
source_url: https://arxiv.org/abs/2305.05176
source_author: Lingjiao Chen, Matei Zaharia, James Zou
accessed: 2026-05-01
summary: "Introduces cost-aware cascades, prompt adaptation, and LLM approximation to deliver GPT-4-class quality at a fraction of the cost via power-aware routing."
tags: [routing, cost, multi-model, research]
---

# FrugalGPT (Chen, Zaharia, Zou, 2023)

Seminal paper on cost-awareed LLM routing: cheap models attempt first, escalate to stronger models only when needed. Foundational reference for DDx's cost-awareed work pattern (cheap models do, strong models review) and Fizeau-style routing.
