<bead-review>
  <bead id="ddx-38f694b2" iter=1>
    <title>Add progress rendering corpus for canonical and legacy samples</title>
    <description>
Build a DDx golden corpus for rendering canonical progress and legacy fallback samples. The corpus should include representative Claude, Codex, native/Fizeau, and at least one secondary harness sample. It exists to prevent regressions like over-truncated paths, invisible turn counters, missing output summaries, and missing LLM throughput.

In-scope files:
- cli/internal/agent test fixtures and formatter/progress tests
- historical sample extraction into small sanitized fixtures

Required samples:
- Claude stream-style historical records
- Codex/Fizeau progress-style records
- Native agent records
- &lt;out ... lines&gt; or equivalent long-output summaries
- long paths including cli/internal/agent/session_log_format.go
- turn_index values 21, 22, and 23
- LLM response timing and token usage sufficient to render tok/sec

Out-of-scope:
- Storing millions of raw logs in the repository.
- Live provider execution.
    </description>
    <acceptance>
1. Sanitized fixture corpus includes Claude, Codex/Fizeau, native, long-output, long-path, turn-counter, and tok/sec samples.
2. Tests assert rendered output preserves important basenames, shows turn_index counting up, includes output bytes/lines/excerpt where present, and includes tok/sec only when calculable.
3. Tests avoid arbitrary &lt;40 character limits; normal lines target 72-80 characters with the SD-011 documented 120-character tool-command exception.
4. cd cli &amp;&amp; go test ./internal/agent -run "Test.*Progress|TestFormatSessionLogLines|Test.*Corpus" -count=1 passes.
    </acceptance>
    <labels>area:agent, area:test, area:progress, kind:quality, upstream-fizeau</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T084607-e1f68b2b/manifest.json</file>
    <file>.ddx/executions/20260506T084607-e1f68b2b/result.json</file>
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

  <diff rev="208c8c26b00816517a229263b8f8fee6348a5dc7">
