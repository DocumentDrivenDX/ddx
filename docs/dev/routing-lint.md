# Routing-cleanup lint (`routinglint`)

`routinglint` is the CI-enforced dead-code / anti-reintroduction check for
the compensating DDx-side routing helpers retired by **ddx-3bd7396a** under
**FEAT-006** (parent epic ddx-fdd3ea36 §AC#7, cleanup bead ddx-653f6ac9).

## Why it exists

Parent epic ddx-fdd3ea36 §AC#7 said:

> Search DDx for code that re-implements upstream routing concerns:
> allow-list checks, exact-pin filtering, profile-to-preference mapping,
> provider cost scoring. Must be absent on the default path.
> `ResolveProfileLadder`, `ResolveTierModelRef`, `workersByHarness`
> escalation helpers remain reachable via `--escalate` only. Dead-code
> detector run in CI catches orphans.

The follow-up cleanup bead **ddx-3bd7396a** went further than the parent
AC: it deleted those helpers and the `--escalate` / `--override-model`
flags entirely, and made `agent.routing.profile_ladders` /
`agent.routing.model_overrides` config keys hard-error at load. The
"reachable via `--escalate` only" half of the AC is therefore moot — the
default path is the *only* path.

`routinglint` enforces the stricter post-cleanup invariant: those
identifiers and flag/config-key strings must not reappear anywhere in
`cli/` Go source. If they do, the routing cleanup has regressed and CI
fails.

## What it checks

Two closed lists, both maintained as Go constants in
`cli/tools/lint/routinglint/analyzer.go`:

- **Forbidden identifiers** (exact `Ident.Name` match — substrings inside
  larger identifiers such as test-function names are not flagged):
  `ResolveProfileLadder`, `ResolveTierModelRef`,
  `ResolveProfileLadderCallCount`, `AdaptiveMinTier`, `workersByHarness`.

- **Forbidden string literals** (exact value match):
  `--escalate`, `--override-model`,
  `profile_ladders`, `model_overrides`,
  `agent.routing.profile_ladders`, `agent.routing.model_overrides`.

A diagnostic is emitted at every occurrence in any package under `cli/`.
The analyzer's own package (`tools/lint/routinglint/...`) is exempt
because the closed-list constants legitimately embed every forbidden
token.

## Where it runs in CI

The lint is wired into the lefthook `ci` block as `routing-lint-all`
(see `lefthook.yml`), which the GitHub Actions `CI Validation` job
invokes via `lefthook run ci`. The full-tree gate runs on every push
and PR — there is no staged-file fast-path because the invariant is
"zero matches anywhere", not "no new matches in the diff".

To run it locally:

```bash
cd cli
go run ./tools/lint/routinglint/cmd/routinglint ./...
```

Exit code 0 means the cleanup is intact. Any non-zero exit code lists
the offending file/line and which retired symbol/literal was found.

## Deliberate rejection paths

Some code paths *must* mention the retired tokens by literal — for
example, `cli/internal/config/config.go::checkRoutingMigration` reads
`profile_ladders` and `model_overrides` keys out of an incoming config
to issue the hard-error rejection that ddx-3bd7396a §AC#4 mandates;
the corresponding tests in `cli/internal/config/routing_migration_test.go`
and `cli/cmd/agent_execute_loop_routing_reject_test.go` assert that the
error message names the retired field.

Those occurrences are exempted by an inline annotation:

```go
// routinglint:legacy-rejection reason="hard-error rejection of retired routing keys per ddx-3bd7396a"
for _, field := range []string{"default_harness", "profile_ladders", "model_overrides"} {
```

The annotation may sit on the same line as the offending node or on a
comment line directly above it. The `reason="..."` clause must be
present and non-empty; the analyzer reports a separate diagnostic
("annotation is missing a non-empty reason= clause") if it is omitted.

The exemption is **per-line**, not per-file or per-package. Adding it
is a deliberate, reviewable signal that this specific occurrence is the
rejection / migration path, not new compensating-routing logic.

## Updating the closed lists

If the cleanup ever needs to retire an additional helper or flag,
extend `forbiddenIdents` or `forbiddenLiterals` in
`cli/tools/lint/routinglint/analyzer.go` and add a fixture line in
`cli/tools/lint/routinglint/testdata/src/violations/violations.go` so
`TestViolations` pins the new diagnostic.

The analyzer is intentionally conservative: it matches identifiers
exactly and string literals exactly. It does not pattern-match cost
scoring or allow-list shapes structurally — those are caught at code
review against the bullet list above. The closed-list approach keeps
false positives close to zero and makes every regression a one-line
fix or a one-line annotation, with the reviewer in the loop.
