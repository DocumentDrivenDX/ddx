<bead-review>
  <bead id="ddx-61357e13" iter=1>
    <title>server: singleton-per-machine guard + CLI validates pid-alive before trusting server.addr</title>
    <description>
Two related bugs causing 'ddx server workers list' (and other server-talking CLI commands) to fail with 'connection refused dial tcp 127.0.0.1:&lt;stale-port&gt;' even when a live server is running.

## Diagnosis

**writeAddrFile already exists** at cli/internal/server/server.go:287-312 and is called from line 273 on every startup. So the file IS written. The real bugs are:

**Bug A: No singleton-server guard.** Multiple 'ddx server' instances can start on the same machine concurrently. Each one calls writeAddrFile and overwrites the previous record. When one dies (e.g., a manual launch test), it leaves the address-file pointing to itself even though it's no longer running. systemd unit (Type=simple) prevents systemd-spawned duplicates of the same unit name but does NOT stop a manual 'ddx server' from racing.

Observed today: live systemd server PID 3442304 listening on 7743 (started ~14:27), but address file shows PID 3482077 / port 4174 (started ~15:01, died). Some manual or test launch overwrote the file then died.

**Bug B: CLI doesn't validate pid-alive before trusting the address file.** ReadServerAddr at cli/internal/server/server.go:328+ returns whatever's in the file. CLI commands then dial the recorded URL. If the recorded pid is dead, dial fails. No fallback to the systemd default (127.0.0.1:7743), no warning.

## Fix

Two halves, same surface (server bootstrap + CLI client read path):

**Server-side (singleton guard):**
- On startup, server acquires flock on ~/.local/share/ddx/server.lock
- If lock is held by an alive process: refuse to start with clear error ('ddx server already running, pid=&lt;N&gt;, addr=&lt;URL&gt;; only one ddx server per machine'). Exit non-zero.
- If lock is acquired (or stale — held by dead pid): proceed; release-on-exit handler clears the lock.
- writeAddrFile happens AFTER the lock acquires — so the address file always corresponds to the lock-holder.

**Client-side (defense in depth):**
- ReadServerAddr (or its callers) validates the recorded pid is alive (kill -0 equivalent on Unix, OpenProcess on Windows)
- If pid is dead: emit warning ('server.addr points to a dead pid &lt;N&gt;; falling back to default 127.0.0.1:7743') and use the default
- Optionally clear the stale file (best-effort; don't fail if unwritable)

## Out of scope

- Multi-server-per-machine support (the singleton guard is intentional — one server per machine is the design per FEAT-020)
- Federation: hub/spoke topology (Story 14) is multi-node, NOT multi-server-per-node
    </description>
    <acceptance>
1. Server acquires flock on ~/.local/share/ddx/server.lock at startup. 2. Second 'ddx server' launch refuses with clear error message naming the existing pid + addr; exits non-zero. 3. Lock release on graceful shutdown (SIGTERM); stale-lock detection via dead-pid check on next startup. 4. writeAddrFile only runs after lock acquisition (file is consistent with lock holder). 5. ReadServerAddr (or its callers) validates recorded pid is alive before connecting. 6. If pid is dead, CLI emits a warning and falls back to default 127.0.0.1:7743. 7. New tests in cli/internal/server/: TestServer_SingletonGuardRefusesSecondLaunch + TestServer_StaleAddressFallbackWithWarning. 8. Manual verification: kill the live server; launch a new one; address-file matches the new server. Launch a second 'ddx server' while first is running; second refuses. 'ddx server workers list' succeeds against the live server after this fix.
    </acceptance>
    <labels>phase:2, area:server, kind:bug, observed-failure</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T191634-fc858450/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="a282d05f4950d2e5520174aadab92096628bc38f">
diff --git a/.ddx/executions/20260502T191634-fc858450/result.json b/.ddx/executions/20260502T191634-fc858450/result.json
new file mode 100644
index 00000000..f75795d4
--- /dev/null
+++ b/.ddx/executions/20260502T191634-fc858450/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-61357e13",
+  "attempt_id": "20260502T191634-fc858450",
+  "base_rev": "f55074ac001b0819f33fb7221886e7f35c58e0d6",
+  "result_rev": "79605b155aa1ea5be9a6616b7bdb4f3c98b8c666",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-f87f2a88",
+  "duration_ms": 415194,
+  "tokens": 19730,
+  "cost_usd": 2.145058250000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T191634-fc858450",
+  "prompt_file": ".ddx/executions/20260502T191634-fc858450/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T191634-fc858450/manifest.json",
+  "result_file": ".ddx/executions/20260502T191634-fc858450/result.json",
+  "usage_file": ".ddx/executions/20260502T191634-fc858450/usage.json",
+  "started_at": "2026-05-02T19:16:36.166359971Z",
+  "finished_at": "2026-05-02T19:23:31.361340835Z"
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
