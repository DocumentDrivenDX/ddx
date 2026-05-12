# Execute-Bead Protocol

`legacy agent execute-bead <id>` creates an isolated git worktree from the base revision,
runs the configured harness, and either merges the result or preserves the branch on timeout.

The merge path calls `git merge --no-ff` to record execution history as a non-fast-forward
commit. The preserved branch is named `execute-bead-<run-id>` and stored in the tracker.
