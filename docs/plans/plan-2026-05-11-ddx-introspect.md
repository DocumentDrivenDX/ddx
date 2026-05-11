---
ddx:
  id: plan-2026-05-11-ddx-introspect
---
# `ddx __introspect` Primitive: Versioned Command-Tree Dump

Date: 2026-05-11
Status: Draft — gates both the pip-distribution Python API codegen and the website CLI-reference generator

## Why this exists

Two in-flight plans both want a deterministic dump of DDx's Cobra command tree:

- `docs/plans/plan-2026-05-10-pip-distribution-and-python-api.md` — drives `python/scripts/codegen.py` which generates Python wrapper functions for every CLI command.
- `docs/plans/plan-2026-05-11-website-autogen.md` — drives the website's CLI command reference pages.

Both consumers will read "the same JSON of commands, flags, args, and help" and project it into their target medium. Without a single specified primitive, the two consumers will derive their own schemas from the Cobra source, diverge the first time either needs a new field, and silently break each other's output.

The right shape: **one hidden CLI command, one versioned JSON schema, one golden test.** Multiple consumers depend on a tagged schema version; the golden test fails on drift.

## What `ddx __introspect` does

Add a hidden Cobra subcommand at the root: `ddx __introspect`. Hidden so it doesn't appear in `--help` output (it's a tooling primitive, not a user-facing command). Walks the root Cobra command tree, emits a single JSON document to stdout, exits zero on success or non-zero on schema-validation failure.

### CLI surface

```
ddx __introspect [--schema-version <n>]

Emit a JSON description of the DDx command tree, flags, args, and help
strings. Output is deterministic — same source produces byte-identical
output across runs. Schema is versioned; consumers should pin a version.

Flags:
  --schema-version int   Emit a specific schema version (default: latest stable)
```

Output goes to stdout. Errors to stderr.

### Output schema (v1)

```json
{
  "$schema": "https://ddx.dev/schemas/introspect/v1.json",
  "schema_version": 1,
  "generated_at": "2026-05-11T00:00:00Z",
  "ddx_version": "0.x.y",
  "commands": [
    {
      "name": "bead",
      "full_name": "ddx bead",
      "path": ["bead"],
      "short": "Manage work item tracker",
      "long": "<full markdown body of Cobra cmd.Long, untouched>",
      "hidden": false,
      "deprecated": false,
      "args": [
        {
          "name": "id",
          "type": "string",
          "required": true,
          "variadic": false,
          "help": "Bead identifier"
        }
      ],
      "flags": [
        {
          "name": "json",
          "shorthand": "",
          "type": "bool",
          "default": "false",
          "help": "Output as JSON",
          "persistent": false,
          "deprecated": false,
          "hidden": false
        }
      ],
      "inherited_flags": [],
      "subcommands": [
        { /* recursive: same shape */ }
      ],
      "annotations": {
        "streaming": "true"
      }
    }
  ]
}
```

### Schema invariants (normative)

1. **Deterministic output.** Two runs against the same binary produce byte-identical JSON (excluding `generated_at` and `ddx_version`, which are sortable to a fixed-position field at the end of the envelope to support `diff`-based comparison). Field order within objects is alphabetical except for `name` (always first) and `subcommands`/`flags`/`args` (always last). Subcommands sorted by `name`.
2. **No unresolved values.** Every flag's `default` is rendered as its string representation, never as an opaque type. Pflag custom types render with their `String()` method; if that's unimplemented, the generator emits a `"default": null` plus a `"default_note": "<type> has no String()"` field so consumers can decide policy.
3. **`long` is raw markdown.** Whatever the Cobra `cmd.Long` field contains, verbatim, with no Hugo or Jinja preprocessing. Consumers render it through their own markdown pipeline.
4. **`inherited_flags` is separate from `flags`.** Cobra's `Flags().VisitAll` includes persistent flags inherited from parents; emitting them only under `inherited_flags` (with the originating command path) lets consumers either dedupe or display them distinctly.
5. **Annotations passthrough.** Whatever's in `cmd.Annotations` map propagates verbatim. Consumers (website, pip codegen) read specific keys: e.g. `streaming: true` means the Python wrapper emits an iterator variant.
6. **No I/O side effects.** The command does not read or write `.ddx/`, the network, or anything but `stdout`/`stderr`. Safe to invoke in CI without project context.

### Schema versioning

`schema_version: <int>` is incremented when an incompatible change is made (field removed, field renamed, semantics changed). Additive fields (new optional fields) do NOT bump the version — consumers ignoring unknown fields keep working.

Consumers pin a minimum version: `if schema_version < 1: fail`. CI runs of both consumers against the latest `ddx __introspect` output validate that the schema they expect still matches.