<untrusted-data>
diff --git a/.ddx/executions/20260506T084607-e1f68b2b/manifest.json b/.ddx/executions/20260506T084607-e1f68b2b/manifest.json
new file mode 100644
index 00000000..97ad7f9c
--- /dev/null
+++ b/.ddx/executions/20260506T084607-e1f68b2b/manifest.json
@@ -0,0 +1,169 @@
+{
+  "attempt_id": "20260506T084607-e1f68b2b",
+  "bead_id": "ddx-38f694b2",
+  "base_rev": "3d109c10f74f280ba27f589e2ec9cd20181619ce",
+  "created_at": "2026-05-06T08:46:09.622749697Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-38f694b2",
+    "title": "Add progress rendering corpus for canonical and legacy samples",
+    "description": "Build a DDx golden corpus for rendering canonical progress and legacy fallback samples. The corpus should include representative Claude, Codex, native/Fizeau, and at least one secondary harness sample. It exists to prevent regressions like over-truncated paths, invisible turn counters, missing output summaries, and missing LLM throughput.\n\nIn-scope files:\n- cli/internal/agent test fixtures and formatter/progress tests\n- historical sample extraction into small sanitized fixtures\n\nRequired samples:\n- Claude stream-style historical records\n- Codex/Fizeau progress-style records\n- Native agent records\n- \u003cout ... lines\u003e or equivalent long-output summaries\n- long paths including cli/internal/agent/session_log_format.go\n- turn_index values 21, 22, and 23\n- LLM response timing and token usage sufficient to render tok/sec\n\nOut-of-scope:\n- Storing millions of raw logs in the repository.\n- Live provider execution.",
+    "acceptance": "1. Sanitized fixture corpus includes Claude, Codex/Fizeau, native, long-output, long-path, turn-counter, and tok/sec samples.\n2. Tests assert rendered output preserves important basenames, shows turn_index counting up, includes output bytes/lines/excerpt where present, and includes tok/sec only when calculable.\n3. Tests avoid arbitrary \u003c40 character limits; normal lines target 72-80 characters with the SD-011 documented 120-character tool-command exception.\n4. cd cli \u0026\u0026 go test ./internal/agent -run \"Test.*Progress|TestFormatSessionLogLines|Test.*Corpus\" -count=1 passes.",
+    "parent": "ddx-dda48755",
+    "labels": [
+      "area:agent",
+      "area:test",
+      "area:progress",
+      "kind:quality",
+      "upstream-fizeau"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T08:46:07Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T06:41:32.302530546Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T063525-679a998e\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2989141,\"output_tokens\":31325,\"total_tokens\":3020466,\"cost_usd\":0,\"duration_ms\":364318,\"exit_code\":0}",
+          "created_at": "2026-05-06T06:41:32.535422811Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=3020466 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T06:41:39.492428812Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=8ff57d1d7e9e7cdf37a3fa83d254c864e69d9e35\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T02:46:45-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=14786\noutput_bytes=0\nelapsed_ms=4126",
+          "created_at": "2026-05-06T06:41:45.245587857Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=8ff57d1d7e9e7cdf37a3fa83d254c864e69d9e35\nbase_rev=30235c96584d05caf02df4ee7d916c09bca4c00c",
+          "created_at": "2026-05-06T06:41:45.462955135Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T06:53:20.195106035Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T064618-ba305e61\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":3900969,\"output_tokens\":19088,\"total_tokens\":3920057,\"cost_usd\":0,\"duration_ms\":419236,\"exit_code\":0}",
+          "created_at": "2026-05-06T06:53:20.419353724Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=3920057 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T06:53:26.889157191Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=2a182db7272712e1a760e6aec80dc6766a3b9fb7\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T02:58:32-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=17103\noutput_bytes=0\nelapsed_ms=4178",
+          "created_at": "2026-05-06T06:53:32.149907411Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=2a182db7272712e1a760e6aec80dc6766a3b9fb7\nbase_rev=85938eaa32efdc40168a2521109402c2156714dc",
+          "created_at": "2026-05-06T06:53:32.371019186Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T07:59:35.415733256Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T075455-d2c52778\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2588912,\"output_tokens\":12984,\"total_tokens\":2601896,\"cost_usd\":0,\"duration_ms\":277690,\"exit_code\":0}",
+          "created_at": "2026-05-06T07:59:35.655180292Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2601896 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T07:59:41.959772653Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=0fd4303c5c2c7adf0ccf8c6f9f8365e9132cbd9e\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T04:04:47-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=19410\noutput_bytes=0\nelapsed_ms=4167",
+          "created_at": "2026-05-06T07:59:47.263158113Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=0fd4303c5c2c7adf0ccf8c6f9f8365e9132cbd9e\nbase_rev=8433b7cc43f254e16e1cb7d26389d7d29de8aebd",
+          "created_at": "2026-05-06T07:59:47.671783401Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T08:46:07.178089309Z",
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
+    "dir": ".ddx/executions/20260506T084607-e1f68b2b",
+    "prompt": ".ddx/executions/20260506T084607-e1f68b2b/prompt.md",
+    "manifest": ".ddx/executions/20260506T084607-e1f68b2b/manifest.json",
+    "result": ".ddx/executions/20260506T084607-e1f68b2b/result.json",
+    "checks": ".ddx/executions/20260506T084607-e1f68b2b/checks.json",
+    "usage": ".ddx/executions/20260506T084607-e1f68b2b/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-38f694b2-20260506T084607-e1f68b2b"
+  },
+  "prompt_sha": "48f8cb7087224c483a8a2df093897fcdb7bd81e50c92600cfa6379b076422fb9"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T084607-e1f68b2b/result.json b/.ddx/executions/20260506T084607-e1f68b2b/result.json
new file mode 100644
index 00000000..d88c6235
--- /dev/null
+++ b/.ddx/executions/20260506T084607-e1f68b2b/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-38f694b2",
+  "attempt_id": "20260506T084607-e1f68b2b",
+  "base_rev": "3d109c10f74f280ba27f589e2ec9cd20181619ce",
+  "result_rev": "fe571380899d16b833760fc7b6cb356a37d73627",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-4810d39d",
+  "duration_ms": 81395,
+  "tokens": 608770,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T084607-e1f68b2b",
+  "prompt_file": ".ddx/executions/20260506T084607-e1f68b2b/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T084607-e1f68b2b/manifest.json",
+  "result_file": ".ddx/executions/20260506T084607-e1f68b2b/result.json",
+  "usage_file": ".ddx/executions/20260506T084607-e1f68b2b/usage.json",
+  "started_at": "2026-05-06T08:46:09.623150363Z",
+  "finished_at": "2026-05-06T08:47:31.018804623Z"
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
