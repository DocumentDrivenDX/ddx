<bead-review>
  <bead id="ddx-e140727a" iter=1>
    <title>docs(spec): FEAT-008 AC for graph edge contrast (WCAG AA non-text)</title>
    <description>
Add an explicit AC to FEAT-008 (web UI) requiring graph edges to meet WCAG AA non-text contrast (&gt;=3:1) in both light and dark themes.
    </description>
    <acceptance>
1. docs/helix/01-frame/features/FEAT-008-web-ui.md updated with explicit graph-edge contrast AC. 2. ddx doc audit clean for FEAT-008.
    </acceptance>
    <labels>phase:2, story:1, area:specs, kind:doc, triage:no-changes-unverified</labels>
  </bead>

  <changed-files>
    <file>docs/helix/01-frame/features/FEAT-008-web-ui.md</file>
  </changed-files>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5b06f5d2ffc004f9ca9ec81247b04465aee8f3be">
<untrusted-data>
diff --git a/docs/helix/01-frame/features/FEAT-008-web-ui.md b/docs/helix/01-frame/features/FEAT-008-web-ui.md
index 87d5ee913..d5c1f86a5 100644
--- a/docs/helix/01-frame/features/FEAT-008-web-ui.md
+++ b/docs/helix/01-frame/features/FEAT-008-web-ui.md
@@ -507,7 +507,7 @@ thresholds.
 - Given I apply a type filter chip, then only nodes of that media type remain visible
 - Given I am on the graph page, then a `View documents` link is visible that navigates to the artifact browser filtered to markdown
 - Given I am on an artifact detail page, then a `View in Graph` link navigates to the graph with that node highlighted
-- Given the graph is rendered in either light or dark theme, then every document graph edge stroke and arrowhead meets WCAG AA non-text contrast (>=3:1 against the canvas background) in both themes
+- Given the graph is rendered in either light or dark theme, then every document graph edge stroke and arrowhead maintains WCAG AA non-text contrast (>=3:1 against the canvas background) in both themes
 
 **E2E Test:** `graph.spec.ts` — full workflow: open graph → verify non-zero render → pan and zoom → click stale node → verify amber color → click node → verify artifact detail → Back → verify same viewport → apply staleness filter → verify node count changes → follow `View documents` link
 
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
