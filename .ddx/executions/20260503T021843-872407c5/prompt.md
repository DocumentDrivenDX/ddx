<bead-review>
  <bead id="ddx-076147ee" iter=1>
    <title>TD: client-server worker architecture — workers as long-lived API clients; server restart preserves in-flight work</title>
    <description>
Operator-observed gap: when ddx-server restarts, in-flight worker subprocesses lose their connection to the orchestrator. The current model has two execution paths (--local vs server-spawned) that diverge in lifecycle management, configuration plumbing (the dropped-flags class — see ddx-29058e2a), and observability (server can see only what it spawned).

Proposed architectural shift: WORKERS ARE LONG-LIVED PROCESSES THAT PHONE HOME TO THE SERVER VIA API.

DESIGN PROPOSALS (this TD chooses among them)

PROPOSAL A — Server orchestrates, workers are long-lived clients
- Worker subprocess starts (either via `ddx work` CLI or via server-spawned)
- Worker registers with server (via /api/workers/register, gets worker-id + heartbeat token)
- Worker polls /api/workers/&lt;id&gt;/next-bead via long-poll or websocket; receives bead spec from server
- Worker reports progress via /api/workers/&lt;id&gt;/event {kind, body}
- Worker reports completion via /api/workers/&lt;id&gt;/result {bead-id, status, evidence}
- If server restarts: worker's heartbeat fails → worker enters "disconnected" state, finishes current bead, reconnects on server return
- If worker dies: server's heartbeat-timeout reclaims the bead

PROPOSAL B — Server is optional sidecar, worker is autonomous
- Worker runs independently; no server required
- If a server is reachable (via .ddx/server.addr), worker emits events to it best-effort
- Server is a passive observer, not an orchestrator
- Restart-safe by construction (worker doesn't depend on server)
- Loses: server-driven label filters, dynamic priority overrides, centralized cancel

PROPOSAL C — Hybrid: workers are autonomous executors but yield orchestration to server when present
- Worker checks for .ddx/server.addr at startup
- If present: registers as managed; receives bead assignments from server's queue
- If absent or unreachable: runs autonomously using local bead store
- Server restart: managed workers detect via heartbeat failure; gracefully complete current bead, then either retry registration OR fall back to autonomous mode (configurable)

UNIFIED API CONTRACT (all proposals)

POST /api/workers/register { project_root, harness, model_pref, label_filter, capabilities[] } → { worker_id, session_token, heartbeat_interval_ms }
POST /api/workers/&lt;id&gt;/heartbeat { state: idle|claiming|executing|reviewing } → { server_command: continue|pause|drain|terminate }
GET  /api/workers/&lt;id&gt;/next-bead → { bead, attempt_id, base_rev } | { wait_for_seconds: N }
POST /api/workers/&lt;id&gt;/event { kind, body, bead_id } → 204
POST /api/workers/&lt;id&gt;/result { bead_id, attempt_id, status, evidence_dir, commit_sha } → 204
POST /api/workers/&lt;id&gt;/disconnect → 204 (graceful)

The existing --local mode collapses to: a worker that registers with itself (no server) using an in-process implementation of the API. Same code path, same flags, same lifecycle.

INTEGRATION WITH IN-FLIGHT WORK

This TD intersects:
- ddx-29058e2a (ExecuteLoopSpec unification): the spec becomes the registration payload — single struct still applies
- ddx-5cb6e6cd refactor epic: the work package boundary aligns with worker autonomy; C9 (StopCondition) becomes worker-side state
- ddx-4c51d33e (cross-project leak): workers identify their project at registration; server enforces project boundaries via session token
- ddx-dc157075 (worker stay-alive): becomes natural — worker stays alive, polls for work, doesn't exit on empty queue

IMPLEMENTATION OUTLINE (post-TD)
1. Define API contract + handlers (cli/internal/server/worker_api.go)
2. Add worker client (cli/internal/agent/work/client.go) implementing the API
3. Add in-process API implementation for --local mode (cli/internal/agent/work/local_api.go)
4. Migrate execute_bead_loop / drain_loop to consume the API instead of direct store access
5. Migrate server worker spawn to register-then-poll instead of inline-execution

NOT IN SCOPE FOR THIS TD
- Specific code changes (this TD documents the design; implementation is follow-up beads)
- Multi-server federation (each server owns its workers; cross-server is a separate concern)
- Auth beyond session tokens (the worker-server channel is local; no remote workers in v1)

OUTPUTS (TD deliverables)
- ADR document at docs/helix/03-decide/ADR-XXX-worker-client-server-architecture.md
- Updated FEAT-006 (or new FEAT) describing the worker contract
- Sequence diagrams for: register, claim, execute, restart-recovery, graceful-shutdown
- Compatibility table: which existing tests/behaviors break, which migrate cleanly
- Implementation roadmap: ordered list of beads to file as follow-ups

DEPS
- Should land BEFORE major refactor children that touch the loop body (C5, C7, C9) — otherwise they'd refactor against the OLD model and need re-doing
- Should NOT block routine bug fixes or feature beads
    </description>
    <acceptance>
1. ADR document at docs/helix/03-decide/ADR-XXX-worker-client-server-architecture.md exists, named appropriately, committed.
2. ADR explicitly evaluates Proposals A, B, C (or articulates a fourth) and documents the chosen design with rationale.
3. ADR includes the worker-server API contract: full request/response schemas for register, heartbeat, next-bead, event, result, disconnect.
4. ADR includes sequence diagrams (text/mermaid) for: worker registration, bead claim, execute+report, server-restart recovery, graceful shutdown.
5. ADR includes a compatibility analysis: what existing tests break, what behaviors migrate, what's a breaking change for operators.
6. ADR specifies how --local mode collapses into the same code path (in-process API impl).
7. ADR specifies sequencing relative to: ddx-29058e2a (spec unification), ddx-5cb6e6cd refactor children, ddx-dc157075 (stay-alive — likely subsumed).
8. ADR includes implementation roadmap as an ordered list (with proposed bead titles, NOT actual filed beads — those come after ADR review).
9. FEAT-006 or new FEAT amended to describe the worker contract (whichever is more appropriate per existing HELIX layout).
10. Reviewed and approved by user before any implementation beads are filed from the roadmap.
    </acceptance>
    <labels>phase:2, area:server, area:agent, kind:design, td</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260503T021424-dac100b6/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="fabf513096b522e921020e323ef1bf4a9cf36afa">
diff --git a/.ddx/executions/20260503T021424-dac100b6/result.json b/.ddx/executions/20260503T021424-dac100b6/result.json
new file mode 100644
index 00000000..163631cd
--- /dev/null
+++ b/.ddx/executions/20260503T021424-dac100b6/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-076147ee",
+  "attempt_id": "20260503T021424-dac100b6",
+  "base_rev": "528bb6ee9c523ad5fe7a76b101611ba9b597339a",
+  "result_rev": "16fd637b43599345c4d9aed3d097c628eb545d89",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-eca61029",
+  "duration_ms": 249728,
+  "tokens": 14820,
+  "cost_usd": 1.0767455,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260503T021424-dac100b6",
+  "prompt_file": ".ddx/executions/20260503T021424-dac100b6/prompt.md",
+  "manifest_file": ".ddx/executions/20260503T021424-dac100b6/manifest.json",
+  "result_file": ".ddx/executions/20260503T021424-dac100b6/result.json",
+  "usage_file": ".ddx/executions/20260503T021424-dac100b6/usage.json",
+  "started_at": "2026-05-03T02:14:28.276832345Z",
+  "finished_at": "2026-05-03T02:18:38.005623371Z"
+}
\ No newline at end of file
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
