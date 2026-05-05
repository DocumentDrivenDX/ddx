<bead-review>
  <bead id="ddx-4fd71cf3" iter=1>
    <title>artifact-types: ArtifactTypePanel.svelte + detail-view integration + vitest</title>
    <description>
PROBLEM
No frontend component renders the ArtifactType definitions for an artifact. When an artifact matches a type definition (by prefix), the operator has no UI surface to view the associated template, prompt, and examples. The detail-view of any artifact is missing the type-aware panel.

ROOT CAUSE
- cli/internal/server/frontend/src/routes/ contains artifact detail pages but no ArtifactTypePanel.svelte component.
- The typeDefinitions resolver (ddx-9ca4b5bf, a dep) provides the data, but nothing in the frontend consumes it.
- On prefix collision (multiple type definitions matching the same prefix), no selector dropdown or ?typeDef= URL state exists.
- prefixOf utility at cli/internal/server/frontend/src/lib/artifacts/grouping.ts (from AC #10 of parent) is the expected import source.

PROPOSED FIX
- Add cli/internal/server/frontend/src/lib/ArtifactTypePanel.svelte:
  - Tabbed layout: Reference Prompt | Template | Examples tabs (labels neutral, not 'Generate from').
  - On prefix collision: selector dropdown with ?typeDef= URL round-trip.
  - Imports prefixOf from artifacts/grouping.ts (single source of truth).
- Integrate ArtifactTypePanel into the artifact detail-view page.
- Add vitest unit tests for the component.

NON-SCOPE
- The typeDefinitions GraphQL resolver (that's ddx-9ca4b5bf, a dep).
- Generating artifacts from a prompt (not in this bead; labels must say 'Reference Prompt').
    </description>
    <acceptance>
1. ArtifactTypePanel.svelte exists at cli/internal/server/frontend/src/lib/ArtifactTypePanel.svelte.
2. Artifact detail view shows ArtifactTypePanel for artifacts with matching type definitions from typeDefinitions resolver.
3. Tabbed layout: Reference Prompt / Template / Examples tabs.
4. Selector dropdown appears on prefix collision; ?typeDef= URL round-trip preserves selected type on refresh.
5. ArtifactTypePanel imports prefixOf from artifacts/grouping.ts.
6. vitest tests cover: single type definition, multi-match collision selector, ?typeDef= URL state.
7. bun run test green (vitest).
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:17, area:web, kind:feature</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T192435-3425b75e/manifest.json</file>
    <file>.ddx/executions/20260505T192435-3425b75e/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="951eae298e866b5889a5d80fd7b2be42122d21ce">
<untrusted-data>
diff --git a/.ddx/executions/20260505T192435-3425b75e/manifest.json b/.ddx/executions/20260505T192435-3425b75e/manifest.json
new file mode 100644
index 00000000..487147fe
--- /dev/null
+++ b/.ddx/executions/20260505T192435-3425b75e/manifest.json
@@ -0,0 +1,38 @@
+{
+  "attempt_id": "20260505T192435-3425b75e",
+  "bead_id": "ddx-4fd71cf3",
+  "base_rev": "5b292901f6f2f2cf072e448154db20792bc3f093",
+  "created_at": "2026-05-05T19:24:42.78474651Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-4fd71cf3",
+    "title": "artifact-types: ArtifactTypePanel.svelte + detail-view integration + vitest",
+    "description": "PROBLEM\nNo frontend component renders the ArtifactType definitions for an artifact. When an artifact matches a type definition (by prefix), the operator has no UI surface to view the associated template, prompt, and examples. The detail-view of any artifact is missing the type-aware panel.\n\nROOT CAUSE\n- cli/internal/server/frontend/src/routes/ contains artifact detail pages but no ArtifactTypePanel.svelte component.\n- The typeDefinitions resolver (ddx-9ca4b5bf, a dep) provides the data, but nothing in the frontend consumes it.\n- On prefix collision (multiple type definitions matching the same prefix), no selector dropdown or ?typeDef= URL state exists.\n- prefixOf utility at cli/internal/server/frontend/src/lib/artifacts/grouping.ts (from AC #10 of parent) is the expected import source.\n\nPROPOSED FIX\n- Add cli/internal/server/frontend/src/lib/ArtifactTypePanel.svelte:\n  - Tabbed layout: Reference Prompt | Template | Examples tabs (labels neutral, not 'Generate from').\n  - On prefix collision: selector dropdown with ?typeDef= URL round-trip.\n  - Imports prefixOf from artifacts/grouping.ts (single source of truth).\n- Integrate ArtifactTypePanel into the artifact detail-view page.\n- Add vitest unit tests for the component.\n\nNON-SCOPE\n- The typeDefinitions GraphQL resolver (that's ddx-9ca4b5bf, a dep).\n- Generating artifacts from a prompt (not in this bead; labels must say 'Reference Prompt').",
+    "acceptance": "1. ArtifactTypePanel.svelte exists at cli/internal/server/frontend/src/lib/ArtifactTypePanel.svelte.\n2. Artifact detail view shows ArtifactTypePanel for artifacts with matching type definitions from typeDefinitions resolver.\n3. Tabbed layout: Reference Prompt / Template / Examples tabs.\n4. Selector dropdown appears on prefix collision; ?typeDef= URL round-trip preserves selected type on refresh.\n5. ArtifactTypePanel imports prefixOf from artifacts/grouping.ts.\n6. vitest tests cover: single type definition, multi-match collision selector, ?typeDef= URL state.\n7. bun run test green (vitest).\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-43d67aa5",
+    "labels": [
+      "phase:2",
+      "story:17",
+      "area:web",
+      "kind:feature"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T19:24:35Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3918937",
+      "execute-loop-heartbeat-at": "2026-05-05T19:24:35.832599926Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260505T192435-3425b75e",
+    "prompt": ".ddx/executions/20260505T192435-3425b75e/prompt.md",
+    "manifest": ".ddx/executions/20260505T192435-3425b75e/manifest.json",
+    "result": ".ddx/executions/20260505T192435-3425b75e/result.json",
+    "checks": ".ddx/executions/20260505T192435-3425b75e/checks.json",
+    "usage": ".ddx/executions/20260505T192435-3425b75e/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-4fd71cf3-20260505T192435-3425b75e"
+  },
+  "prompt_sha": "6fbc91fc3e69cc869170962c5fa0279c45e371d28911570e14c254d242357562"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T192435-3425b75e/result.json b/.ddx/executions/20260505T192435-3425b75e/result.json
new file mode 100644
index 00000000..db6ba3fe
--- /dev/null
+++ b/.ddx/executions/20260505T192435-3425b75e/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-4fd71cf3",
+  "attempt_id": "20260505T192435-3425b75e",
+  "base_rev": "5b292901f6f2f2cf072e448154db20792bc3f093",
+  "result_rev": "dc0649a2a6748ca7d23d551aaa4b7cf535e82715",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-5d5e47df",
+  "duration_ms": 297480,
+  "tokens": 3818560,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T192435-3425b75e",
+  "prompt_file": ".ddx/executions/20260505T192435-3425b75e/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T192435-3425b75e/manifest.json",
+  "result_file": ".ddx/executions/20260505T192435-3425b75e/result.json",
+  "usage_file": ".ddx/executions/20260505T192435-3425b75e/usage.json",
+  "started_at": "2026-05-05T19:24:42.785163344Z",
+  "finished_at": "2026-05-05T19:29:40.265887176Z"
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
