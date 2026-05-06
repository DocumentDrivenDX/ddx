<bead-review>
  <bead id="ddx-c96fc86c" iter=1>
    <title>checks: backfill production-reachability — internal/persona (16 unreached)</title>
    <description>
Decomposed from REACH-BACKFILL (ddx-83440482). The Go production-reachability check (library/checks/go-production-reachability) flagged 16 symbol(s) in package `internal/persona` as unreachable from cli/ entry roots (deadcode RTA).

Symbols:
- internal/persona/binding.go:18 — NewBindingManager
- internal/persona/claude.go:17 — NewClaudeInjector
- internal/persona/claude.go:24 — NewClaudeInjectorWithPath
- internal/persona/claude.go:31 — ClaudeInjectorImpl.InjectPersona
- internal/persona/claude.go:50 — ClaudeInjectorImpl.InjectMultiple
- internal/persona/claude.go:96 — ClaudeInjectorImpl.RemovePersonas
- internal/persona/claude.go:114 — ClaudeInjectorImpl.GetLoadedPersonas
- internal/persona/claude.go:129 — ClaudeInjectorImpl.removePersonasSection
- internal/persona/claude.go:169 — ClaudeInjectorImpl.buildPersonasSection
- internal/persona/claude.go:199 — ClaudeInjectorImpl.extractRolePersonaPairs
- internal/persona/claude.go:236 — formatRoleDisplay
- internal/persona/claude.go:261 — ClaudeInjectorImpl.saveClaudeFile
- internal/persona/claude.go:284 — ClaudeInjectorImpl.getExistingPersonas
- internal/persona/claude.go:370 — formatRoleFromDisplay
- internal/persona/loader.go:44 — NewPersonaLoaderWithDir
- internal/persona/loader.go:52 — NewPersonaLoaderWithDirs

For each: WIRE (add to production call graph) or DELETE (genuinely obsolete). If neither is clear within ~15 min, annotate `// wiring:pending &lt;follow-up-bead-id&gt;` and file a follow-up bead.

Decision rule: if the originating bead's AC describes desired runtime behavior → WIRE; if speculative or design has moved on → DELETE.

Initial-violations evidence: .ddx/executions/20260503T124553-282667f7/initial-violations.json
    </description>
    <acceptance>
1. Each of the 16 listed symbols is either wired into the production call graph reachable from main() OR deleted (with originating bead reopened/closed-as-obsolete as appropriate).
2. Any remaining wiring:pending annotations cite open follow-up beads.
3. After landing: deadcode RTA (`go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from cli/) reports zero remaining dead symbols in `internal/persona`.
4. cd cli &amp;&amp; go test ./... green.
5. Decisions log written to .ddx/executions/&lt;run-id&gt;/decisions.md (one line per symbol: WIRE|DELETE|PENDING &lt;reason&gt;).
    </acceptance>
    <labels>phase:2, area:checks, kind:backfill</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T171059-330b6d6f/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="80aadf9c0f62429e1cc0a845164ebaeacb003adb">
<untrusted-data>
diff --git a/.ddx/executions/20260506T171059-330b6d6f/result.json b/.ddx/executions/20260506T171059-330b6d6f/result.json
new file mode 100644
index 000000000..7a98f15f4
--- /dev/null
+++ b/.ddx/executions/20260506T171059-330b6d6f/result.json
@@ -0,0 +1,24 @@
+{
+  "bead_id": "ddx-c96fc86c",
+  "attempt_id": "20260506T171059-330b6d6f",
+  "base_rev": "935a1650b215c89d364f6eba4905e551916eafc4",
+  "result_rev": "1a13dd1f560ab6c79b600f8b3b9d2f35279fbab5",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-8ae4e663",
+  "duration_ms": 172677,
+  "tokens": 1782248,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T171059-330b6d6f",
+  "prompt_file": ".ddx/executions/20260506T171059-330b6d6f/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T171059-330b6d6f/manifest.json",
+  "result_file": ".ddx/executions/20260506T171059-330b6d6f/result.json",
+  "usage_file": ".ddx/executions/20260506T171059-330b6d6f/usage.json",
+  "started_at": "2026-05-06T17:11:03.673736779Z",
+  "finished_at": "2026-05-06T17:13:56.350907758Z"
+}
\ No newline at end of file
</untrusted-data>
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
