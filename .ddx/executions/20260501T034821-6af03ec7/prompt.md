<bead-review>
  <bead id="ddx-db06411c" iter=1>
    <title>website: wire DESIGN.md lever/load/fulcrum tokens to Hugo via assets/css/custom.css</title>
    <description/>
    <acceptance>
Hugo site uses CSS variables from DESIGN.md (#F4EFE6 canvas, #3B5B7A accent-lever, Inter/Newsreader/Space Grotesk fonts, 0px border-radius); homepage visual matches design language spec
    </acceptance>
    <labels>area:website, design-tokens</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="5cfa757c8fd4796b463b7ee4434041dd428e3b94">
commit 5cfa757c8fd4796b463b7ee4434041dd428e3b94
Merge: 51374ae7 9a1e45fe
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Thu Apr 30 23:48:18 2026 -0400

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
