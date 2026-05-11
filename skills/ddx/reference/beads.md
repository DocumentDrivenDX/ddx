# Beads — Writing Execution-Ready Work Items

A bead is a portable work item. It's created, picked up by an agent,
run against a real codebase, and closed with evidence. The single
hardest rule: **the bead description must be readable cold**. An
automated `ddx try` run has no chat history, no open
tabs, no prior context — only what's in the bead.

## Required structure

Every bead you create should have:

1. **Title** — imperative, specific. "Fix the pagination off-by-one
   in the bead list endpoint", not "Pagination bug".
2. **Type** — `task`, `bug`, `epic`, or `chore`.
3. **Description** — inline context (see below).
4. **Acceptance criteria** — a command that passes.
5. **Labels** — at minimum `area:<subsystem>` and `kind:<category>`.
6. **`spec-id`** (when applicable) — pointer to the governing
   FEAT-\*, SD-\*, TD-\*, or ADR-\* that authorizes the work.

## Inline context, not links

The description must stand alone. Do not write "see the discussion in
PR #142" or "as per the plan in ~/.claude/plans/...". Those are
ephemeral or inaccessible to an isolated-worktree agent. Inline the
relevant facts.

Bad:
> Fix the sync bug from last week's incident.

Good:
> When `ddx update` runs in a worktree whose `.ddx/config.yaml`
> specifies `library.path: /custom/path`, the library fetch uses the
> wrong remote. Root cause: `resolveLibraryPath()` in
> `cli/internal/config/paths.go:43` returns the default path when
> `library.path` is an absolute path. Make the function respect
> absolute paths without prefixing the worktree root.

## Explicit file scope

List the files in scope. Paths are repo-relative, specific.

```
In-scope files:
- cli/internal/config/paths.go
- cli/internal/config/paths_test.go (add test for absolute path case)

Out-of-scope (file a separate bead):
- cli/cmd/update.go — the downstream caller; its behavior is unchanged
- cli/internal/subtree/ — not touched by this fix
```

The "out-of-scope" list is **scope negation**. Without it, agents
drift into unrelated cleanup.

## Favor many small beads

Rule of thumb: if a bead would touch more than ~3 files or cross
more than one subsystem, **decompose it** into child beads under an
epic. Two small beads that each land cleanly are better than one
big bead that stalls.

```
Epic: "Add structured bead review evidence"
├── Task: Render per-criterion verdict rows
├── Task: Attach review evidence to a bead
├── Task: Reject malformed verdicts without evidence
└── Task: Add integration test using script reviewer output
```

Each child bead has ~3 files in scope, a command-based AC, and
depends on its predecessors via `ddx bead dep add`.

## Acceptance is a command

"X works" is not an acceptance criterion. A command that passes is.

Bad:
> Tests pass. Coverage is adequate.

Good:
> 1. `cd cli && go test ./internal/bead/... -run TestDep` passes.
> 2. `ddx bead dep tree ddx-test-0001` returns a tree with the
>    expected parent→child relationships (see test fixture).
> 3. `ddx bead dep tree ddx-test-0001` shows the dependent bead waiting
>    on its open parent.

Each AC line is a verifiable condition. No subjective language.

## Implementations include tests

The AC must reference at least one test command. A bead whose
implementation adds code without tests is half-done. How the tests
are structured (test-first, test-alongside, or test-after-for-a-
reason) is a workflow choice — the ddx-native requirement is that
tests exist, run, and pass.

## Rollback and cleanup

If the bead is partially implemented and the agent returns
`execution_failed` / `land_conflict` / etc., the bead goes back to
the ready queue **unclaimed** — no half-owned state. Your
description should tell the agent what state to leave behind if it
gives up:

- Don't leave dirty worktrees (`git stash` or revert changes).
- Don't leave partial files in the final location (write to a temp
  path and `mv` atomically, or don't commit partial edits).
- Don't leave external state changed (database rows, network calls
  with side effects) unless the bead is specifically about that
  side effect.

## spec-id when a governing artifact exists

If the bead implements part of a FEAT-\*, SD-\*, TD-\*, or ADR-\*,
link it via `--set spec-id=<ID>`. This ties the bead to the document
that authorized the work and enables traceability in review.

If there's no governing artifact (routine bug fix, dependency bump,
small chore), omit `spec-id`. Don't invent one.

## Anti-patterns

- **Description is a link.** "See PR #123." Agents can't read your
  chat scrollback; inline the facts.
- **Acceptance is a sentence.** "Works well." No way to verify;
  gives the agent wiggle room to claim success without evidence.
- **Scope creeps across files.** "Refactor the auth module." Break
  into beads per file or per behavior.
- **Dependencies wired implicitly.** "After the auth PR lands." Use
  `ddx bead dep add <this> <that>` so the queue reflects order.
- **Forgotten test command.** "Add a new endpoint." If the AC
  doesn't reference a test, the endpoint might not be tested.
- **Mandatory methodology labels you don't use.** Don't require
  `helix` or workflow labels unless your project actually uses that
  workflow. Use `area:*`, `kind:*`, and your project's own tags.

## CLI reference

```bash
# Create
ddx bead create "Title" --type task --labels area:cli,kind:bug \
  --description "..." --acceptance "..." \
  --set spec-id=FEAT-011

# Query
ddx bead list [--status open|closed] [--label <l>]
ddx bead ready                 # execution-ready open beads
ddx bead blocked               # status=blocked external blockers
ddx bead show <id>             # full detail
ddx bead status                # lifecycle and derived queue counts

# Update
ddx bead update <id> --labels ... --acceptance "..."
ddx bead update <id> --claim   # mark in-progress
ddx bead update <id> --unclaim # release
ddx bead close <id>            # close (success path)
ddx bead reopen <id>           # reopen a closed bead

# Dependencies
ddx bead dep add <from> <to>   # from depends on to
ddx bead dep remove <from> <to>
ddx bead dep tree <id>

# Import / export
ddx bead import --from jsonl <file>
ddx bead export [--status open]

# Evidence (append-only per-bead execution history)
ddx bead evidence add <id> ...
```

Full flag list: `ddx bead <subcommand> --help`.
