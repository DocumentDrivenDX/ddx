<bead-review>
  <bead id="ddx-5930ed71" iter=1>
    <title>execution-bundle archive: configurable mirror for .ddx/executions/ (including embedded traces)</title>
    <description>
Execution bundles under .ddx/executions/&lt;attempt&gt;/ are the primary forensic record of what DDx automation did, but the most valuable part — the per-iteration agent trace at embedded/agent-*.jsonl — cannot reasonably be checked in. Individual traces can exceed 50 MB; a busy automation day produces hundreds of MB. Axon hit this on 2026-04-15 when GitHub warned about a 50.18 MB embedded trace file, and embedded/ is now gitignored per-repo.

We still want that level of detail for *every* run, for post-mortems, routing/model analysis, and training-data curation. We need a configurable out-of-band mirror.

## Proposed shape

1. **Archive target** configured via `.ddx/config.yaml` (and/or env var):
   - `executions.mirror.kind: local | s3 | gcs | http`
   - `executions.mirror.path: s3://bucket/ddx-executions/{project}/{attempt_id}/` (template with `{project}`, `{attempt_id}`, `{date}`, `{bead_id}`)
   - `executions.mirror.include: [manifest, prompt, result, usage, checks, embedded]` (allow excluding embedded for bandwidth-sensitive setups, but default = everything)
   - `executions.mirror.async: true` (upload in background after execute-bead returns so it never blocks the hot path)

2. **When to mirror**: at execute-bead finalization, after result.json is written. On failure, retry async with bounded attempts; log failures to `.ddx/agent-logs/mirror.log` but do not fail the bead.

3. **Index**: each mirrored bundle gets a pointer row appended to a local `.ddx/executions/mirror-index.jsonl` (attempt_id, bead_id, mirror_uri, uploaded_at, byte_size). This gives analysts a quick `jq` lookup without walking the remote store.

4. **Retrieval**: `ddx agent executions fetch &lt;attempt_id&gt;` pulls a bundle back to .ddx/executions/&lt;attempt_id&gt;/ for local inspection.

5. **GC**: optional local retention policy (`executions.retain_days`) that prunes old local bundles while the mirror keeps the full history.

## Non-goals

- Not a full audit trail service — just a mirror of DDx's existing bundles.
- Not a replacement for git-tracked manifest/prompt/result/usage summaries — those stay checked in.
- Not a distributed log aggregator — one-shot copy per bundle, no streaming.

## Why now

Axon's ADR-018 session produced ~85 MB of execution artifacts in a single day; only 1.5 MB of that is useful as checked-in summary. Without a mirror, every project using DDx execute-bead faces the same tension: either gitignore the traces and lose the forensic data, or commit them and bloat the repo forever.
    </description>
    <acceptance>
- [ ] Schema added to .ddx/config.yaml for executions.mirror (kind, path, include, async)
- [ ] At least one mirror backend implemented (local-dir is the minimum; s3 preferred)
- [ ] execute-bead finalization triggers an async mirror upload that does not block bead completion
- [ ] mirror-index.jsonl appended with attempt_id, bead_id, mirror_uri, bytes, timestamp
- [ ] Mirror failures logged but do not fail the bead
- [ ] `ddx agent executions fetch &lt;attempt_id&gt;` retrieves a mirrored bundle back to local disk
- [ ] Path template supports {project}, {attempt_id}, {date}, {bead_id}
- [ ] Default config includes the full bundle (embedded included)
- [ ] Integration test: local-dir mirror roundtrip (run execute-bead stub, assert bundle mirrored, fetch it back, bytes match)
- [ ] Docs: update TD-010 (or add TD-011) with the mirror design and operator runbook
    </acceptance>
    <labels>ddx, area:agent, area:observability, area:storage, kind:feature</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="d810ddff35013426b0e3ec92f4e1ad1fcb45c777">
commit d810ddff35013426b0e3ec92f4e1ad1fcb45c777
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Sat Apr 18 02:37:24 2026 -0400

    chore: add execution evidence [20260418T061717-]

