---
name: ddx-refine-plan
description: |
  Aggressively refine a draft plan, design, or proposal through adversarial
  multi-reviewer critique and automatic folding. Returns a sharper plan, not
  a pile of review notes. Invoke when the user asks to "review [this/the]
  plan", "review with codex", "fresh eyes review", "re-review", "refine this
  plan", "sharpen this plan", "pressure-test this", "get a second opinion",
  "critique this", "what does codex think", "peer review", or any phrasing
  that asks for external scrutiny and improvement of a draft. Runs fresh-eyes
  (general-purpose subagent) and codex (ddx agent run) in parallel, folds
  findings back into the plan between rounds, loops until convergent or
  until a reviewer surfaces a question that needs user input.
argument-hint: "[path-to-plan.md or leave blank to use current conversation plan]"
---

# DDx Refine Plan

**The output is a refined plan, not review notes.** Reviewer findings are
intermediate machinery — surfaced for transparency, not the deliverable. You
(the session model) are the refinement author: you fold findings each round
and produce the final sharper plan.

Two reviewers in parallel (fresh-eyes via general-purpose subagent, codex via
`ddx agent run --harness codex`), visible per-round iteration, convergence
detection, auto-stop on reviewer questions.

## When to invoke

Trigger on user phrasings like:
- "review [this/the] plan" / "review with codex" / "review with codez"
- "fresh eyes review" / "fresh-eyes review"
- "re-review" / "review again" / "another review"
- "refine this plan" / "sharpen this plan" / "tighten the plan"
- "pressure-test this" / "adversarial review" / "peer review"
- "second opinion" / "what does codex think" / "critique this"
- "review it with codex" (after revising a plan)
- Any request to subject a draft to external scrutiny AND improve it

Do NOT invoke for:
- Post-implementation code review (use helix-review instead)
- Simple "does this look right?" checks (just answer directly)
- Reviewing committed work
- Cases where the user wants notes but not edits (rare; they'll say "just review, don't change the plan" explicitly)

## Input

- If user passed a path argument: read that file as the plan.
- Otherwise: the plan is whatever draft is currently in conversation context.
  Write it to `/tmp/ddx-refine-<timestamp>.md` first so reviewers have a
  stable artifact to read.

## Per-round procedure

1. **Dispatch both reviewers in parallel**:
   - Fresh-eyes: Agent tool with `subagent_type: general-purpose`, prompt
     includes the current plan + the reviewer response template (below).
   - Codex: `ddx agent run --harness codex --prompt /tmp/ddx-refine-<ts>.md`
     in background (typical latency 5-15min; use `run_in_background: true`
     and `ScheduleWakeup` to check).
2. **Wait for both to return**.
3. **Show findings inline in the conversation** — transparency matters so the
   user can interrupt if a reviewer goes off-rail. For each reviewer:
   - Reviewer label + round number
   - Per-section findings (Keep/Cut/Missing/Risks/Questions)
4. **Check stop conditions** (order matters):
   - If ANY reviewer has a `Questions:` section → stop refining, surface
     questions to user, return current plan with open questions annotated.
   - If this round's findings duplicate the prior round (no new
     Cut/Missing/Risks items) → converged, return refined plan.
   - If round count has hit 5 → stop, return plan with note that max
     rounds was hit.
   - If all findings are `Keep:` only → converged, return.
5. **Otherwise**: fold substantive findings into the plan. Show a brief
   refinement summary (1-3 bullets: what changed, what was rejected with
   reason). Loop.

## Reviewer response template

Include this instruction in both reviewer prompts verbatim:

> Respond with these sections (omit any that are empty):
>
> **Keep:** items in the plan that are correct and load-bearing
> **Cut:** items that should be removed (overengineered, premature, wrong)
> **Missing:** items that should be added (gaps, unhandled cases, missing constraints)
> **Risks:** items that might be wrong or need verification (cite evidence where possible)
> **Questions:** questions requiring USER input before the plan can proceed
> (not questions you can answer yourself by reading code)
>
> Be concise. Per-finding limit: 2 sentences. Severity-ordered within each
> section (most important first). Do not invent missing context; if you need
> something the prompt didn't give you, flag it under Questions.

## Final output

When the loop terminates, present:

1. **The refined plan** — this is the deliverable.
2. **Refinement log** (brief):
   - Rounds run (and why loop terminated: converged / question / max)
   - Per-round: what was folded in, what was rejected and why
   - Reviewer-source map: which reviewer caught which item
3. **Open questions** (if stopped for user input): clearly marked, quoted from reviewer.

Lead with the refined plan. The log and questions go after, for the user to
skim or ignore.

## Arm selection

Default arms: fresh-eyes (general-purpose subagent) + codex (`ddx agent run
--harness codex`). If user specifies different reviewers, honor:
- "with opus" → add claude harness with opus model
- "with three reviewers" → add a third arm (suggest claude opus)

## Cost and interruption

Codex runs cost 5-15min latency per round and measurable tokens. 5 rounds ×
2 reviewers = up to 10 reviewer invocations. Stop early on convergence — do
NOT force the loop to max rounds if diminishing returns are visible. If the
user interrupts mid-loop, honor that immediately and return whatever the
plan looks like at that point.

## File conventions

- Draft under active refinement: `/tmp/ddx-refine-<timestamp>.md` (ephemeral)
- Reviewer outputs: returned as tool results; append inline into the draft
  as `## Review round N — <reviewer>` sections for traceability
- Final refined plan: stays in conversation by default. If the user asks to
  persist, write to a location they specify (HELIX convention:
  `docs/helix/02-design/plan-*.md`).
