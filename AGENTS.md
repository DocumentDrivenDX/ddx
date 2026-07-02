# DDx Agent Instructions

This repository uses DDx's built-in bead tracker for durable work management.

## Default Interactive Mode

When an agent is opened in this repository without an explicit execution
directive, the default mode is **`queue_steward`**: survey, triage, and
advise on the DDx bead queue without claiming or editing anything unless
the user explicitly asks.

| Mode | Trigger | Allowed | Prohibited |
|---|---|---|---|
| `queue_steward` | Broad DDx questions: "what's on the queue", "what's ready", "how am I doing", "do work" without an explicit worker directive | Read tracker and docs; report status; advise on bead quality; run `ddx bead list/ready/status/show` | Claiming or editing beads; starting `ddx work`/`ddx try` autonomously |
| `bead_execution` | Explicit worker directive: `ddx work`, `ddx try <id>`, or an `execute-bead` harness invocation | Full execute-bead lifecycle per the bead body | Scope creep outside the named bead |
| `direct_user_implementation` | User explicitly asks to edit code or docs ("fix this bug", "update this file") | Edit code and docs as instructed; commit to the current branch | Draining the bead queue autonomously |
| `review` | User explicitly asks for a review ("review this PR", "grade this bead") | Read-only evidence-based review per §Reviewer Mode | Writing commits or claiming beads |

**`bead_execution` supersedes `queue_steward`** when DDx invokes a worker
explicitly. Tracker, merge-policy, and safety instructions remain
load-bearing across all modes.

Execution-ready beads belong to `ddx work` (or an explicit `ddx try <id>`).
Do not start execution unless the user explicitly requests direct implementation.

## Bead Policy

- Treat `.ddx/beads.jsonl` as DDx-managed data, not as a hand-edited document.
- Create beads only with `ddx bead create`. Before filing, conform to `docs/helix/06-iterate/bead-authoring-template.md` — descriptions + AC must be standalone (no `/tmp/*`, no chat refs), cite root cause by file:line, name specific `Test*` symbols, and include the `cd cli && go test ./<pkg>/...` + `lefthook run pre-commit` gate. Sub-agent execution depends on this floor (principle P7).
- See docs/helix/06-iterate/reliability-principles.md for the P1-P10 reliability principles applied to ddx try / ddx work execution.
- Modify bead metadata only with `ddx bead update`.
- Manage dependencies only with `ddx bead dep add` and `ddx bead dep remove`.
- Close work only with `ddx bead close`.
- Use `ddx bead import` and `ddx bead export` for bulk migration or interchange.
- Commit tracker mutations by default after bead commands.
- If the tracker change stands alone, make a tracker-only commit promptly.
- If it belongs to related implementation/docs work already being prepared for
  commit, fold the tracker change into that same commit instead of leaving
  `.ddx/beads.jsonl` dirty.

## When filing beads — bead-authoring template

Every bead must satisfy the 8-criterion rubric documented in
`docs/helix/06-iterate/bead-authoring-template.md` before it is filed
or dispatched to a sub-agent. The bead body is the entire prompt the
sub-agent will see; if a competent agent given only the bead text
cannot pick a file to edit and run tests without asking, the bead
must be retrofitted before dispatch.

Required template fields: title (imperative + subsystem), description
with PROBLEM + ROOT CAUSE WITH FILE:LINE + PROPOSED FIX + NON-SCOPE,
numbered AC including specific `Test*` names + `cd cli && go test ...`
+ `lefthook run pre-commit`, labels (phase + area + kind + cross-refs),
and explicit parent + deps.

Do not cite `/tmp/...` plan files as load-bearing context — they do
not survive between machines or sessions. Inline the relevant excerpt
into the description instead.

## Merge Policy

Branches containing execute-bead or work commits carry a
per-attempt execution audit trail in their git history. This trail is
load-bearing data, not noise:

- `chore: update tracker (execute-bead <TIMESTAMP>)` — one commit per
  attempt; the timestamp is the attempt ID.
- `Merge bead <bead-id> attempt <TIMESTAMP>- into <branch>` — the merge
  commit that lands a successful attempt from its isolated worktree.
- `feat|fix|refactor|...: ... [ddx-<id>]` — the substantive bead work,
  tagged with the bead ID.

Bead records store `closing_commit_sha` as a pointer back into git
history; cost, latency, retry, and tier-escalation reports read these
commits directly. Any SHA rewrite breaks the pointers and destroys the
`output = bead(input)` accuracy the system is built on.

**NO HISTORY REWRITING on execute-bead branches.** The only acceptable
merge strategies are:

