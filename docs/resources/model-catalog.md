# Model Catalog

Reference table for routing tier assignment, benchmark scores, and pricing.
Used to drive model selection in agent routing (defined by Fizeau CONTRACT-003-fizeau-service).

**Primary benchmarks:**
- **SWE-bench Verified** — real-world Python bug fixes from GitHub issues; most discriminative for software engineering capability at the frontier
- **LiveCodeBench** — contamination-resistant, continuously updated competitive programming tasks; strong signal for interactive/agentic coding

HumanEval is omitted — saturated at 90%+ across all frontier models, not useful for differentiation.

**Last updated:** 2026-04-12
**Pricing source:** OpenRouter `/api/v1/models` (input/output per 1M tokens USD)

---

## Tiers

| Tier | When to use |
|------|-------------|
| **smart** | User interactive sessions, HELIX document alignment, complex multi-file reasoning, when explicitly requested |
| **standard** | Default for most coding tasks — refactoring, feature work, test writing, code review |
| **fast** | Structured extraction, log analysis, bead status updates, simple transformations, high-volume loops |

---

## Smart Tier

| Model | Provider | SWE-bench | LiveCodeBench | $/M in | $/M out | Notes | As-of |
|-------|----------|-----------|---------------|--------|---------|-------|-------|
| claude-opus-4-6 | Anthropic/OpenRouter | 80.8% | — | $15 | $75 | Top SWE-bench score; primary for interactive sessions | 2026-04-12 |
| gpt-5.4 | OpenAI (codex) | 78.2% | — | — | — | Codex harness primary; no API pricing published | 2026-04-12 |
| gpt-5.3-codex | OpenAI (codex) | 78.0% | — | — | — | Full codex model; routes via codex harness | 2026-04-12 |

---

## Standard Tier

| Model | Provider | SWE-bench | LiveCodeBench | $/M in | $/M out | Notes | As-of |
|-------|----------|-----------|---------------|--------|---------|-------|-------|
| claude-sonnet-4-6 | Anthropic/OpenRouter | 79.6% | — | $3 | $15 | Reference standard; strong instruction following and context | 2026-04-12 |
| minimax-m2.5 | MiniMax/OpenRouter | 80.2% | 65.0% | ~$0.40 | ~$1.60 | 229B; marginally beats Sonnet on SWE-bench; weaker on LiveCodeBench | 2026-04-12 |
| kimi-k2.5 | Moonshot/OpenRouter | 76.8% | 85.0% | ~$0.50 | ~$2.50 | 1T MoE (32B active); standout LiveCodeBench score; best for agentic/interactive loops | 2026-04-12 |
| gpt-4.1 | OpenAI/OpenRouter | ~78% | — | $2 | $8 | GPT-4.1 series; strong instruction following; Sonnet peer | 2026-04-12 |
| gpt-oss-120b | OpenAI (vidar/LM Studio) | — | — | local | local | 88.3% HumanEval; local inference on vidar; no SWE-bench published | 2026-04-12 |

---

## Fast Tier

| Model | Provider | SWE-bench | LiveCodeBench | $/M in | $/M out | Notes | As-of |
|-------|----------|-----------|---------------|--------|---------|-------|-------|
| claude-haiku-4-5 | Anthropic/OpenRouter | 73.3% | — | $0.80 | $4 | Fastest Anthropic model; good for high-volume structured tasks | 2026-04-12 |
| gpt-5.4-mini | OpenAI/OpenRouter | — | — | — | — | Pricing not yet published; expected fast/cheap variant of gpt-5.4 | 2026-04-12 |
| gpt-5.4-nano | OpenAI/OpenRouter | — | — | — | — | Smallest gpt-5.4 variant; pricing not published | 2026-04-12 |
| qwen3.5-27b | Qwen/LM Studio (vidar) | 72.4% | — | local | local | Strong local option; available on vidar | 2026-04-12 |
| qwen3-coder-next | Qwen/LM Studio (vidar) | 70.6% | ~70.7% | local | local | 80B MoE (3B active); 262K context; agentic-optimized, efficient local inference | 2026-04-12 |
| gpt-oss-20b | OpenAI (vidar/LM Studio) | — | — | local | local | 81.7% HumanEval; fast local inference | 2026-04-12 |

---

## Excluded / Deferred

| Model | Reason |
|-------|--------|
| gpt-5.3-codex-spark | Research preview (Cerebras/OpenAI); preview-only access, no stable API; will disappear |
| gpt-4o | Superseded by gpt-4.1 and gpt-5 series for most uses |

---

## Notes

- Costs marked `local` indicate self-hosted inference with no per-token charge beyond hardware
- OpenRouter pricing can be queried programmatically: `GET https://openrouter.ai/api/v1/models` — the `pricing` field on each model entry gives input/output/request costs
- SWE-bench scores are affected by scaffolding (agent framework used); compare within same scaffolding where possible
- Kimi K2.5's 85% LiveCodeBench score is the strongest of any model in this table on that benchmark
- MiniMax-M2.5 at 80.2% SWE-bench technically places above Sonnet 4.6; treat as standard/smart boundary depending on task type
