<bead-review>
  <bead id="ddx-b20bdc55" iter=1>
    <title>Consume Fizeau canonical progress events in worker output</title>
    <description>
PROBLEM
DDx worker and live-output paths still derive inner agent progress from Fizeau-adjacent session logs or harness-native records. After SD-011, canonical progress belongs to Fizeau ServiceEvents; DDx should consume those fields for display and keep DDx-owned worker lifecycle events separate.

ROOT CAUSE
- cli/internal/agent/session_log_format.go:83-85 formats Fizeau-style session JSONL records into DDx transcript/progress lines.
- cli/internal/server/workers.go:1655-1704 reads latest agent-*.jsonl logs and calls agent.FormatSessionLogLines for live worker output.
- cli/internal/agent/claude_stream.go:451-456 still writes Fizeau-style progress/session records from a direct parser.
- cli/internal/agent/agent_runner_service.go:243-355 drains service events and reconstructs progress/tool-call entries rather than treating canonical progress as the source of truth.

PROPOSED FIX
Use Fizeau canonical progress ServiceEvent payloads for DDx worker/live output. DDx may format DDx-owned worker lifecycle events, but inner agent action/target/timing/output excerpts must come from canonical Fizeau progress fields when present. Historical fallback handling must be explicit and not used when canonical events exist.

NON-SCOPE
- Deleting the legacy formatter/tailer; ddx-f948b7a4 handles deletion after this bridge lands.
- Parsing Claude stream-json directly.
- Fizeau schema changes.
    </description>
    <acceptance>
1. DDx worker/live output prefers Fizeau canonical progress fields from SD-011: task_id, turn_index, action, target, timing/duration_ms, tok_per_sec when present, and output bytes/lines/excerpt.
2. TestDrainServiceEvents_ForwardsCanonicalProgressPayload proves canonical Fizeau progress is forwarded/rendered without reading session-log JSONL.
3. TestWorkerProgressUsesCanonicalFizeauProgressEvents proves server worker output uses canonical ServiceEvents instead of FormatSessionLogLines when those events exist.
4. DDx-owned worker lifecycle events remain distinct from Fizeau inner progress events in tests and event names.
5. cd cli &amp;&amp; go test ./internal/agent ./internal/server -run "Test.*Service|Test.*Worker|Test.*Progress" -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:agent, area:server, area:progress, kind:cleanup, upstream-fizeau</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T234506-21cbb503/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T234506-21cbb503/manifest.json</file>
    <file>.ddx/executions/20260505T234506-21cbb503/result.json</file>
  </changed-files>

  <governing>
    <ref id="SD-011" path="docs/helix/02-design/solution-designs/SD-011-agent-skills.md" title="Solution Design: DDx Agent Skills">
      <content>
<untrusted-data>
---
ddx:
  id: SD-011
  depends_on:
    - FEAT-011
    - FEAT-001
    - FEAT-015
    - ADR-001
---
# Solution Design: DDx Agent Skills

> **Updated 2026-04-20.** FEAT-011 consolidated the earlier 4-skill layout
> (`ddx-bead`, `ddx-agent`, `ddx-install`, `ddx-status`) into a single
> `ddx` skill with an intent router and per-topic reference files.

## Overview

DDx ships a single agent-facing skill — `ddx` — that provides guidance
for operating every DDx CLI surface: beads, the queue, executions,
agents, harnesses, personas, reviews, and installation. The skill body
is an intent router; the real domain guidance lives under
`reference/*.md` files loaded on demand.

Skills are plain-Markdown guidance wrappers over DDx CLI commands. They
carry no compiled code or runtime dependencies — an agent reads the
skill and follows its instructions by invoking `ddx` CLI commands
directly.

## Skill Format

> **FEAT-015 amendment:** Skill directories are project-local under
> `<projectRoot>/.agents/skills/`. Home-directory skill paths are retired.
> The layout below uses the current project-local model.