1. **Plain fast-forward** — when the target is a strict ancestor of the
   feature branch: `git merge --ff-only` + `git push`. No new commits.
2. **Merge commit** — when divergence exists: `git merge --no-ff`.
   Creates a 2-parent merge; the feature branch commits remain intact.

Never use on an execute-bead branch:

- `gh pr merge --squash` — collapses every attempt into one commit.
- `gh pr merge --rebase` — GitHub's rebase-merge replays commits as NEW
  SHAs, breaking `closing_commit_sha` pointers.
- `git rebase -i` with `fixup`, `squash`, `drop`, or `reword` — rewrites
  SHAs.
- `git filter-branch` / `git filter-repo` stripping "chore" commits — same.
- `git commit --amend` on any commit already in the trail — same.

When in doubt, check `git log <branch> --oneline | grep -E 'execute-bead|\[ddx-'`.
If any match, preserve history on the merge.

The `pre-push` hook in `lefthook.yml` (`merge-policy` command) enforces
this: if a push would drop execute-bead or `[ddx-*]` commits the remote
already has — i.e. a force-push rewriting that history — the hook
rejects it. Do not disable this hook to work around it; the right move
is to keep the commits intact and use `--ff-only` or a `--no-ff` merge.

## Prohibited Actions

- Do not edit `.ddx/beads.jsonl` manually.
- Do not add, remove, or rewrite bead rows with `apply_patch`, editors, scripts, or ad hoc JSONL manipulation.
- Do not invent bead IDs or prefixes such as `hx-*` or `ddx-*`.
- Do not treat nearby tracker entries as a naming pattern to copy.
- Do not squash, rebase, or filter branches containing execute-bead commits (see Merge Policy above).

## If The CLI Seems Insufficient

- Prefer the nearest supported `ddx bead` command.
- If the required tracker operation is not supported by `ddx bead`, stop and ask rather than editing tracker storage directly.

## Verification

- Use `ddx bead show <id>` to inspect one bead.
- Use `ddx bead list`, `ddx bead ready`, and `ddx bead status` to inspect queue state.
- Use `ddx bead --help` and `ddx bead create --help` before assuming a flag exists.

## Skill Policy

- Treat `SKILL.md` frontmatter as a strict interface, not freeform metadata.
- Published DDx skills must use top-level YAML frontmatter fields `name` and `description`.
- Use `argument-hint` only when the skill accepts a trailing positional or shorthand invocation hint.
- Do not use nested `skill:` frontmatter for DDx repo skills.
- Run `ddx skills check [path ...]` for reusable validation across repo skills and plugin skills.
- Run `make skill-schema` after editing any file under `skills/*/SKILL.md` or `cli/internal/skills/*/SKILL.md`.

## Reviewer Mode

- When a task explicitly calls for no-tool reviewer mode, keep the pass
  read-only and evidence-based. That convention is the structured-evidence
  review mode described in TD-033; do not attach tools mid-pass.
- Prefer the supplied artifacts and repo files over exploratory tool use.
- If you need a factual citation, point to the smallest relevant file:line
  span and avoid broad re-reading of unrelated context.

## Pre-Claim Worktree Cleanliness

Before a `ddx work` worker claims the next bead, it verifies the landing
worktree has no staged changes that a new claim could clobber. Not every
staged path blocks a claim:

- **Block pre-claim** (real work that must be committed first): any staged
  code, doc, or test file — anything outside the DDx-managed tracker set
  below. A staged code change surfaces as a `preclaim_systemic` idle.
- **Do NOT block pre-claim** (DDx-managed tracker/metadata, rewritten
  continuously by concurrent workers): `.ddx/beads.jsonl`,
  `.ddx/beads-archive.jsonl`, `.ddx/metrics/attempts.jsonl`, and anything
  under `.ddx/attachments/`. These are append-mostly metadata, not code, and
  the next claim rewrites them anyway. When only these are staged the worktree
  is treated as clean.

If a worker idles specifically because tracker files are mid-commit on a busy
multi-worker host, that transient state is reported as
`preclaim_tracker_contention` (distinct from `preclaim_systemic`). After
several consecutive idle cycles on the same blocker the worker raises a
non-terminal operator-attention event instead of looping silently.

## Wedge / Timeout Lease-Release Contract

A `ddx work` worker that claims a bead holds a lease on it. Three guards release
that lease and flag the bead for operator attention rather than letting a single
wedged bead hold its lease (and stall the single-threaded worker) indefinitely:

