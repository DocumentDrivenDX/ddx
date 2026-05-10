Run `ddx bead list --status open` to view pending items.
To drain the queue: ddx work --harness claude.
Execute tests with: cd cli && go test ./internal/docprose/...
Use ddx agent execute-bead <id> --from <rev> to run in a worktree.
