# DDx Agent Instructions

This repository uses DDx's built-in bead tracker for durable work management.

## Bead Policy

- Treat `.ddx/beads.jsonl` as DDx-managed data, not as a hand-edited document.
- Create beads only with `ddx bead create`.
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

Branches containing execute-bead or execute-loop commits carry a
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

Branches containing `ddx agent execute-bead` or `execute-loop` commits
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
