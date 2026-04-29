<bead-review>
  <bead id="ddx-33e07890" iter=1>
    <title>[artifact-run-arch] update product-vision.md for artifact + 3-layer architecture</title>
    <description>
Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md.

Scope of vision update:

(0) PREAMBLE for Core Thesis (canonical wording — use verbatim):

&gt; Software is leverage. Fifty years of practice has shown that there are physics of software that must be respected to produce and maintain systems at scale. The agentic era allows every team to produce systems at scale and makes this even more important.
&gt;
&gt; Generative AI brings its own physics. DDx exists at the seam where these two meet — the fulcrum that lets software's lever do work. Without it, shipping software with agents quietly degrades into prompt-and-pray. The six principles below are the load-bearing claims on both sides.

The preamble REPLACES the existing two-paragraph Core Thesis. Existing paragraph 2 ("Creating documentation and using it as an abstraction... DDx encodes that insight into infrastructure...") drifts into positioning rather than thesis — its substance is captured by principle #1 plus existing 'What DDx Is' section. Drop it from Core Thesis when this bead lands.

(1) Thesis principles — restructure Core Thesis around 6 principles in 3 groups (Physics of Software / Physics of Generative AI / The intersection — DDx's reason to exist). Final formulations:

A. Physics of Software (engineering truths agents amplify):
1. Abstraction is the lever. Multi-level artifact stacks (vision -&gt; spec -&gt; test -&gt; code) with maintained relationships are how intent propagates without being lost. True for human teams; load-bearing for agents because they don't carry tacit knowledge between invocations.
2. Software is iteration over tracked work. Repeated trials over an explicit work substrate — beads, queues, dependency DAGs — is how software gets built. Pre-existed agents; agents make the substrate non-optional.
3. Methodology is plural. Different teams, projects, and problem domains demand different workflows — waterfall, agile, kanban, HELIX, ad-hoc. No tool that bakes one in survives the rest. DDx provides primitives (artifacts, runs, beads) that any methodology composes.

B. Physics of Generative AI (claims about LLM behavior):
4. LLMs are stochastic, unreliable, and costly. Cost-tier ladders, ensemble verification, and 'cheapest model that works' are the operating shape of agentic work, not optimizations.
5. Evidence provides memory. Agents carry no state between invocations and outputs aren't bit-reproducible. The only thing that survives a run is what we captured as it happened. That captured evidence is the substrate for evaluation, trust, debugging, and learning — without it, every other principle degrades to anecdote.

C. The intersection — DDx's reason to exist:
6. Human-AI collaboration is the fulcrum. Abstraction levers intent across the artifact stack, but only collaboration converts leverage into shipped software. Humans supply intent and accountability; AI supplies volume and execution. DDx is the toolkit at the seam — handoffs in both directions, at every level.

Note: #1 and #6 form a deliberate rhetorical bookend (lever + fulcrum = leverage). The preamble primes the metaphor without spoiling it.

(2) Operating principles — keep existing list (Git-native, file-first, etc.) but add one-line preamble: 'Operating principles are the choices DDx makes in response to the physics above.' Update Principle #1 from 'Documents are the product' to 'Artifacts are the product (documents primary, other media supported).'

(3) Artifact + 3-layer architecture changes (per original bead scope):
- Thesis copy: 'documents AI agents consume' -&gt; 'artifacts agents produce and consume'
- Artifact-management bullet: broaden to non-document media + generators
- One sentence acknowledging four-way producer/consumer space (no separate matrix subsection)
- New Design-Philosophy subsection: Three-layer run architecture (ddx run / ddx try / ddx work)
- KVP table additions: 'Multi-media artifact graph'; 'Three-layer run architecture (run/try/work)'; note invocation is upstream

Do NOT touch docs/helix/01-frame/principles.md — that's HELIX engineering decision-guide, separate concern.
    </description>
    <acceptance/>
    <labels>frame, plan-2026-04-29</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260429T175223-fc7d9a1b/manifest.json</file>
    <file>.ddx/executions/20260429T175223-fc7d9a1b/result.json</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="0276dfa112695a0d8541d33c42fc18afd4fb2317">
diff --git a/.ddx/executions/20260429T175223-fc7d9a1b/manifest.json b/.ddx/executions/20260429T175223-fc7d9a1b/manifest.json
new file mode 100644
index 00000000..de5fad78
--- /dev/null
+++ b/.ddx/executions/20260429T175223-fc7d9a1b/manifest.json
@@ -0,0 +1,35 @@
+{
+  "attempt_id": "20260429T175223-fc7d9a1b",
+  "bead_id": "ddx-33e07890",
+  "base_rev": "255c18f9c2f3d6b01f4e9bea785b595b4a2ce026",
+  "created_at": "2026-04-29T17:52:24.576812577Z",
+  "requested": {
+    "harness": "claude",
+    "model": "sonnet",
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-33e07890",
+    "title": "[artifact-run-arch] update product-vision.md for artifact + 3-layer architecture",
+    "description": "Per plan: docs/helix/02-design/plan-2026-04-29-artifact-and-run-architecture.md.\n\nScope of vision update:\n\n(0) PREAMBLE for Core Thesis (canonical wording — use verbatim):\n\n\u003e Software is leverage. Fifty years of practice has shown that there are physics of software that must be respected to produce and maintain systems at scale. The agentic era allows every team to produce systems at scale and makes this even more important.\n\u003e\n\u003e Generative AI brings its own physics. DDx exists at the seam where these two meet — the fulcrum that lets software's lever do work. Without it, shipping software with agents quietly degrades into prompt-and-pray. The six principles below are the load-bearing claims on both sides.\n\nThe preamble REPLACES the existing two-paragraph Core Thesis. Existing paragraph 2 (\"Creating documentation and using it as an abstraction... DDx encodes that insight into infrastructure...\") drifts into positioning rather than thesis — its substance is captured by principle #1 plus existing 'What DDx Is' section. Drop it from Core Thesis when this bead lands.\n\n(1) Thesis principles — restructure Core Thesis around 6 principles in 3 groups (Physics of Software / Physics of Generative AI / The intersection — DDx's reason to exist). Final formulations:\n\nA. Physics of Software (engineering truths agents amplify):\n1. Abstraction is the lever. Multi-level artifact stacks (vision -\u003e spec -\u003e test -\u003e code) with maintained relationships are how intent propagates without being lost. True for human teams; load-bearing for agents because they don't carry tacit knowledge between invocations.\n2. Software is iteration over tracked work. Repeated trials over an explicit work substrate — beads, queues, dependency DAGs — is how software gets built. Pre-existed agents; agents make the substrate non-optional.\n3. Methodology is plural. Different teams, projects, and problem domains demand different workflows — waterfall, agile, kanban, HELIX, ad-hoc. No tool that bakes one in survives the rest. DDx provides primitives (artifacts, runs, beads) that any methodology composes.\n\nB. Physics of Generative AI (claims about LLM behavior):\n4. LLMs are stochastic, unreliable, and costly. Cost-tier ladders, ensemble verification, and 'cheapest model that works' are the operating shape of agentic work, not optimizations.\n5. Evidence provides memory. Agents carry no state between invocations and outputs aren't bit-reproducible. The only thing that survives a run is what we captured as it happened. That captured evidence is the substrate for evaluation, trust, debugging, and learning — without it, every other principle degrades to anecdote.\n\nC. The intersection — DDx's reason to exist:\n6. Human-AI collaboration is the fulcrum. Abstraction levers intent across the artifact stack, but only collaboration converts leverage into shipped software. Humans supply intent and accountability; AI supplies volume and execution. DDx is the toolkit at the seam — handoffs in both directions, at every level.\n\nNote: #1 and #6 form a deliberate rhetorical bookend (lever + fulcrum = leverage). The preamble primes the metaphor without spoiling it.\n\n(2) Operating principles — keep existing list (Git-native, file-first, etc.) but add one-line preamble: 'Operating principles are the choices DDx makes in response to the physics above.' Update Principle #1 from 'Documents are the product' to 'Artifacts are the product (documents primary, other media supported).'\n\n(3) Artifact + 3-layer architecture changes (per original bead scope):\n- Thesis copy: 'documents AI agents consume' -\u003e 'artifacts agents produce and consume'\n- Artifact-management bullet: broaden to non-document media + generators\n- One sentence acknowledging four-way producer/consumer space (no separate matrix subsection)\n- New Design-Philosophy subsection: Three-layer run architecture (ddx run / ddx try / ddx work)\n- KVP table additions: 'Multi-media artifact graph'; 'Three-layer run architecture (run/try/work)'; note invocation is upstream\n\nDo NOT touch docs/helix/01-frame/principles.md — that's HELIX engineering decision-guide, separate concern.",
+    "labels": [
+      "frame",
+      "plan-2026-04-29"
+    ],
+    "metadata": {
+      "claimed-at": "2026-04-29T17:52:21Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "932496",
+      "execute-loop-heartbeat-at": "2026-04-29T17:52:21.707469295Z"
+    }
+  },
+  "paths": {
+    "dir": ".ddx/executions/20260429T175223-fc7d9a1b",
+    "prompt": ".ddx/executions/20260429T175223-fc7d9a1b/prompt.md",
+    "manifest": ".ddx/executions/20260429T175223-fc7d9a1b/manifest.json",
+    "result": ".ddx/executions/20260429T175223-fc7d9a1b/result.json",
+    "checks": ".ddx/executions/20260429T175223-fc7d9a1b/checks.json",
+    "usage": ".ddx/executions/20260429T175223-fc7d9a1b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-33e07890-20260429T175223-fc7d9a1b"
+  }
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260429T175223-fc7d9a1b/result.json b/.ddx/executions/20260429T175223-fc7d9a1b/result.json
new file mode 100644
index 00000000..e771264c
--- /dev/null
+++ b/.ddx/executions/20260429T175223-fc7d9a1b/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-33e07890",
+  "attempt_id": "20260429T175223-fc7d9a1b",
+  "base_rev": "255c18f9c2f3d6b01f4e9bea785b595b4a2ce026",
+  "result_rev": "29df13937e430a1a14421e7b2dc413e8379d7a1a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-b3b088c2",
+  "duration_ms": 139318,
+  "tokens": 6907,
+  "cost_usd": 0.32305320000000004,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260429T175223-fc7d9a1b",
+  "prompt_file": ".ddx/executions/20260429T175223-fc7d9a1b/prompt.md",
+  "manifest_file": ".ddx/executions/20260429T175223-fc7d9a1b/manifest.json",
+  "result_file": ".ddx/executions/20260429T175223-fc7d9a1b/result.json",
+  "usage_file": ".ddx/executions/20260429T175223-fc7d9a1b/usage.json",
+  "started_at": "2026-04-29T17:52:24.577151243Z",
+  "finished_at": "2026-04-29T17:54:43.895488336Z"
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