## How consumers use it

### Website CLI generator (`cli/tools/gen-website/cli_introspector.go`)

```go
// Run `ddx __introspect` (or import the introspection package directly,
// avoiding subprocess overhead). Parse the JSON. Walk commands recursively
// emitting Hugo data file entries under website/data/cli/commands.yaml.
```

The unified `gen-website` binary calls into the introspection package directly (no subprocess), since both live in the same Go module. The JSON envelope shape is the same as the subprocess-emitted form.

### Pip Python codegen (`python/scripts/codegen.py`)

```python
# Invokes `ddx __introspect` as a subprocess (Python codegen lives outside
# the Go module). Parses the JSON. Writes per-command-group .py files
# under src/ddx/api/ with real function signatures and docstrings.
```

The subprocess call is tagged with `--schema-version 1` to pin the expected schema; if the binary in the wheel build environment doesn't support v1, the codegen fails loudly at build time.

### Why not just have the website use the Go package directly and skip the subprocess?

It does. The website generator imports the introspection package. The pip codegen has to use the subprocess because Python tooling can't link to Go libraries. Two consumers, two paths to the same data, one schema. The subprocess form is for cross-language consumers; the Go import is for Go consumers.

## Golden test

A single Go test in `cli/cmd/introspect_test.go` (or wherever the introspection lives):

```go
func TestIntrospect_GoldenSnapshot(t *testing.T) {
    out := captureIntrospect(t)
    golden := loadGolden(t, "testdata/introspect.golden.json")
    if !equalIgnoringTimestamps(out, golden) {
        t.Fatalf("introspect output drifted from golden; either fix the regression or update with -update flag")
    }
}
```

The golden file (`testdata/introspect.golden.json`) is checked in. Adding a new command, flag, or default value to the codebase causes the test to fail; the fix is `go test -update`, regenerating the golden, which becomes part of the PR. CI catches accidental schema drift.

The `equalIgnoringTimestamps` comparator drops `generated_at` and `ddx_version` from both sides.

## Where the implementation lives

```
cli/cmd/introspect.go              # the hidden Cobra command
cli/cmd/introspect_test.go         # golden test
cli/cmd/testdata/introspect.golden.json
cli/internal/introspect/           # the actual walker (importable by website generator)
  walker.go                        # walks rootCmd, emits the struct
  schema.go                        # the JSON output types
  schema.go                        # version constant
```

The Cobra command is a thin shim that calls `introspect.Walk(rootCmd())` and JSON-encodes the result. The walker has no Cobra-specific output dependencies — it returns Go structs. This separation lets the website generator skip the JSON marshal/unmarshal round-trip while preserving the subprocess form for the pip consumer.

## Acceptance criteria

1. `ddx __introspect` exists, hidden, emits JSON to stdout.
2. Output is deterministic (verified by the golden test: two runs produce identical output modulo `generated_at`).
3. `cli/internal/introspect/` package exposes `Walk(rootCmd *cobra.Command) (*Document, error)` and `Document` JSON-marshals to the v1 schema.
4. Golden test `TestIntrospect_GoldenSnapshot` exists and passes.
5. The hidden command does not appear in `ddx --help` output (verified by a test grep).
6. The schema version constant `IntrospectSchemaVersion = 1` is exported so consumers can compile-time-assert against it.
7. `cd cli && go test ./cmd/... ./internal/introspect/...` is green.
8. `lefthook run pre-commit` passes.

## Sequencing

This is **upstream of both the pip plan and the website autogen plan**. File this bead first. Pip Python API codegen (bead in `plan-2026-05-10-pip-distribution-and-python-api.md`) depends on it. Website CLI generator (bead A3 in the revised autogen plan) depends on it.

## Risks

| Risk | Mitigation |
|------|------------|
| Cobra adds a new field type the walker doesn't know about | Walker uses `flag.Value.Type()` exhaustively; unknown types emit `"type": "string"` with a warning logged. Test the warning path. |
| `cmd.Long` body contains content that breaks downstream markdown renderers | Out of scope for the introspect primitive — consumers are responsible for their own rendering. The primitive emits raw markdown faithfully. |
| Golden test becomes noisy on every CLI change | Expected. The cost of catching drift is regular golden updates. Mitigation: golden update is one-flag (`-update`), reviewable in the PR diff. |
| Cross-language schema drift | Schema version constant + subprocess `--schema-version` flag. The Python consumer pins v1; if the binary emits a higher version, the consumer fails loudly. |
| Hidden command discovered by an automation that thinks `__`-prefix is internal | Document the `__`-prefix as the "tooling-primitive" convention in the command's own help text. Recommend other tooling primitives (`__doc-graph-dump`, `__attempt-dump`) follow the same convention. |
