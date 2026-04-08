---
ddx:
  id: SD-012
  depends_on:
    - FEAT-012
    - FEAT-001
    - FEAT-007
---
# Solution Design: Git Awareness and Revision Control Integration

## Purpose

Extend DDx with a thin, deliberate git integration layer: auto-commit on
document writes, document history and diff commands, an MCP write-and-commit
tool, and named checkpoints. DDx does not replace git; it exposes git
operations where DDx operations need them.

## Configuration Schema

Added to `.ddx/config.yaml` under the `git:` key:

```yaml
git:
  auto_commit: always   # always | prompt | never (default: never)
  commit_prefix: docs   # prefix for commit message scope
  checkpoint_prefix: ddx/  # prefix for checkpoint tags
```

`AutoCommitConfig` in `internal/git/autocommit.go` already maps these fields.
The config loader populates it from the `git` section via Viper.

## Commit Message Format

```
<prefix>(<artifact-id>): <description> [ddx: <operation>]
```

Examples:
- `docs(FEAT-001): stamp [ddx: doc-stamp]`
- `docs(FEAT-001): write via MCP [ddx: mcp-write]`
- `docs(FEAT-001): write via HTTP [ddx: http-write]`

`<prefix>` defaults to `docs`; overridden by `git.commit_prefix`.

## AutoCommit Helper

`internal/git/autocommit.go` exposes:

```go
func AutoCommit(filePath, artifactID, operation string, cfg AutoCommitConfig) error
```

Behavior:
1. Returns nil immediately if `cfg.AutoCommit` is `"never"` or empty.
2. Returns nil if not in a git repository (`IsRepository(".")`).
3. Stages `filePath` with `git add`.
4. Commits with `--no-verify` and a structured message.
5. Times out after 30 seconds.

`"prompt"` mode is reserved for interactive CLI callers; MCP/HTTP callers
always use `"always"`.

## Document History Commands

All three commands live under `ddx doc` and delegate to `internal/git`.
Artifact ID-to-path resolution uses the document graph (`internal/doc`),
which maps IDs to file paths via frontmatter and directory scan.

### `ddx doc history <id> [--since <ref>]`

```
git log --follow [--since <ref>] -- <resolved-path>
```

Output: short hash, date, author, subject — one line per commit.

### `ddx doc diff <id> [<ref1>] [<ref2>]`

- Zero refs: `git diff HEAD -- <path>` (working copy vs HEAD)
- One ref: `git diff <ref1> -- <path>`
- Two refs: `git diff <ref1> <ref2> -- <path>`

### `ddx doc changed [--since <ref>]`

```
git diff --name-only [<ref>] HEAD
```

Filters output to paths that resolve to a known artifact ID, then prints
`<id>  <path>` pairs.

All three commands fail gracefully (clear message, exit 1) when the working
directory is not inside a git repository.

## MCP Write+Commit Flow (`ddx_doc_write`)

```
Client calls ddx_doc_write(id, content)
  1. Resolve artifact ID → absolute file path (internal/doc)
  2. Write content to file (atomic: write temp, rename)
  3. Call AutoCommit(path, id, "mcp-write", cfg)
     a. If commit succeeds → return success response
     b. If commit fails → restore file from pre-write backup, return error
  4. If no git repo → write succeeds, response notes "not committed"
```

The backup/restore step ensures the working tree is not left dirty on commit
failure. A pre-write copy is taken before the temp-file write. MCP callers
always use `"always"` mode; they have no interactive channel.

## HTTP Write+Commit Flow (`PUT /api/docs/:id`)

Same sequence as MCP. The server handler calls the same `internal/doc` write
function which calls `AutoCommit`. The response body includes a `committed`
boolean and, on failure, an `error` string.

## Checkpoint Command (`ddx checkpoint`)

Thin wrapper over git tags with the configured prefix (default `ddx/`).

| Subcommand | Git operation |
|---|---|
| `ddx checkpoint <name>` | `git tag ddx/<name>` |
| `ddx checkpoint --list` | `git tag -l 'ddx/*'` |
| `ddx checkpoint --restore <name>` | `git checkout ddx/<name>` |

Tag names must be non-empty and contain only alphanumeric characters, hyphens,
and dots.

## Security Invariants

DDx git operations are additive and read-only against history:

- **Never force-push.** DDx does not invoke `git push` at all.
- **Never rebase.** DDx does not invoke `git rebase` (except the managed
  execute-bead exception below).
- **Never delete branches.** DDx does not invoke `git branch -d` or
  `git branch -D`.
- **Never amend.** All commits are new commits; no `--amend`.
- **Hooks skipped on auto-commit only.** `--no-verify` is used for mechanical
  auto-commits to stay under the 500 ms target. User-initiated commits (future
  `"prompt"` mode) run hooks normally.

These invariants are enforced by the limited surface of `internal/git`: only
`git add`, `git commit`, `git log`, `git diff`, `git tag`, and `git checkout`
(for restore) are invoked outside the execute-bead managed flow.

## Managed Exception: ddx agent execute-bead

`ddx agent execute-bead` is the one controlled exception to the conservative
safety posture. The following git operations are explicitly permitted within
this workflow only:

| Operation | Purpose |
|-----------|---------|
| Checkpoint commit on dirty worktree | Preserve caller state before execution begins |
| `git worktree add` | Create isolated execution environment |
| `git rebase <base>` | Prepare a fast-forward landing only — not for history rewriting |
| Fast-forward branch update | Land a successful, merge-eligible iteration |
| `git update-ref refs/ddx/...` | Preserve a non-landed iteration under a hidden ref |
| `git worktree remove` | Clean up after the workflow (always runs) |

**No-orphan-worktree invariant:** DDx must remove the execution worktree after
the workflow completes, whether the iteration lands or is only preserved. A
crash during cleanup may leave a worktree, but DDx startup or the next
`execute-bead` invocation should detect and remove orphaned worktrees matching
the `refs/ddx/iterations/` namespace.

**Hidden-ref naming scheme:**

Non-landed iterations are preserved under:
```
refs/ddx/iterations/<bead-id>/<timestamp>-<base-shortsha>
```

Where:
- `<timestamp>` is in `YYYYMMDDTHHMMSSZ` format (UTC, compact ISO-8601), e.g., `20260408T130000Z`
- `<base-shortsha>` is at least a 12-character prefix of the base commit SHA

Example: `refs/ddx/iterations/ddx-abc12345/20260408T130000Z-418a646def01`

These refs are local only. DDx does not push them. Preserved iterations can be
enumerated with `git for-each-ref 'refs/ddx/iterations/<bead-id>/*'`.

### Acceptance Criteria (execute-bead git operations)

- Given execute-bead creates a hidden ref, when the ref is inspected, then its name matches `refs/ddx/iterations/<bead-id>/YYYYMMDDTHHMMSSZ-<12charsha>` with a UTC compact timestamp and at least 12-character SHA.
- Given execute-bead completes (any outcome), when the filesystem is inspected, then no worktree matching the execute-bead worktree path pattern remains.
- Given the target branch is fast-forward updated by execute-bead, when `git log --merges` is inspected, then no merge commit exists — history is linear.

## Graceful Degradation

If `IsRepository(".")` returns false, all git-dependent features (auto-commit,
history, diff, changed, checkpoint) return a clear error:

```
not in a git repository; git features are unavailable
```

Core DDx functionality (bead tracker, agent service, document library) is
unaffected.
