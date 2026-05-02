<bead-review>
  <bead id="ddx-cc90cee5" iter=1>
    <title>Story 6 B4a: TD-NNN artifact search semantics + size caps + binary skip + deterministic ordering + perf bench fixture</title>
    <description>
Design + measurement bead. Strong-tier work. Depends on B1.

Deliverables:
- Extend TD-NNN (created/owned with B3, but the search-semantics section may land first via this bead) with: search field precedence (title -&gt; path -&gt; description -&gt; frontmatter -&gt; body), per-file size cap (target 256 KB), binary-file skip rule (detect via content sniff or extension allowlist), deterministic ordering when scores tie ((sortKey, id)), and the upgrade-trigger threshold for moving to bleve/sqlite-FTS.
- Add Go benchmark in cli/internal/server/graphql/ that builds a 500-artifact fixture (mix of textual + binary, varied sizes) and measures p95 latency for full-text content search. Document p95 budget in the TD.
- Do NOT implement the resolver-level body search in this bead; that is B4b. This bead lands the design + benchmark harness only.
    </description>
    <acceptance>
- TD-NNN search-semantics section committed with explicit field precedence, size cap, binary skip, ordering, and upgrade threshold.
- Benchmark file (e.g. resolver_artifacts_bench_test.go) compiles, runs, and reports p95 against 500-artifact fixture.
- Documented p95 budget in TD-NNN matches measured baseline (or explains delta).
- 'cd cli &amp;&amp; go test -bench=. -run=^$ ./internal/server/graphql/...' produces output.
    </acceptance>
    <labels>phase:2,  story:6</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260502T143922-9c85f0d5/manifest.json</file>
    <file>.ddx/executions/20260502T143922-9c85f0d5/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="230994a2ce0e8fe63733a7f30610b513083419ab">
diff --git a/.ddx/executions/20260502T143922-9c85f0d5/manifest.json b/.ddx/executions/20260502T143922-9c85f0d5/manifest.json
new file mode 100644
index 00000000..da43cacb
--- /dev/null
+++ b/.ddx/executions/20260502T143922-9c85f0d5/manifest.json
@@ -0,0 +1,36 @@
+{
+  "attempt_id": "20260502T143922-9c85f0d5",
+  "bead_id": "ddx-cc90cee5",
+  "base_rev": "a0a96fd63d3ccf2bbf5e6fb5a8bafdeef20d91fc",
+  "created_at": "2026-05-02T14:39:23.510448285Z",
+  "requested": {
+    "harness": "claude",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-cc90cee5",
+    "title": "Story 6 B4a: TD-NNN artifact search semantics + size caps + binary skip + deterministic ordering + perf bench fixture",
+    "description": "Design + measurement bead. Strong-tier work. Depends on B1.\n\nDeliverables:\n- Extend TD-NNN (created/owned with B3, but the search-semantics section may land first via this bead) with: search field precedence (title -\u003e path -\u003e description -\u003e frontmatter -\u003e body), per-file size cap (target 256 KB), binary-file skip rule (detect via content sniff or extension allowlist), deterministic ordering when scores tie ((sortKey, id)), and the upgrade-trigger threshold for moving to bleve/sqlite-FTS.\n- Add Go benchmark in cli/internal/server/graphql/ that builds a 500-artifact fixture (mix of textual + binary, varied sizes) and measures p95 latency for full-text content search. Document p95 budget in the TD.\n- Do NOT implement the resolver-level body search in this bead; that is B4b. This bead lands the design + benchmark harness only.",
+    "acceptance": "- TD-NNN search-semantics section committed with explicit field precedence, size cap, binary skip, ordering, and upgrade threshold.\n- Benchmark file (e.g. resolver_artifacts_bench_test.go) compiles, runs, and reports p95 against 500-artifact fixture.\n- Documented p95 budget in TD-NNN matches measured baseline (or explains delta).\n- 'cd cli \u0026\u0026 go test -bench=. -run=^$ ./internal/server/graphql/...' produces output.",
+    "parent": "ddx-4728ae0f",
+    "labels": [
+      "phase:2",
+      " story:6"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-02T14:39:22Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1724970",
+      "execute-loop-heartbeat-at": "2026-05-02T14:39:22.201505118Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260502T143922-9c85f0d5",
+    "prompt": ".ddx/executions/20260502T143922-9c85f0d5/prompt.md",
+    "manifest": ".ddx/executions/20260502T143922-9c85f0d5/manifest.json",
+    "result": ".ddx/executions/20260502T143922-9c85f0d5/result.json",
+    "checks": ".ddx/executions/20260502T143922-9c85f0d5/checks.json",
+    "usage": ".ddx/executions/20260502T143922-9c85f0d5/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-cc90cee5-20260502T143922-9c85f0d5"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260502T143922-9c85f0d5/result.json b/.ddx/executions/20260502T143922-9c85f0d5/result.json
new file mode 100644
index 00000000..cb98ed02
--- /dev/null
+++ b/.ddx/executions/20260502T143922-9c85f0d5/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-cc90cee5",
+  "attempt_id": "20260502T143922-9c85f0d5",
+  "base_rev": "a0a96fd63d3ccf2bbf5e6fb5a8bafdeef20d91fc",
+  "result_rev": "bc1177255ef4c2b776051ebbcdf9637f54aec60d",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-a6dd492c",
+  "duration_ms": 303708,
+  "tokens": 14978,
+  "cost_usd": 1.5936332500000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260502T143922-9c85f0d5",
+  "prompt_file": ".ddx/executions/20260502T143922-9c85f0d5/prompt.md",
+  "manifest_file": ".ddx/executions/20260502T143922-9c85f0d5/manifest.json",
+  "result_file": ".ddx/executions/20260502T143922-9c85f0d5/result.json",
+  "usage_file": ".ddx/executions/20260502T143922-9c85f0d5/usage.json",
+  "started_at": "2026-05-02T14:39:23.510745034Z",
+  "finished_at": "2026-05-02T14:44:27.21950219Z"
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
