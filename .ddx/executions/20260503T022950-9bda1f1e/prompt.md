<bead-review>
  <bead id="ddx-5b3e57f4" iter=1>
    <title>execute-loop: distinguish worker-disrupted from model-gave-up; do not cooldown on disruption</title>
    <description>
The current cooldown logic treats any ExecutionFailed status as evidence the model can't make progress, applying noProgressCooldown=6h when BaseRev == ResultRev (cli/internal/agent/execute_bead_loop.go:1074-1086 via shouldSuppressNoProgress at line 1550). This silently freezes important work whenever a worker is disrupted (killed, server restart, network blip, OS issue) — disruption produces ExecutionFailed + no commit, indistinguishable from "model genuinely tried and produced nothing."

OBSERVED (today, 2026-05-02)
After the picker fix investigation found the four "starved" P0 beads (ddx-dc157075, ddx-aee651ec, ddx-29058e2a, ddx-9d55601f) all had execute-loop-retry-after timestamps ~3 hours in the future. Investigation per ddx-9d55601f confirmed these were NOT genuine model failures — they were prior worker disruptions during this development session (worker kills, server restarts) being treated as model-gave-up.

User directive: "Cooldown should be reserved for cases where the model is truly unable to make progress on a task."

CURRENT COOLDOWN POLICY (cli/internal/agent/execute_bead_loop.go)
- LandConflictCooldown = 15min — short cooldown after merge/land conflicts (line 64)
- noProgressCooldown = 6h default — when shouldSuppressNoProgress is true (line 367)
- MaxLoopCooldown = 24h — push_failed, push_conflict, decomposition decline (line 57)

shouldSuppressNoProgress (line 1550): returns true if BaseRev != "" AND ResultRev != "" AND BaseRev == ResultRev. This catches:
- Model wrote nothing AND came back cleanly (true no_progress) ✓
- Worker killed mid-execution before commit (FALSE no_progress — disruption)
- Network/provider error before model started (FALSE no_progress — disruption)
- Routing preflight rejection (FALSE no_progress — config issue)

The mis-classification is the bug.

PROPOSED FIX

Add a Disposition::Disrupted variant (or equivalent flag on ExecuteBeadReport) that explicitly marks results that are NOT evidence of model inability:
- ctx.Err() != nil (cancellation, deadline, parent died)
- syscall.SIGTERM / SIGINT received during execution
- Provider-side 5xx, connection refused, timeout (transport disruption)
- Worker process killed (parent never received clean result)
- Server-restart-detected marker
- Routing preflight rejection (operator config issue, not model failure)

For Disrupted: skip cooldown entirely (or use a tiny defer like 30s) and unclaim the bead so the next worker pickup is immediate.
For genuine no-progress (model returned clean with no commit AND rationale present): keep current 6h cooldown.

ALTERNATIVE: exponential backoff per bead. First failure: 5min; second: 30min; third: 2h; cap at 24h. Resets on any successful attempt or operator clear. This is more forgiving than the current flat 6h on the first hiccup.

PROBABLY BOTH: Disrupted skips cooldown entirely; non-Disrupted failures use exponential backoff starting at 5min, not flat 6h.

DETECTION POINTERS

- ctx.Err() check at the call site that produces ExecutionFailed (line 513, 629, 637, 1011)
- Check for `.ddx/executions/&lt;run-id&gt;/no_changes_rationale.txt` presence — if missing, the model never wrote one (likely disruption)
- Check the executor's exit code: SIGKILL/SIGTERM = disruption; clean exit with empty result = no-progress
- Track parent process restart count or boot ID; if changed since the attempt started, mark Disrupted

NOT IN SCOPE
- Changing the cooldown duration constants themselves
- Reworking the per-bead retry policy framework (separate concern)
- Changing land-conflict or push-failed cooldowns (those are correct as-is)
- The exponential-backoff alternative is OPTIONAL; primary fix is Disrupted classification

INTERSECTION WITH OTHER BEADS
- ddx-dc157075 (worker stay-alive): the stay-alive fix prevents some classes of disruption (worker no longer exits on empty poll), but does NOT fix the cooldown-on-disruption mis-classification
- ddx-076147ee (worker client-server architecture TD): if workers become long-lived clients, server-restart disruption goes away by construction. But this fix is needed BEFORE that TD is implemented because the disruption classification is needed regardless.
- ddx-9228a484 C9 (StopCondition enum): may absorb this — the StopCondition enum could include Disrupted as a state. Verify and align.
    </description>
    <acceptance>
