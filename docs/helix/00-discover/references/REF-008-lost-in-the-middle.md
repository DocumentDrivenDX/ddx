---
id: REF-008
title: "Lost in the Middle: How Language Models Use Long Contexts"
kind: reference
source_url: https://aclanthology.org/2024.tacl-1.9/
source_author: Nelson F. Liu, Kevin Lin, John Hewitt, Ashwin Paranjape, Michele Bevilacqua, Fabio Petroni, Percy Liang
source_organization: TACL 2024
accessed: 2026-05-01
summary: "Empirical study showing LLM performance degrades sharply when relevant information sits in the middle of long contexts, with U-shaped recall curves favoring start and end positions."
tags: [context, long-context, evaluation, research]
---

# Lost in the Middle (Liu et al., TACL 2024)

Establishes that long-context models do not use their full context uniformly: information placed in the middle is recalled far less reliably than information at the start or end. Cited to justify DDx/HELIX's emphasis on small, governing artifacts and curated context windows over dumping everything into the prompt.
