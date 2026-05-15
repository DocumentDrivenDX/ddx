WIRE cli/cmd/agent_execute_loop_escalation.go:63 investigationRetryInitialMinPower — normal execute-loop dispatch now calls the non-inference wrapper from cli/cmd/execute_loop_shared.go, keeping the runtime helper in the production graph.
DELETE cli/cmd/install.go:726 copyDirTree — local installs symlink the plugin root and use registry.CopyScriptFromRoot for script copies, so this tree-copy helper had no production caller.
DELETE cli/cmd/install.go:881 shouldSkipLocalInstallPath — this skip helper only existed to support the deleted copyDirTree path.
DELETE cli/cmd/shared_helpers.go:111 resolveWorktree — no production caller remained; execute-bead and landing worktree creation already live in internal/agent.
DELETE cli/cmd/work_status.go:177 fixedScanner.Scan — moved the workerstatus.Scanner stub into cli/cmd/work_status_test.go because it is test-only scaffolding.
