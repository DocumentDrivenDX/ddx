<bead-review>
  <bead id="ddx-9e0eff22" iter=1>
    <title>Personas UI is read-only with no explanation: decide library-vs-project policy and add lifecycle UX</title>
    <description>
## Observed

`/nodes/.../projects/.../personas` is a well-crafted browser + bind UX, but it is read-only: no add, no edit, no delete, no explanation of what the reader is looking at or why they cannot change it. A user looking at it can reasonably ask "what is this for?" and "why can't I change anything?" and the UI answers neither.

## Ground truth

- **What a persona is.** A markdown file with frontmatter under `library/personas/*.md` defining AI personality, expertise, principles, communication style. Shipped defaults: architect, code-reviewer, implementer, specification-enforcer, test-engineer.
- **How it's used.** Projects declare role→persona bindings in `.ddx/config.yaml` via `persona_bindings`. When an agent performs role X, the bound persona's body is injected into its system prompt.
- **What the tool exposes today.**
  - CLI: `ddx persona --list`, `--show &lt;name&gt;`, `--bind &lt;persona&gt; --role &lt;role&gt;`. No create/edit/delete.
  - GraphQL: `personaBind` mutation only (`cli/internal/server/graphql/resolver_feat008.go:150`). No `personaCreate/Update/Delete` in schema.
  - UI: list/show/bind parity with CLI.
- **Why it evolved this way.** Personas have been treated as *library content* — authored carefully, version-controlled upstream, pulled into projects via `ddx update` and contributed back via `ddx contribute`. The tool deliberately did not become a persona editor.

## The gap

The library-only model doesn't hold up for real use:

1. **Project-local personas are a real need.** Teams want "our code-reviewer, with our opinions about our codebase." Today they either (a) fork the persona file and push upstream (wrong — it's not generally useful), (b) never customize (bad outcomes), or (c) drop a file into some ad-hoc local path and hope it works.
2. **The UI implies capability it doesn't have.** A list + detail view with no hint about source policy reads as "unfinished feature" rather than "intentional read-only".
3. **Binding alone isn't enough.** Even if editing stays out of scope, users need to understand *which personas are available*, *where they come from* (library vs project-local), and *how to change that*.

## Proposed direction

Two decisions (pick before implementation):

### D1 — Policy: who owns persona content?

Recommend: **both, with a clear boundary.**
- Library personas: `library/personas/*.md` — read-only in UI, contribute upstream via `ddx contribute`.
- Project-local personas: new directory `.ddx/personas/*.md` — fully editable in UI, live with the project, not in the shared library. Override semantics: a project-local persona with the same name as a library persona takes precedence for that project's bindings.

### D2 — Scope of the UI

Minimum viable, given D1:
- **Explainer copy** on the personas page header: "Personas are AI personality templates. Library personas are shared; project personas live with this project. Bind a persona to a role in `.ddx/config.yaml`."
- **Source badge** on each persona row: `library` or `project`.
- **Project-local editor.** For project personas:
  - `New persona` button → form (name, roles/tags, body markdown). Creates `.ddx/personas/&lt;name&gt;.md` with frontmatter.
  - `Edit` on project rows.
  - `Delete` on project rows with confirm.
- **Library rows stay read-only.** Action on a library row: `Fork to project` — copies the library file to `.ddx/personas/&lt;name&gt;.md` (with a default rename suggestion on collision) and opens the editor. No in-place edit, no delete.
- **Bind affordance remains.** Binding UI continues to work for both sources.

## What this bead does NOT decide

- Multi-project persona sharing (e.g., org-level personas between library and project). File a follow-up if it becomes pressing.
- Rich editor affordances beyond a plain markdown textarea with preview. Frontmatter can be a YAML-ish textarea for v1.
- Syncing project personas upstream — that remains `ddx contribute` and is out of scope.
- Any change to how personas are injected into agent prompts (the read path on the server).

## CLI parity

Whatever the UI gains, the CLI gains too. Add:
- `ddx persona new &lt;name&gt;` — creates `.ddx/personas/&lt;name&gt;.md` scaffold.
- `ddx persona edit &lt;name&gt;` — opens `$EDITOR` on the project-local persona (errors on library).
- `ddx persona fork &lt;library-name&gt; [--as &lt;new-name&gt;]` — copies library → project.
- `ddx persona delete &lt;name&gt;` — project-local only.

UI and CLI call the same underlying `cli/internal/persona` operations; no duplicate logic.
    </description>
    <acceptance>
**User story:** As a developer looking at the personas UI, I want to understand at a glance what personas are, where they come from (library vs project), and I want to fork or create a project-local persona without leaving the UI. Library content stays read-only so I cannot accidentally mutate shared content; project content is fully editable.

**Acceptance criteria:**

1. **Design note + policy decision recorded.** Before implementation, a one-page note in `docs/helix/02-design/` captures D1 (library vs project policy, override rules, file locations) and D2 (scoped UI additions). Implementation PR links to it.

2. **Project-local persona directory.** `.ddx/personas/*.md` is a recognized location. `ddx persona --list` shows both library and project personas with a `source` tag. Override rule: a project persona with the same name as a library persona wins for that project's bindings; unit test covers this precedence.

3. **GraphQL additions.** New mutations: `personaCreate(name, body, projectId) → Persona`, `personaUpdate(name, body, projectId) → Persona`, `personaDelete(name, projectId) → {ok}`. All gated on `source == "project"`; attempting any of them on a library persona returns a typed error. Each mutation persists to `.ddx/personas/&lt;name&gt;.md` under the given project.

4. **CLI parity.** `ddx persona new`, `edit`, `fork`, `delete` exist and share the same underlying operations as the GraphQL mutations (no duplicated logic). Integration test asserts that a `ddx persona new` followed by a GraphQL `personas` query returns the new persona.

5. **UI — explainer copy.** The personas page shows a one-sentence description of what personas are and a second sentence distinguishing library vs project sources. Playwright asserts both sentences are present.

6. **UI — source badge.** Each persona row shows a `library` or `project` badge. Playwright seeds one of each and asserts correct badges.

7. **UI — New / Edit / Delete for project personas.** A "New persona" button opens a form with name + body fields. Project-source rows have Edit and Delete affordances. Library-source rows have neither. Playwright creates a persona, edits it, verifies the edit persisted, then deletes it.

8. **UI — Fork library to project.** Library rows have a "Fork to project" action. Clicking it prompts for a name (default: library name with `-local` suffix on collision), creates `.ddx/personas/&lt;name&gt;.md`, and navigates to the editor. Playwright asserts the forked file exists and opens in the editor.

9. **No regressions.** Existing list/show/bind flows still work. Playwright smoke of the pre-existing binding UX passes.

10. **Empty project-local state.** When a project has zero project-local personas, the UI shows a subtle hint: "No project personas yet. Fork a library persona or create a new one." Playwright asserts.
    </acceptance>
    <labels>feat-008, personas, ui, design</labels>
  </bead>

  <governing>
    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>
  </governing>

  <diff rev="bb547ae98e023e08e2ac7265872122c3f14db74b">
commit bb547ae98e023e08e2ac7265872122c3f14db74b
Author: ddx-land-coordinator <coordinator@ddx.local>
Date:   Wed Apr 22 23:06:46 2026 -0400

    chore: add execution evidence [20260423T023822-]

diff --git a/.ddx/executions/20260423T023822-a2d45fb0/result.json b/.ddx/executions/20260423T023822-a2d45fb0/result.json
new file mode 100644
index 00000000..0a8af60a
--- /dev/null
+++ b/.ddx/executions/20260423T023822-a2d45fb0/result.json
@@ -0,0 +1,22 @@
+{
+  "bead_id": "ddx-9e0eff22",
+  "attempt_id": "20260423T023822-a2d45fb0",
+  "base_rev": "ed76c00d471e591ed0bee5511175e8a270052555",
+  "result_rev": "5e13d45c9fcdccc4dc4559d3986831715c41f04a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "session_id": "eb-2c2611c2",
+  "duration_ms": 1702680,
+  "tokens": 90805,
+  "cost_usd": 21.85301600000001,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260423T023822-a2d45fb0",
+  "prompt_file": ".ddx/executions/20260423T023822-a2d45fb0/prompt.md",
+  "manifest_file": ".ddx/executions/20260423T023822-a2d45fb0/manifest.json",
+  "result_file": ".ddx/executions/20260423T023822-a2d45fb0/result.json",
+  "usage_file": ".ddx/executions/20260423T023822-a2d45fb0/usage.json",
+  "started_at": "2026-04-23T02:38:22.813449839Z",
+  "finished_at": "2026-04-23T03:06:45.494006709Z"
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
## Review: ddx-9e0eff22 iter 1

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