- **Route-resolution timeout** — route resolution / routing preflight is bounded
  by a per-operation deadline (default **60s**, `DefaultRouteResolutionTimeout`,
  override with `--route-resolution-timeout`). On expiry the lease is released
  and the bead is flagged, not auto-retried.
- **Progress watchdog** — phase-empty heartbeats (harness, model, and route all
  empty) that persist past the applicable phase budget (defaults: **5m while
  resolving**, **30m while running**; `work.DefaultPhaseBudgets`) fire the
  watchdog, which cancels the attempt and releases the lease.
- **Consecutive-wedge guard** — when a bead wedges (route-resolution timeout or
  watchdog fire) on consecutive claims up to the threshold (default **2**,
  `DefaultConsecutiveWedgeThreshold`), the worker stops re-claiming it, parks it
  to `proposed`, and continues draining the rest of the queue.

Each release appends a durable `operator_attention` event carrying the bead-id,
attempt-id, last_activity_at, and a diagnosis string. Inspect every such release
with:

```bash
ddx bead operator-attention            # text: one line per release
ddx bead operator-attention --json     # structured rows for tooling
```

## Lock Lifetime Contract

The two short-lived git locks an executing worker touches must be scoped to
the mutation window only — never held across the long LLM/harness wait. Do not
regress this:

- **Never hold a lock across an LLM subprocess.** `.git/index.lock` and
  `.ddx/.git-tracker.lock` may be held only for the git/tracker mutation
  itself. Acquire after the harness returns, release before the next wait.
  The "LLM wait" happens entirely outside any lock.
- **Hold-time caps.** A worker that holds a lock past its cap is treated as
  hung: the lock is force-released and a violation is recorded. Defaults are
  **10s** for `index.lock` (`DefaultIndexLockCap`) and **30s** for
  `tracker.lock` (`DefaultTrackerLockCap`), defined in
  `cli/internal/lockmetrics/lockcap.go`. Override per-lock in milliseconds via
  the `DDX_LOCK_CAP_INDEX_MS` and `DDX_LOCK_CAP_TRACKER_MS` environment
  variables.
- **Violation evidence.** When a cap is exceeded the watchdog writes
  `.ddx/executions/<run-id>/lock-violation.json` (lock name, cap, actual hold,
  holder PID, stack) so the post-execution reviewer sees the over-long hold.
- **Enforcing test.** The wired-in proof is
  `cli/internal/integration/lock_contention_test.go`
  (`TestIntegration_MultiWorkerLockContention_*`): 5 concurrent `ddx work`
  workers drain a shared queue while an operator hammers the tracker, and the
  p99 index/tracker hold times must stay under the caps with no operator
  timeouts.

<!-- DDX-AGENTS:START -->
<!-- Managed by ddx init / ddx update. Edit outside these markers. -->

# DDx

This project uses [DDx](https://github.com/DocumentDrivenDX/ddx) for
document-driven development. Use the `ddx` skill for beads, work,
review, agents, and status — every skills-compatible harness (Claude
Code, OpenAI Codex, Gemini CLI, etc.) discovers it from
`.claude/skills/ddx/` and `.agents/skills/ddx/`.

## Files to commit

After modifying any of these paths, stage and commit them:

- `.ddx/beads.jsonl` — work item tracker
- `.ddx/config.yaml` — project configuration
- `.agents/skills/ddx/` — the ddx skill (shipped by ddx init)
- `.claude/skills/ddx/` — same skill, Claude Code location
- `docs/` — project documentation and artifacts

## Conventions

- Use `ddx bead` for work tracking (not custom issue files).
- Documents with `ddx:` frontmatter are tracked in the document graph.
- Run `ddx doctor` to check environment health.
- Run `ddx doc stale` to find documents needing review.

## Merge Policy

Branches containing `ddx try` or `ddx work` commits
carry a per-attempt execution audit trail:

- `chore: update tracker (execute-bead <TIMESTAMP>)` — attempt heartbeats
- `Merge bead <bead-id> attempt <TIMESTAMP>- into <branch>` — successful lands
- `feat|fix|...: ... [ddx-<id>]` — substantive bead work

Bead records store `closing_commit_sha` pointers into this history. Any
SHA rewrite breaks the trail. **Never squash, rebase, or filter** these
branches. Use only:

- `git merge --ff-only` when the target is a strict ancestor, or
- `git merge --no-ff` when divergence exists

Forbidden on execute-bead branches: `gh pr merge --squash`,
`gh pr merge --rebase`, `git rebase -i` with fixup/squash/drop,
`git filter-branch`, `git filter-repo`, and `git commit --amend` on
any commit already in the trail.
<!-- DDX-AGENTS:END -->
