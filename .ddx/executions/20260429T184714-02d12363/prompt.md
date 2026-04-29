<bead-review>
  <bead id="ddx-0097af14" iter=1>
    <title>execute-loop: land_conflict should attempt resolution before discarding preserved iteration</title>
    <description>
When ddx agent execute-bead lands a commit locally but merging into the base branch produces conflicts, the loop today preserves the result at refs/ddx/iterations/&lt;bead&gt;/&lt;ts&gt;-&lt;base&gt; and unclaims the bead with status: land_conflict. The bead reopens; subsequent attempts re-execute from scratch — burning more agent time and dollars even though the original work is intact in the preserved ref.

OBSERVED 2026-04-29 on ddx-52469547 (FEAT-015 bead 3, 'runtime fallback removal + audit-side migration'):
- Sonnet ran for ~3h, produced commit 12fbf6dd1b98 covering all 7 AC items.
- The base branch advanced significantly during the run (V1, V2, V3 vision beads + dogfood + concurrent work).
- The run hit the agent's 3h hard timeout right at the test phase.
- execute-loop preserved at refs/ddx/iterations/ddx-52469547/20260429T175646Z-aa7b31785a44 with status execution_failed (timeout, but functionally identical to land_conflict for the merge story).
- Operator attempted git merge --no-ff 12fbf6dd, hit 7 file-level conflicts (mechanical drift from parallel beads), aborted, and only avoided wasting the work because a parallel attempt (263b5eff) had landed the exact same scope cleanly. That parallel landing is a coincidence, not a guarantee.

The general case: a preserved iteration represents real money spent and real work done. land_conflict / execution_failed-with-result that bails to 'reopen and re-run' throws that work away. The loop should at least attempt to reuse the preserved result.

REQUESTED BEHAVIOR

1. On land_conflict (merge produces conflicts when applying the iteration onto current tip), execute-loop should:
   a. Try a 3-way merge of the preserved iteration ref onto current main using merge.conflictStyle=diff3 and ort strategy with -X ours/-X theirs heuristics. Only if the merge resolves cleanly under those heuristics, commit it as a non-fast-forward merge with a message like 'Merge preserved iteration ddx-&lt;id&gt; after base drift'.
   b. If 3-way auto-resolution fails, dispatch a SMALL focused agent run (cheap tier) with the conflict files, the preserved iteration's commit message, and the bead AC. Its sole job: resolve the conflicts, verify build/tests, commit. Do NOT re-do the bead from scratch.
   c. Only if (a) and (b) both fail, fall back to today's behavior (preserve, unclaim, set short cooldown, mark for human review with a structured outcome distinct from generic execution_failed — perhaps kind:land-conflict-unresolvable).

2. Cooldown for land_conflict-unresolvable should be short (5-30 min), not the 24h cap, since the conflict will resolve faster as soon as a human or another bead lands a clarifying change.

3. Same logic should apply to execution_failed-with-preserved-result (timeouts mid-merge): if a commit was produced before the failure, attempt to merge it before discarding the run.

4. Reuse the auto-recover plumbing landed for push race in ddx-a458af7c (landPushAutoRecover, capLoopCooldown). Specifically the conflict-vs-clean-recovery branching should mirror landPushAutoRecover's structure.

5. Regression tests:
   - Fake git that produces a mechanical conflict resolvable via -X ours: assert auto-resolution succeeds and bead lands.
   - Fake git that produces an irreconcilable structural conflict: assert escalation to focused-resolve agent (or, at minimum, that today's behavior is taken with the new structured outcome).
   - Preserved-iteration-on-timeout case: assert the preserved commit IS attempted as a merge before falling back.

ACCEPTANCE OF FAILURE: if the focused-resolve agent itself fails or escalates to BLOCKING, that's a structured outcome (kind:land-conflict-needs-human) — the bead is parked with the conflict context and preserved ref clearly recorded.

Related: ddx-a458af7c (push race auto-recover, this session's predecessor work).
    </description>
    <acceptance>
1. execute-loop attempts 3-way auto-resolution of preserved iteration onto current tip before discarding (ort strategy + sane heuristics). 2. On 3-way failure, execute-loop dispatches a focused conflict-resolve agent run (cheap tier) with the conflict files + AC + iteration commit message; on success, commits the merge. 3. Only after (1) and (2) fail does the loop park the bead. Park outcome is kind:land-conflict-unresolvable (or kind:land-conflict-needs-human if the focused-resolve agent escalated to BLOCKING). 4. Cooldown for land-conflict-unresolvable is 5-30 min, capped under the 24h cap from ddx-a458af7c. 5. Same code path applies to execution_failed when the agent produced a commit before failing (timeout-with-preserved-commit case). 6. Regression tests: (a) mechanical conflict auto-resolves, (b) structural conflict escalates to focused-resolve, (c) timeout-with-preserved-commit attempts the merge. 7. CHANGELOG entry. 8. Update FEAT-006 (or wherever the execute-loop outcome contract is documented) to list the new structured outcomes.
    </acceptance>
    <labels>execute-loop, beads, quality-of-life, area:agent</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="1550c5382a41b9af89ae6bc296c204608559095a">
commit 1550c5382a41b9af89ae6bc296c204608559095a
Merge: bcec21d8 9f3bedd6
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 29 14:47:11 2026 -0400

    Merge origin/main into main after push race
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
