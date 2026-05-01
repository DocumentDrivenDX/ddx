---
ddx:
  id: CONCEPT-bounded-context-execution
  depends_on:
    - FEAT-010
---
# Concept: Bounded Context Execution

**Status:** Stable
**Owner:** DDx Team

This concept note explains *why* DDx's execution layering
([FEAT-010](../../01-frame/features/FEAT-010-executions.md)) is shaped the way
it is. Each bead is one bounded execution unit, executed by a fresh agent
invocation in an isolated worktree, because long-lived agent sessions degrade
in measurable, well-documented ways. `ddx work` is a bounded context execution
loop, not a persistent session.

## 1. Context Rot

**Context rot** is the steady decay in agent output quality as a single
session's context window fills. It is not a hard cliff at the token limit —
quality drops well before that, and it does so for reasons inherent to how
transformer attention scales over a single window.

### What causes it

- **Attention dilution.** Every additional token in context competes with the
  original instructions for the model's attention. The longer the transcript,
  the smaller the share of attention any one earlier instruction receives.
- **Positional bias.** Models systematically attend more strongly to the
  start and the end of their context than to the middle. Material parked in
  the middle of a long context is, in effect, partly forgotten — even when it
  is the most load-bearing fact in the prompt.
- **Distractor accumulation.** Tool output, failed attempts, exploratory
  reads, and partial reasoning all stay in context. Each one is a distractor
  the model must filter past to recover the original contract.
- **Self-conditioning on stale state.** Once the agent has written a wrong
  hypothesis, retracted a plan, or chased a dead lead, those tokens are
  permanent for the rest of the session. The model conditions on its own
  prior mistakes and tends to repeat or rationalize them.
- **Drift from the contract.** As the session grows, the share of context
  describing what was *requested* shrinks against the share describing what
  has *happened*. The agent gradually optimizes for "make the transcript
  consistent" rather than "satisfy the original ask."

### Why it degrades output quality

The visible symptoms — repeated tool calls, hallucinated file paths,
re-litigated decisions, instructions silently dropped, increasingly verbose
"summaries" of what was just done — are all downstream of the same effect:
the model at minute 60 is no longer reasoning over the same effective prompt
it was at minute 1. Its judgment has degraded, and no in-context reminder
("remember to…") fixes it, because the rot lives in the context itself, not
in any single instruction the prompt could re-issue.

The practical consequence: an agent that has been running for a long time
needs *more* review per unit of output, not less. Long sessions are not a
free lunch in capability — they are a tax on reliability.

## 2. The Ralph Loop

The **ralph loop** is the failure mode this note exists to avoid: a single,
unbounded agent session that is asked to keep working — pulling new tasks,
reading new files, holding new state — without ever resetting its context.

The shape:

```
start session → take task A → finish A → take task B → take task C → ...
             (one growing context, no reset)
```

Each new task piles onto the same window. The session feels productive at
first, then begins to stall: tool calls repeat, prior decisions are second-
guessed, the agent forgets which file it just edited, errors compound. By
the time the operator notices the regression, the session has already merged
or proposed work that reflects degraded judgment, not the agent's actual
capability.

The ralph loop is failure-prone for exactly the reasons context rot is real.
There is no prompt engineering trick that defeats it; the only fix is
structural — bound the context.

## 3. DDx's Answer

DDx treats context as a budget that must be reset between units of work.

- **One bead = one bounded execution unit.** Each bead carries its own
  description, acceptance criteria, and dependency context — enough to be
  executed in isolation against a fresh window. The bead is the contract;
  nothing else needs to be in context to satisfy it.
- **One bead = one fresh agent invocation.** Every attempt at a bead starts
  a new agent process with a clean context. The agent does not inherit
  transcript from a previous bead, a previous attempt at this bead, or a
  previous loop iteration. It re-reads the bead, re-reads the files it needs,
  and works against the contract as if meeting it for the first time.
- **One attempt = one isolated worktree.** `ddx try` (see
  [FEAT-010](../../01-frame/features/FEAT-010-executions.md)) runs the
  attempt in a dedicated git worktree off a known base revision. The agent's
  side effects are bounded to that worktree; the worktree's final state is
  captured as evidence under `.ddx/executions/` and either merged or
  preserved.
- **`ddx work` is a loop of bounded executions, not a session.** The
  drain layer iterates `ddx try` over the ready queue. Each iteration is a
  fresh invocation. There is no shared agent state across beads — the loop
  itself is the only thing that persists across the boundary, and it persists
  on disk, not in any model's context.
- **Learning crosses the boundary as evidence, not transcript.** When work
  needs to inform the next attempt — review findings, a `no_changes_rationale.txt`,
  an updated bead description, a commit message, a `<review-findings>` block
  threaded into the next prompt — it crosses the boundary as a small,
  intentional artifact on disk. The next agent invocation hydrates from
  those artifacts in a clean window. Nothing else carries over.

The agent at iteration N+1 is, in the relevant sense, the same agent meeting
the work for the first time as the agent at iteration 1. That is the point.
DDx does not try to make any single agent invocation smarter or longer-lived;
it makes the loop around the agent the durable thing, and keeps each
invocation small, fresh, and accountable to a contract on disk.

## 4. External References

Published research on in-context degradation that informs this design:

- **Liu et al., *Lost in the Middle: How Language Models Use Long Contexts*
  (2023).** Documents the U-shaped positional bias — models attend strongly
  to the start and end of their context and weakly to the middle — across
  multiple frontier models and retrieval-style tasks. The canonical reference
  for why long contexts are not free.
  [arXiv:2307.03172](https://arxiv.org/abs/2307.03172)
- **Hsieh et al., *RULER: What's the Real Context Size of Your Long-Context
  Language Models?* (2024).** Synthetic long-context benchmark showing that
  effective context length — the length over which a model maintains
  task-relevant accuracy — is typically a fraction of advertised context
  length, and that accuracy degrades smoothly well before the hard limit.
  [arXiv:2404.06654](https://arxiv.org/abs/2404.06654)
- **Levy et al., *Same Task, More Tokens: the Impact of Input Length on the
  Reasoning Performance of Large Language Models* (2024).** Holds the task
  constant and varies only input length; reasoning quality drops monotonically
  as input grows, even far below the model's context limit.
  [arXiv:2402.14848](https://arxiv.org/abs/2402.14848)
- **Li et al., *LongICLBench: Long-context LLMs Struggle with Long In-context
  Learning* (2024).** Shows that even strong long-context models fail to use
  examples placed deep in their context for in-context learning, again
  consistent with positional and distractor effects.
  [arXiv:2404.02060](https://arxiv.org/abs/2404.02060)
- **Anthropic, *Effective context engineering for AI agents* (2025).** Vendor
  guidance on treating context as a finite budget for agentic systems and
  resetting it between units of work — the operational counterpart to the
  research above.
  https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents

These results converge on the same operational rule that shapes DDx's run
architecture: do not let any single agent context grow without bound across
units of work. Bound the context, capture the evidence, and start the next
unit fresh.

## See Also

- [FEAT-010 — Three-Layer Run Architecture](../../01-frame/features/FEAT-010-executions.md)
  — the on-disk and CLI shape of `ddx run` / `ddx try` / `ddx work` that
  implements bounded context execution.
- [`ddx agent execute-loop` command reference](../../../../website/content/docs/cli/commands/ddx_agent_execute-loop.md)
  — the queue-drain surface (aliased as `ddx work`) that runs this loop.
- [Website concept page: Bounded Context Execution](../../../../website/content/docs/concepts/bounded-context-execution.md)
  — the public-facing explainer of the same concept.