```
.agents/skills/ddx/   # project-local (FEAT-015)
├── SKILL.md
├── evals/
│   └── routing.jsonl
└── reference/
    ├── beads.md
    ├── agents.md
    ├── executions.md
    ├── personas.md
    └── ...
```

### SKILL.md Frontmatter

The skill uses the top-level frontmatter schema enforced by
`ddx skills check` (AGENTS.md §Skill Policy):

```yaml
---
name: ddx
description: Operates the DDx toolkit for document-driven development. ...
---
```

- `name` — exactly matches the directory name (`ddx`).
- `description` — intent triggers keyed to user phrasing ("drain the
  queue", "run a bead", "create a bead", etc.). The description is
  load-bearing for router selection by skills-aware agents.
- `argument-hint` — optional; used only when the skill takes a
  trailing positional or shorthand invocation hint.
- **Nested `skill:` metadata is rejected.** The DDx skill uses
  top-level fields only.

### SKILL.md Body

The body opens with an overview and then an **intent router** — a
table mapping user phrasing to the matching `reference/<topic>.md`
file. The directive to the agent is strict: load the matching
reference file before responding to a DDx-related request.

Reference files cover:

- `reference/beads.md` — bead CRUD, dependencies, claims, evidence
- `reference/agents.md` — power-bound dispatch, passthrough constraints,
  `ddx run`, `ddx try`, and `ddx work`
- `reference/executions.md` — execution definitions and immutable run
  history (`ddx metric` / `ddx exec`)
- `reference/personas.md` — persona listing, show, binding
- `reference/install.md` — plugin and skills install flows
- additional topics as DDx surfaces grow

## Installation Mechanism

### Embedded Source

Skill source lives in `cli/internal/skills/ddx/`. The binary embeds
the tree via `//go:embed` (FEAT-011) so the skill ships with every
DDx release and never requires a separate download.

### Project-Local Install (`ddx init`)

`ddx init` writes a project-local copy into `.ddx/skills/ddx/` and
registers skill symlinks under `.agents/skills/` and `.claude/skills/`
for the two major skill runtimes. Real files are copied (not
symlinked to global) so project worktrees can evolve independently.

### Global Install (`ddx install --global`)

> **Retired (FEAT-015):** `ddx install --global` has been removed. Skills are
> installed project-locally via `ddx init`. No home-directory writes occur.

### Plugin-Declared Skills (`ddx install <plugin>`)

Plugins may declare additional skills in their `package.yaml`. The
installer materializes relative symlinks from `.agents/skills/` and
`.claude/skills/` into the plugin's skill directories and prunes
stale links from prior plugin versions (FEAT-015 AC-004 / AC-013,
tracked by `ddx-20fe27c7`).

### Manual Management

Users may edit or replace the skill files directly. `ddx init` does
not overwrite manually modified files unless `--force` is passed.

## CLI Invocation Pattern

Reference files invoke the `ddx` binary on `$PATH`. They do not
shell-expand or hard-code paths. If `ddx` is absent, the agent emits a
clear error and halts. All CLI calls use structured flags — no
positional argument guessing.

## Validation

- `ddx skills check [path ...]` validates SKILL.md frontmatter for any
  skill tree: top-level `name`, top-level `description`, optional
  `argument-hint`, rejects nested `skill:` metadata, requires a
  non-empty body.
- `make skill-schema` (at `cli/Makefile:82`) runs `ddx skills check`
  against both the canonical source (`skills/ddx`) and the embedded
  copy (`cli/internal/skills/ddx`). Pre-commit and CI both enforce
  this gate.
- Unit tests in `cli/internal/skills/` verify the embedded tree
  parses cleanly.

## Testing Strategy

- Static validation of every bundled `SKILL.md` via
  `ddx skills check`.
- Router evals: `skills/ddx/evals/routing.jsonl` contains labelled
  user phrasings and expected reference-file selections. The eval is
  the regression harness for router drift.
- Integration tests for `ddx init` assert the skill directory exists
  and contains a readable `SKILL.md` after initialization.
- No end-to-end agent execution tests — skill correctness is
  validated by inspecting the skill content and router evals, not by
  running an agent.

## Non-Goals

- Workflow-specific skills (HELIX provides those under its own
  install path; FEAT-011 stays platform-agnostic).
- Skills for commands that need no guidance (`ddx version`,
  `ddx upgrade`).
- Interactive TUI or GUI — skills are agent-facing Markdown.
- Compiled skill logic — all intelligence lives in CLI commands, not
  skill files.
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="d0a1a7ab74389124ef61d1bd519b95e6d1307b20">
<untrusted-data>
diff --git a/.ddx/executions/20260505T234506-21cbb503/checks/production-reachability.json b/.ddx/executions/20260505T234506-21cbb503/checks/production-reachability.json
new file mode 100644
index 00000000..246408be
--- /dev/null
+++ b/.ddx/executions/20260505T234506-21cbb503/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no new top-level functions or methods added"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T234506-21cbb503/manifest.json b/.ddx/executions/20260505T234506-21cbb503/manifest.json
new file mode 100644
index 00000000..7c6933eb
--- /dev/null
+++ b/.ddx/executions/20260505T234506-21cbb503/manifest.json
@@ -0,0 +1,130 @@
+{
+  "attempt_id": "20260505T234506-21cbb503",
+  "bead_id": "ddx-b20bdc55",
+  "base_rev": "8b2a728b461cddd67a05356c7c50cce5274bedd7",
+  "created_at": "2026-05-05T23:45:08.741994229Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b20bdc55",
+    "title": "Consume Fizeau canonical progress events in worker output",
+    "description": "PROBLEM\nDDx worker and live-output paths still derive inner agent progress from Fizeau-adjacent session logs or harness-native records. After SD-011, canonical progress belongs to Fizeau ServiceEvents; DDx should consume those fields for display and keep DDx-owned worker lifecycle events separate.\n\nROOT CAUSE\n- cli/internal/agent/session_log_format.go:83-85 formats Fizeau-style session JSONL records into DDx transcript/progress lines.\n- cli/internal/server/workers.go:1655-1704 reads latest agent-*.jsonl logs and calls agent.FormatSessionLogLines for live worker output.\n- cli/internal/agent/claude_stream.go:451-456 still writes Fizeau-style progress/session records from a direct parser.\n- cli/internal/agent/agent_runner_service.go:243-355 drains service events and reconstructs progress/tool-call entries rather than treating canonical progress as the source of truth.\n\nPROPOSED FIX\nUse Fizeau canonical progress ServiceEvent payloads for DDx worker/live output. DDx may format DDx-owned worker lifecycle events, but inner agent action/target/timing/output excerpts must come from canonical Fizeau progress fields when present. Historical fallback handling must be explicit and not used when canonical events exist.\n\nNON-SCOPE\n- Deleting the legacy formatter/tailer; ddx-f948b7a4 handles deletion after this bridge lands.\n- Parsing Claude stream-json directly.\n- Fizeau schema changes.",
+    "acceptance": "1. DDx worker/live output prefers Fizeau canonical progress fields from SD-011: task_id, turn_index, action, target, timing/duration_ms, tok_per_sec when present, and output bytes/lines/excerpt.\n2. TestDrainServiceEvents_ForwardsCanonicalProgressPayload proves canonical Fizeau progress is forwarded/rendered without reading session-log JSONL.\n3. TestWorkerProgressUsesCanonicalFizeauProgressEvents proves server worker output uses canonical ServiceEvents instead of FormatSessionLogLines when those events exist.\n4. DDx-owned worker lifecycle events remain distinct from Fizeau inner progress events in tests and event names.\n5. cd cli \u0026\u0026 go test ./internal/agent ./internal/server -run \"Test.*Service|Test.*Worker|Test.*Progress\" -count=1 passes.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-dda48755",
+    "labels": [
+      "phase:2",
+      "area:agent",
+      "area:server",
+      "area:progress",
+      "kind:cleanup",
+      "upstream-fizeau"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T23:45:06Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "427120",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T16:33:28.932083105Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T162215-533f3a56\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":14136827,\"output_tokens\":35263,\"total_tokens\":14172090,\"cost_usd\":0,\"duration_ms\":671128,\"exit_code\":0}",
+          "created_at": "2026-05-05T16:33:29.169065032Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=14172090 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T16:33:39.092954709Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=ed808c6bc8851deed53e884c9ffa02ba37036be4\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T12:38:44-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=16087\noutput_bytes=0\nelapsed_ms=4187",
+          "created_at": "2026-05-05T16:33:44.726893313Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "merged onto current tip\nresult_rev=ed808c6bc8851deed53e884c9ffa02ba37036be4\nbase_rev=aed89e63a8bf28da1e0656a9bb319a8cb1e48104",
+          "created_at": "2026-05-05T16:33:44.951619963Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T18:15:11.835966609Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T180431-93d6720e\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":12230576,\"output_tokens\":32052,\"total_tokens\":12262628,\"cost_usd\":0,\"duration_ms\":637626,\"exit_code\":0}",
+          "created_at": "2026-05-05T18:15:12.059940528Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=12262628 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T18:15:20.1054042Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=13d19bda04e92616bd4537abe8a6f7480c298a9c\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T14:20:25-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=18508\noutput_bytes=0\nelapsed_ms=4181",
+          "created_at": "2026-05-05T18:15:25.474766125Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=13d19bda04e92616bd4537abe8a6f7480c298a9c\nbase_rev=b306cf3077816745f6dafeb71b070b6367e2ecd5",
+          "created_at": "2026-05-05T18:15:25.696437047Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T23:45:06.620728487Z",
+      "spec-id": "SD-011"
+    }
+  },
+  "governing": [
+    {
+      "id": "SD-011",
+      "path": "docs/helix/02-design/solution-designs/SD-011-agent-skills.md",
+      "title": "Solution Design: DDx Agent Skills"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260505T234506-21cbb503",
+    "prompt": ".ddx/executions/20260505T234506-21cbb503/prompt.md",
+    "manifest": ".ddx/executions/20260505T234506-21cbb503/manifest.json",
+    "result": ".ddx/executions/20260505T234506-21cbb503/result.json",
+    "checks": ".ddx/executions/20260505T234506-21cbb503/checks.json",
+    "usage": ".ddx/executions/20260505T234506-21cbb503/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-b20bdc55-20260505T234506-21cbb503"
+  },
+  "prompt_sha": "10dc85906afcfbdd79d2a0d0a4acc31c1ee3c950666202f0603cb80c042fdcbf"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T234506-21cbb503/result.json b/.ddx/executions/20260505T234506-21cbb503/result.json
new file mode 100644
index 00000000..a3701c65
--- /dev/null
+++ b/.ddx/executions/20260505T234506-21cbb503/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-b20bdc55",
+  "attempt_id": "20260505T234506-21cbb503",
+  "base_rev": "8b2a728b461cddd67a05356c7c50cce5274bedd7",
+  "result_rev": "3dc25e354879d24e7f6bd3730c33ac8ecb7ea5c4",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-c850ba0b",
+  "duration_ms": 499708,
+  "tokens": 8939835,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T234506-21cbb503",
+  "prompt_file": ".ddx/executions/20260505T234506-21cbb503/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T234506-21cbb503/manifest.json",
+  "result_file": ".ddx/executions/20260505T234506-21cbb503/result.json",
+  "usage_file": ".ddx/executions/20260505T234506-21cbb503/usage.json",
+  "started_at": "2026-05-05T23:45:08.742465437Z",
+  "finished_at": "2026-05-05T23:53:28.451339577Z"
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