diff --git a/.ddx/executions/20260418T061717-1993d293/manifest.json b/.ddx/executions/20260418T061717-1993d293/manifest.json
new file mode 100644
index 00000000..c2b10f62
--- /dev/null
+++ b/.ddx/executions/20260418T061717-1993d293/manifest.json
@@ -0,0 +1,59 @@
+{
+  "attempt_id": "20260418T061717-1993d293",
+  "bead_id": "ddx-5930ed71",
+  "base_rev": "510f3728eae2be2750cb26d4ef1eaa3d9238789e",
+  "created_at": "2026-04-18T06:17:17.517985173Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-5930ed71",
+    "title": "execution-bundle archive: configurable mirror for .ddx/executions/ (including embedded traces)",
+    "description": "Execution bundles under .ddx/executions/\u003cattempt\u003e/ are the primary forensic record of what DDx automation did, but the most valuable part — the per-iteration agent trace at embedded/agent-*.jsonl — cannot reasonably be checked in. Individual traces can exceed 50 MB; a busy automation day produces hundreds of MB. Axon hit this on 2026-04-15 when GitHub warned about a 50.18 MB embedded trace file, and embedded/ is now gitignored per-repo.\n\nWe still want that level of detail for *every* run, for post-mortems, routing/model analysis, and training-data curation. We need a configurable out-of-band mirror.\n\n## Proposed shape\n\n1. **Archive target** configured via `.ddx/config.yaml` (and/or env var):\n   - `executions.mirror.kind: local | s3 | gcs | http`\n   - `executions.mirror.path: s3://bucket/ddx-executions/{project}/{attempt_id}/` (template with `{project}`, `{attempt_id}`, `{date}`, `{bead_id}`)\n   - `executions.mirror.include: [manifest, prompt, result, usage, checks, embedded]` (allow excluding embedded for bandwidth-sensitive setups, but default = everything)\n   - `executions.mirror.async: true` (upload in background after execute-bead returns so it never blocks the hot path)\n\n2. **When to mirror**: at execute-bead finalization, after result.json is written. On failure, retry async with bounded attempts; log failures to `.ddx/agent-logs/mirror.log` but do not fail the bead.\n\n3. **Index**: each mirrored bundle gets a pointer row appended to a local `.ddx/executions/mirror-index.jsonl` (attempt_id, bead_id, mirror_uri, uploaded_at, byte_size). This gives analysts a quick `jq` lookup without walking the remote store.\n\n4. **Retrieval**: `ddx agent executions fetch \u003cattempt_id\u003e` pulls a bundle back to .ddx/executions/\u003cattempt_id\u003e/ for local inspection.\n\n5. **GC**: optional local retention policy (`executions.retain_days`) that prunes old local bundles while the mirror keeps the full history.\n\n## Non-goals\n\n- Not a full audit trail service — just a mirror of DDx's existing bundles.\n- Not a replacement for git-tracked manifest/prompt/result/usage summaries — those stay checked in.\n- Not a distributed log aggregator — one-shot copy per bundle, no streaming.\n\n## Why now\n\nAxon's ADR-018 session produced ~85 MB of execution artifacts in a single day; only 1.5 MB of that is useful as checked-in summary. Without a mirror, every project using DDx execute-bead faces the same tension: either gitignore the traces and lose the forensic data, or commit them and bloat the repo forever.",
+    "acceptance": "- [ ] Schema added to .ddx/config.yaml for executions.mirror (kind, path, include, async)\n- [ ] At least one mirror backend implemented (local-dir is the minimum; s3 preferred)\n- [ ] execute-bead finalization triggers an async mirror upload that does not block bead completion\n- [ ] mirror-index.jsonl appended with attempt_id, bead_id, mirror_uri, bytes, timestamp\n- [ ] Mirror failures logged but do not fail the bead\n- [ ] `ddx agent executions fetch \u003cattempt_id\u003e` retrieves a mirrored bundle back to local disk\n- [ ] Path template supports {project}, {attempt_id}, {date}, {bead_id}\n- [ ] Default config includes the full bundle (embedded included)\n- [ ] Integration test: local-dir mirror roundtrip (run execute-bead stub, assert bundle mirrored, fetch it back, bytes match)\n- [ ] Docs: update TD-010 (or add TD-011) with the mirror design and operator runbook",
+    "labels": [
+      "ddx",
+      "area:agent",
+      "area:observability",
+      "area:storage",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-18T06:17:17Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1833233",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"qwen3.6-35b-a3b\",\"fallback_chain\":[]}",
+          "created_at": "2026-04-17T18:23:42.944017231Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=qwen3.6-35b-a3b"
+        },
+        {
+          "actor": "ddx",
+          "body": "execution_failed\nresult_rev=a90dfb2f6a7a2f167e7b6881c020c97f5b6e6998\nbase_rev=a90dfb2f6a7a2f167e7b6881c020c97f5b6e6998\nretry_after=2026-04-18T00:23:43Z",
+          "created_at": "2026-04-17T18:23:43.141253108Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-04-18T06:17:17.185296993Z",
+      "execute-loop-last-detail": "execution_failed",
+      "execute-loop-last-status": "execution_failed",
+      "execute-loop-retry-after": ""
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260418T061717-1993d293",
+    "prompt": ".ddx/executions/20260418T061717-1993d293/prompt.md",
+    "manifest": ".ddx/executions/20260418T061717-1993d293/manifest.json",
+    "result": ".ddx/executions/20260418T061717-1993d293/result.json",
+    "checks": ".ddx/executions/20260418T061717-1993d293/checks.json",
+    "usage": ".ddx/executions/20260418T061717-1993d293/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-5930ed71-20260418T061717-1993d293"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260418T061717-1993d293/result.json b/.ddx/executions/20260418T061717-1993d293/result.json
new file mode 100644
index 00000000..c7682d60
--- /dev/null
+++ b/.ddx/executions/20260418T061717-1993d293/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-5930ed71",
+  "attempt_id": "20260418T061717-1993d293",
+  "base_rev": "510f3728eae2be2750cb26d4ef1eaa3d9238789e",
+  "result_rev": "09a278239ddd9234b22aa0824d2e2839691fdd65",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-312c9be3",
+  "duration_ms": 1205930,
+  "tokens": 47825,
+  "cost_usd": 9.055026500000002,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260418T061717-1993d293",
+  "prompt_file": ".ddx/executions/20260418T061717-1993d293/prompt.md",
+  "manifest_file": ".ddx/executions/20260418T061717-1993d293/manifest.json",
+  "result_file": ".ddx/executions/20260418T061717-1993d293/result.json",
+  "usage_file": ".ddx/executions/20260418T061717-1993d293/usage.json",
+  "started_at": "2026-04-18T06:17:17.518309589Z",
+  "finished_at": "2026-04-18T06:37:23.449116676Z"
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
## Review: ddx-5930ed71 iter 1

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
