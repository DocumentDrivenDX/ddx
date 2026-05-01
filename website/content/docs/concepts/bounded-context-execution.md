---
title: Bounded Context Execution
weight: 5
---

How DDx fights **context rot** by running every agent attempt inside a bounded
window — a pattern sometimes called the **Ralph loop**.

## Context Rot

LLMs do not degrade gracefully as their context window fills. Quality drops
well before the hard token limit: the model loses track of earlier
instructions, repeats itself, hallucinates files, and second-guesses prior
decisions. Long-running agent sessions accumulate transcript, tool output,
failed attempts, and partial reasoning — every additional token competes for
attention with the original instructions.

We call this **context rot**: the steady decay in agent output quality as a
single session's context grows. It is not a bug in any one model. It is a
property of how transformer attention scales over a single window, and it
shows up across every frontier model we have measured.

The implication: an agent that has been running for an hour is not the same
agent that started the session. Its judgment has degraded. Its outputs need
more review, not less. And no amount of "remember to..." reminders fixes the
underlying decay — the rot lives in the context, not in the prompt.

## Bounded Context Execution

The fix is structural: keep every unit of agent work inside a fresh,
bounded context window.

**Bounded context execution** is the pattern of:

1. Loading a small, complete contract (the bead description, acceptance
   criteria, and the specific files the work touches) into a clean context.
2. Letting the agent execute against that contract until the contract is
   satisfied or the attempt is abandoned.
3. Discarding the working context. Persistent state — what was done, what was
   learned, what failed — lands on disk as evidence, code, or an updated
   bead, not as transcript carried forward.
4. Starting the next attempt from a fresh window, hydrated only from the
   on-disk substrate.

Each attempt sees only what it needs. Context never accumulates across
attempts. The agent at attempt N+1 is as sharp as the agent at attempt 1
because it is, in the relevant sense, the same agent meeting the work for
the first time.

## The Ralph Loop

The Ralph loop is the name for this iteration shape: **fresh context, bounded
attempt, durable evidence, repeat.** Named after the cartoon character who
introduces himself anew in every scene, the loop trades long-session
"intelligence" for short-session reliability.

A single Ralph iteration:

```
read bead → load minimal context → attempt the work
         → capture evidence → close or re-queue → exit
```

The next iteration starts cold. It re-reads the bead. It re-reads the files.
It does not remember what the previous attempt tried, except through whatever
that attempt wrote down — a commit, a note in the bead, a `no_changes_rationale.txt`,
a `<review-findings>` block threaded into the next prompt.

This is slower per iteration than a long session. It is dramatically more
reliable across iterations, and it is the only known way to drain a queue of
non-trivial work without quality collapsing partway through.

## How DDx Implements It

DDx is built around this loop:

- **Beads** are the bounded contract. Each bead carries its own description,
  acceptance criteria, and dependency context — enough to be executed
  in isolation.
- **`ddx agent execute-bead`** runs a single bounded attempt in an isolated
  git worktree. The agent sees the bead, the worktree, and a tight set of
  instructions. When the attempt ends, the worktree's state is captured as
  evidence and either merged or preserved.
- **`ddx work` (the execute-loop)** drains the bead queue by repeatedly
  invoking `execute-bead`. Each iteration is a fresh agent invocation with a
  fresh context. There is no shared session state across beads.
- **Evidence under `.ddx/executions/`** is the durable memory between
  iterations. Tokens, model, files changed, exit reason, review findings —
  all on disk, all readable by the next iteration if relevant.
- **Review-findings threading** is how learning crosses the boundary without
  carrying transcript. When a review rejects an attempt, the findings are
  written into the next prompt as a `<review-findings>` block. The agent
  does not need the prior transcript; it needs the verdict.

The result is a substrate that respects context rot instead of fighting it.
DDx does not try to make any single agent invocation smarter or longer-lived.
It makes the loop around the agent the smart part, and keeps each invocation
small, fresh, and accountable to a contract on disk.

## See Also

- [Why DDx](/why/) — the pain that this loop addresses, in plain terms.
- [Principles](../principles/) — the load-bearing decisions behind DDx.
- [Execute-loop feature](/features/#execute-loop) — the `ddx work` surface.
