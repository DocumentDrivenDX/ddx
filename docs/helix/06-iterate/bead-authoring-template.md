# Bead Authoring Template

**Authority:** principle P7 (`docs/helix/06-iterate/reliability-principles.md`,
bead `ddx-06b77652`) and audit findings in
`.ddx/executions/20260503T195715-4725673a/bead-quality-audit-2026-05-03.md`
(bead `ddx-57f0cb9e`).

A bead's description + AC must be **sufficient context for a competent
sub-agent to execute the work without operator hand-curation**. Today,
`ddx try` and `ddx work` workers run with the bead text verbatim — no
chat scrollback, no open tabs, no operator interpretation. If the bead
punts the investigation, the worker either:

- Gives up (`no_changes` after limited investigation), or
- Produces wrong work (wrong file, wrong scope), or
- Asks for clarification (impossible in autonomous drain).

This template is the floor.

## The 8-criterion rubric

Every bead is scored on:

- **(a) Title** — one-line scope clarity, imperative voice.
- **(b) Description** — problem statement, ROOT CAUSE w/ file:line, proposed
  fix, explicit non-scope.
- **(c) Acceptance** — numbered, verifiable; specific `Test*` names where
  tests are involved.
- **(d) Wired-in assertion** — when introducing new code paths, AC asserts
  the path is reachable from production callers (not just defined).
- **(e) Test + lefthook gate** — AC includes the exact `cd cli && go test
  ./<path>/... -run <Pattern>` line and a `lefthook run pre-commit` line (or
  doc-only equivalent).
- **(f) Labels** — `phase:N`, `area:*`, `kind:*`, plus cross-refs
  (`adr:NNN`, `prevention`, `triage:*`) when applicable.
- **(g) Parent + Deps** — `--parent` set; `ddx bead dep add` wires hard
  ordering.
- **(h) Standalone description** — readable cold; no references to chat,
  `/tmp/*`, `~/Projects/*`, ephemeral plans, or prior conversations.

Pass = ≥7/8. Anything less needs retrofit before sub-agent dispatch.

## Template fields

```bash
ddx bead create "<imperative-title-with-scope>" \
  --type task \
  --priority <0-3> \
  --parent <parent-id> \
  --labels phase:<n>,area:<sub>,kind:<cat>[,<cross-refs>] \
  --description "$(cat <<'EOF'
<one-paragraph problem statement: what is broken/missing and why it matters>

ROOT CAUSE
<file:line citation for each touchpoint; one per line>
- cli/internal/foo/bar.go:123 — <what's wrong here>
- cli/cmd/baz.go:45 — <what's wrong here>

PROPOSED FIX
<concrete change shape; data structures, function signatures, control flow.
Not "refactor X" — say what new shape replaces what old shape.>

NOT IN SCOPE
- <adjacent change that the worker might be tempted to fold in>
- <future cleanup that has its own bead>

INTERSECTIONS
- <bead-id>: <how this bead relates>
EOF
)" \
  --acceptance "$(cat <<'EOF'
1. <wired-in assertion: new symbol is reachable from a production entry point
   OR called from existing test fixture; name the caller>
2. <behavioral assertion: specific input → specific output, with file/line
   pointers to the assertion>
3. <test command, exactly>: cd cli && go test ./internal/<pkg>/... -run TestSpecificName
4. lefthook run pre-commit passes.
5. Conventional commit ending [<this-bead-id>].
EOF
)"

# Then wire dependencies:
ddx bead dep add <this-id> <upstream-id>
```

## Authoring checklist

Before invoking `ddx bead create`, walk the rubric:

