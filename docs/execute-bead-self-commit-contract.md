# Execute-Bead Self-Commit Contract

`ddx try` / legacy `ddx agent execute-bead` runs an agent inside an isolated
worktree. The agent must produce exactly one of two durable signals before it
exits:

- a git commit containing the completed implementation and tests
- `.ddx/executions/<attempt-id>/no_changes_rationale.txt`

The rationale file is only for attempts that cannot produce a valid commit. It
must explain what is done, what is blocking, and what a follow-up attempt needs.
DDx treats that as a legitimate `no_changes` signal.

If the agent exits without a commit and without the rationale file, DDx records
`no_evidence_produced`. That status is a harness failure, not success and not
ordinary no-changes. DDx does not land an execution-evidence-only iteration, and
the reviewer does not need to spend a pass discovering that the diff contains
only `.ddx/executions` metadata.

When the worktree still has uncommitted paths at that point, DDx includes those
paths in `result.json` as `no_evidence_paths` so an operator can diagnose
whether the agent edited ignored files, failed to commit, or wrote outside the
intended scope.
