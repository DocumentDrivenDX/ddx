# Persona Lifecycle — Library vs Project Policy + UI Scope

Date: 2026-04-22
Bead: `ddx-9e0eff22`

## Implementation map

| Area | Files |
|------|-------|
| Loader + source tracking | `cli/internal/persona/loader.go`, `cli/internal/persona/types.go` |
| Project-local CRUD | `cli/internal/persona/writer.go` |
| CLI subcommands | `cli/cmd/persona.go`, `cli/cmd/command_factory_commands.go` |
| GraphQL schema + resolvers | `cli/internal/server/graphql/schema.graphql`, `cli/internal/server/graphql/resolver_feat008.go`, `cli/internal/server/graphql/resolver_meta.go` |
| UI | `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/personas/{data.ts,PersonasView.svelte}` |
| Tests | `cli/internal/persona/project_personas_test.go`, `cli/cmd/persona_lifecycle_test.go`, `cli/internal/server/graphql/persona_lifecycle_test.go`, `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/personas/personas.e2e.ts` |

## Problem

The personas page is read-only with no explanation of why. A reader cannot tell
whether the feature is unfinished or intentional, and teams who want a
project-flavored persona ("our code-reviewer, with our opinions") have no
sanctioned path: they can fork the library file and push it upstream
(inappropriate), never customize (bad outcomes), or scatter ad-hoc files.

## D1 — Policy: library vs project content

Personas can live in two places:

- **Library personas** — `<library>/personas/*.md`. Shipped with DDx, shared
  across projects, pulled via `ddx update`, contributed upstream via
  `ddx contribute`. **Read-only in UI and API.** Edits are refused with a
  typed error; the only mutation affecting library content is `fork`, which
  copies the file into the project.

- **Project-local personas** — `.ddx/personas/*.md` under the project root.
  Live with the project in its VCS. Fully editable via UI, GraphQL, and CLI.
  Never pushed upstream by `ddx contribute`.

### Override rule

When a project-local persona has the same name as a library persona, the
project-local file wins for that project's bindings and list views. The
library version is still visible in the list with its own source badge so
the reader can see what was overridden. Load order: project wins on name
collision; both appear in `ListPersonas` with correct source labels.

### What this does not decide

- Multi-project / org-level persona sharing — out of scope.
- Rich in-browser markdown editor — a plain `<textarea>` is enough for v1.
- Upstreaming project personas — still happens via `ddx contribute`.
- Changing how personas get injected into agent prompts — server read path
  is unchanged.

## D2 — UI scope

Minimum viable additions to
`/nodes/.../projects/.../personas`:

1. **Explainer copy** in the page header. Two sentences: what personas are,
   and how library vs project sources differ.
2. **Source badge** on each row: `library` or `project`.
3. **New persona** button opens a form (name, roles, description, tags,
   body). Creates `.ddx/personas/<name>.md` with YAML frontmatter.
4. **Edit** affordance on project-source rows only.
5. **Delete** affordance on project-source rows only, with confirm.
6. **Fork to project** action on library-source rows. Copies the library
   file to `.ddx/personas/<name>.md`; if a collision exists, the form
   defaults to `<name>-local`.
7. **Empty-project hint** when no project personas exist: "No project
   personas yet. Fork a library persona or create a new one."
8. **Existing bind flow is untouched.**

## CLI parity (D3)

Same operations surfaced through:

- `ddx persona new <name>` — scaffold `.ddx/personas/<name>.md`.
- `ddx persona edit <name>` — open `$EDITOR` on the project-local file;
  errors on library personas.
- `ddx persona fork <library-name> [--as <new-name>]` — copy library into
  project.
- `ddx persona delete <name>` — project-only; errors on library.

UI and CLI share the same `cli/internal/persona` operations:
`CreateProjectPersona`, `UpdateProjectPersona`, `DeleteProjectPersona`,
`ForkPersonaToProject`. No duplicated write logic.

## Error model

All project-mutating operations check source first:

- If the target persona's source is `library`, return
  `PersonaError{Type: ErrorReadOnlyLibrary}` with message
  `"persona <name> is a library persona and cannot be <op>; fork it first"`.
- If the project file already exists on create, return
  `PersonaError{Type: ErrorAlreadyExists}`.
- If the project file does not exist on update/delete, return
  `PersonaError{Type: ErrorPersonaNotFound}`.

GraphQL mutations surface these as typed errors so the UI can show a
targeted message.