- [ ] **(a) Title** ≤ 80 chars, imperative, names the subsystem AND the
      change ("agent: unify ExecuteLoopSpec across cobra/HTTP/server/worker
      layers", not "fix spec drift").
- [ ] **(b) Root cause cited by file:line.** Open the file. Read the
      surrounding 20 lines. Quote the smell. If you cannot cite a
      file:line, the investigation is not done — do that work BEFORE
      filing the bead, not in the bead.
- [ ] **(b) Non-scope is explicit.** Name the adjacent files/concerns the
      worker is NOT to touch. The list converts implicit operator
      knowledge into a hard contract.
- [ ] **(c) AC is numbered.** Each line is one verifiable claim.
- [ ] **(c) Test names are specific.** `TestExecuteLoopSpec_RoundTripsAll
      Fields_Reflection`, not "tests cover the spec".
- [ ] **(d) Wired-in.** If you add a new function/type/handler/flag, an AC
      line asserts a production caller invokes it. (`go run
      golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` reports zero new dead
      symbols is a strong form.)
- [ ] **(e) Concrete test command.** `cd cli && go test
      ./internal/<pkg>/... -run <Pattern>` — copy-pasteable.
- [ ] **(e) Lefthook gate.** `lefthook run pre-commit passes.`
- [ ] **(f) Labels include cross-refs.** `adr:022` if it's an ADR-022
      slice; `prevention` if it's preventing a class of bug; `spec-id`
      via `--set spec-id=FEAT-NNN` if a governing artifact exists.
- [ ] **(g) Parent + deps.** `--parent` to the epic or governing bead.
      `ddx bead dep add` for hard-order constraints.
- [ ] **(h) Standalone read.** Re-read the description as a stranger:
      every named entity (file, bead, test, concept) either exists in the
      repo today or is defined inline. **No `/tmp/*`, no `~/Projects/*`,
      no "see chat", no "as discussed".**

## Example: a good bead (template-quality)

`ddx-9e4c238d` — auto-routing rejection fix.

What makes it work:

- **Title** names the symptom AND the fix shape: "DDx pre-flight too
  strict; should pass through to fizeau".
- **Description** has an OBSERVED block (operator report with date), an
  EXPECTED block, **5 ROOT CAUSE HYPOTHESES** each with file:line, an
  INVESTIGATION REQUIRED block (what to check first), a FIX SHAPE block,
  a NOT IN SCOPE block, and an INTERSECTION block linking to related
  beads.
- **Acceptance** has 7 numbered lines: investigation report path, the
  behavioral fix, the structured-event requirement, manual verification
  steps, a specifically-named test (`TestAutoRoute_BareWork_NoPreflight
  Rejection`), the test+lefthook gate, and a non-regression clause for
  existing tests.

A sub-agent dispatched against this bead can begin work without asking a
single clarifying question.

## Example: a weak bead (anti-pattern)

`ddx-256af8b5` — S15-7 multi-node trust attestation forwarding.

What's broken:

- **Description** says: *"See /tmp/story-15-final.md §Risks 'Multi-node
  authorization gap' and §Tests Multi-node line."* The path
  `/tmp/story-15-final.md` does not exist in the worktree the worker
  receives. **Annotation:** the operator had this doc open in their
  shell scrollback at filing time. The worker does not.
- **Acceptance** is a single run-on sentence with five conjoined claims
  ("Coordinator forwards…; client-side authorization checks…; ADR
  -conformance lint added…; integration test exercises…"). No numbered
  structure. **Annotation:** numbering each clause exposes that "ADR-
  conformance lint" is a whole sub-bead's worth of work, not an AC line.
- **No file:line citations** for where the change lands.
  **Annotation:** sub-agent will grep blindly; success unlikely.
- **Cross-ref label `area:*` missing.** `area:server`? `area:federation`?
  Operator was undecided; worker has no signal.

Retrofit path: inline the §Risks and §Tests text from the now-gone
`/tmp/story-15-final.md`; convert the AC blob into 5 numbered lines;
cite `cli/internal/server/graphql_adapters.go` (or whichever file) at
the relevant line; add `area:server,area:federation` labels.

## Special cases

### Doc-only beads

(e) is satisfied by `ddx doc audit clean` for the changed file or an
equivalent verification command. `lefthook run pre-commit` still applies.
Skip (d) — doc-only beads have no production code path.

### Epic beads

The epic is a tracker, not an executable bead. Its AC is the
**rollup gate**: which children must close, what soak/observation period
applies (with concrete duration), what evidence collection lands
(CHANGELOG fragment, ADR sign-off, etc.). The detail belongs in the
children, but the rollup gate must be testable: "all 9 children closed +
acceptance bead's soak passed for ≥1 week" requires the soak metric to
be defined upstream.

### Backfill beads (mechanical)

Generate the symbol/file list from tooling output; do not curate by
hand. `ddx-0131ebf0` is the canonical example: 69 unreached symbols
listed `cmd/<file>.go:<line> — <symbol>`, originated from
`go-production-reachability` checker JSON. Mechanical generation makes
(b) trivially pass.

## See also

- `docs/helix/06-iterate/reliability-principles.md` — the 7 reliability
  principles; this template enforces P7.
- `.ddx/executions/20260503T195715-4725673a/bead-quality-audit-2026-05-03.md`
  — the audit that motivated this template; includes per-criterion
  pass-rates and the Wave 1 retrofit list.
- `.agents/skills/ddx/reference/beads.md` — operator-facing bead-writing
  guide; complementary to this template (this template is the
  enforcement bar; the skill reference is the worked-example tutorial).