1. ExecuteBeadReport gains an explicit Disrupted bool (or Disposition variant) set when the loop detects: ctx.Err() != nil, executor exited via SIGKILL/SIGTERM, transport-class provider error, or worker process restart marker.
2. Detection at: cli/internal/agent/execute_bead_loop.go where ExecutionFailed is set (current lines 513 routing-preflight, 629/637 executor-error, 1011 in execute_bead.go) — each set point evaluates whether the failure is Disrupted.
3. The cooldown branch (lines 1074-1086) checks Disrupted FIRST: if Disrupted, skip SetExecutionCooldown entirely; just Unclaim so the bead returns to ready.
4. shouldSuppressNoProgress unchanged for genuine no-progress (BaseRev == ResultRev AND not Disrupted AND no rationale).
5. New diagnostic event `disruption_detected` with kind, reason, and bead context — emitted when Disrupted is set so operators can see disruption rates.
6. (Optional) Exponential backoff for non-Disrupted no-progress: first failure 5min; second 30min; third 2h; cap at 24h. Reset on success. If implementing: replace flat noProgressCooldown with this curve; document in code comment.
7. Tests:
   - TestLoop_DisruptedExecution_NoCooldown — context cancelled mid-execution; assert no execute-loop-retry-after set; bead returns to ready
   - TestLoop_GenuineNoProgress_StillCooldowns — model returns clean with no commit + rationale; assert 6h cooldown set
   - TestLoop_PreflightRejection_NoCooldown — routing preflight fails; assert disruption classification (operator config, not model failure)
   - TestLoop_DisruptionEventEmitted — assert disruption_detected event appears in bead events
8. Manual verification: kill an in-flight worker mid-execution; assert the bead is immediately re-claimable by the next worker (no 6h park).
9. cd cli &amp;&amp; go test ./internal/agent/... green; lefthook pre-commit passes.
10. Documentation: update FEAT-006 (or wherever the cooldown policy is documented) to describe Disrupted vs no-progress and the policy rationale.
    </acceptance>
    <labels>phase:2, area:agent, kind:fix, observed-failure</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="89473d084d436e925934549d31f560e5d6b8702a">
commit 89473d084d436e925934549d31f560e5d6b8702a
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat May 2 22:29:48 2026 -0400

    chore: add execution evidence [20260503T021724-]

diff --git a/.ddx/executions/20260503T021724-8a33226e/result.json b/.ddx/executions/20260503T021724-8a33226e/result.json
new file mode 100644
index 00000000..43e5fc88
--- /dev/null
+++ b/.ddx/executions/20260503T021724-8a33226e/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5b3e57f4",
+  "attempt_id": "20260503T021724-8a33226e",
+  "base_rev": "70af508ab97823f10e636e22ca45ccdca223f0a8",
+  "result_rev": "47d8054ed928f0cd71f21628bf3810e0090e6b10",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-59cfc8c5",
+  "duration_ms": 734161,
+  "tokens": 29450,
+  "cost_usd": 5.18310475,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T021724-8a33226e",
+  "prompt_file": ".ddx/executions/20260503T021724-8a33226e/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T021724-8a33226e/manifest.json",
+  "result_file": ".ddx/executions/20260503T021724-8a33226e/result.json",
+  "usage_file": ".ddx/executions/20260503T021724-8a33226e/usage.json",
+  "started_at": "2026-05-03T02:17:29.792541357Z",
+  "finished_at": "2026-05-03T02:29:43.953596326Z"
+}
\ No newline at end of file
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: ddx-5b3e57f4 iter 1

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | &lt;AC item text, max 60 chars&gt; | APPROVE | path/to/file.go:42 — brief note |
| 2 | &lt;AC item text, max 60 chars&gt; | BLOCK   | — not found in diff |

### Summary

&lt;1–3 sentences on overall implementation quality and any recurring theme in findings.&gt;

### Findings

&lt;Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.&gt;
  </instructions>
</bead-review>
