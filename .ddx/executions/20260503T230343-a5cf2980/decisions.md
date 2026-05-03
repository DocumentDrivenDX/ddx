# Decisions log — ddx-0131ebf0

Backfill of production-reachability violations in package `cmd` (69 listed symbols + 1 in-flight `formatTryResult` flagged by current deadcode RTA but not in the original bead list).

For every symbol below the rule was: WIRE if a production caller exists or could be added; DELETE if obsolete/legacy. None of the 69 had a production caller; all are legacy wrappers, *InDir helpers superseded by `CommandFactory.WorkingDir` methods, or test-only helpers in non-test files.

| Symbol | Decision | Rationale |
| --- | --- | --- |
| cmd/agent_cmd.go:858 — harnessHealthyViaService | DELETE | superseded by upstream-service health check; no callers |
| cmd/agent_cmd.go:1996 — formatTryResult | DELETE | added but never wired (flagged by current deadcode; not on bead's symbol list but cleared for AC #3) |
| cmd/agent_cmd.go:2040 — CommandFactory.executeLoopWithServer | DELETE | server-submission path replaced by direct execute-loop; no callers |
| cmd/agent_execute_bead.go:22 — loadExecutionsMirrorConfig | DELETE | mirror config now read inline by orchestrator; no callers |
| cmd/agent_metrics.go:349 — computeReviewOutcomes | DELETE | thin wrapper around computeReviewOutcomesReport; only test referenced — test now calls Report directly |
| cmd/agent_usage.go:246 — aggregateUsage | DELETE | dead chain; only called itself via aggregateUsageAggs |
| cmd/agent_usage.go:289 — aggregateUsageAggs | DELETE | dead chain (only caller was aggregateUsage) |
| cmd/agent_usage.go:480 — readUsageSessionRecords | DELETE | no callers |
| cmd/command_factory.go:84 — NewCommandFactoryWithViper | DELETE | only used by test_harness.go (also deleted) |
| cmd/command_factory.go:96 — CommandFactory.withWorkingDir | DELETE | sibling-factory pattern abandoned; no callers |
| cmd/command_factory.go:575 — getLibraryPathFromEnv | DELETE | only called by getLibraryPath (also dead) |
| cmd/config.go:143 — runConfig (legacy free-fn) | DELETE | factory-method runConfig is the wired runE; help-test now uses inline stub |
| cmd/config.go:387 — getConfigValueWithWorkingDir | DELETE | superseded by configGet; no callers |
| cmd/config.go:433 — copyFile | DELETE | only used by copyDir (also dead) and 1 test (now inlined) |
| cmd/config.go:455 — showConfigFiles | DELETE | superseded by CommandFactory.outputConfigFiles |
| cmd/config.go:512 — resyncMetaPromptAfterConfigChange | DELETE | no callers |
| cmd/config.go:522 — syncMetaPromptWithConfig | DELETE | no callers |
| cmd/errors.go:41 — HandleError | DELETE | exit-code routing handled by main()'s ExitError check; no callers |
| cmd/errors.go:62 — CheckPersonaNotFound | DELETE | no callers |
| cmd/errors.go:75 — CheckNoConfig | DELETE | no callers |
| cmd/init.go:451 — copyDir | DELETE | no callers |
| cmd/init.go:478 — initializeSynchronizationPure | DELETE | only ref was 1 test block (now removed) |
| cmd/init.go:497 — initializeSynchronization | DELETE | no callers |
| cmd/init.go:515 — isValidRepositoryURL | DELETE | only called by initializeSynchronizationPure (also dead) |
| cmd/init.go:547 — fileExistsInDir | DELETE | no callers |
| cmd/init.go:636 — validateGitRepository | DELETE | superseded by validateGitRepo direct call from factory |
| cmd/log.go:45 — runLog (legacy free-fn) | DELETE | factory-method runLog is the wired runE |
| cmd/persona.go:83 — runPersona (legacy free-fn) | DELETE | factory-method runPersona is the wired runE |
| cmd/persona.go:877 — savePersonaConfig | DELETE | no callers |
| cmd/persona.go:1003 — IsLibraryReadOnly | DELETE | no callers |
| cmd/root.go:41 — isInitialized | DELETE | no callers |
| cmd/root.go:48 — getLibraryPath | DELETE | no callers |
| cmd/test_harness.go:17–264 — WithIsolatedDirectory, GetCommandInDirectory, NewTestHarness, TestHarness.* (22 funcs total) | DELETE | unused test helpers in non-test file; no test references |
| cmd/update.go:203 — isInitializedInDir | DELETE | dead InDir island |
| cmd/update.go:212 — loadConfigFromWorkingDirForUpdate | DELETE | dead InDir island |
| cmd/update.go:225 — validateUpdateStrategy | DELETE | dead InDir island |
| cmd/update.go:247 — checkForUpdatesInDir | DELETE | dead InDir island |
| cmd/update.go:258 — previewUpdateInDir | DELETE | dead InDir island |
| cmd/update.go:274 — synchronizeWithUpstreamInDir | DELETE | dead InDir island |
| cmd/update.go:284 — detectConflictsInDir | DELETE | dead InDir island |
| cmd/update.go:345 — handleUpdateAbortInDir | DELETE | dead InDir island |
| cmd/update.go:397 — handleInteractiveResolutionInDir | DELETE | dead InDir island |
| cmd/update.go:409 — executeUpdateInDir | DELETE | dead InDir island |
| cmd/update.go:453 — createBackupInDir | DELETE | dead InDir island |
| cmd/update.go:477 — copyDirForRestore | DELETE | dead InDir island |
| cmd/update.go:642 — isBinaryFileForUpdate | DELETE | only called by detectConflictsInDir (also dead) |
| cmd/update.go:654 — extractConflictContentForUpdate | DELETE | only called by detectConflictsInDir (also dead) |
| cmd/update.go:683 — runUpdate (legacy free-fn) | DELETE | factory-method runUpdate is the wired runE |
| cmd/update.go:701 — syncMetaPrompt | DELETE | no callers |

## Tests updated

- cmd/config_test.go:247 — replaced `RunE: runConfig` with inline no-op stub (test only validates `--help`).
- cmd/agent_metrics_review_evidence_test.go (2 sites) — switched from `computeReviewOutcomes(dir)` wrapper to `computeReviewOutcomesReport(dir, 0, time.Time{})` and `report.Rows`.
- cmd/init_functional_test.go — removed `initializeSynchronizationPure` block from `TestBusinessLogicIndependence` (kept `validateGitRepo` block); dropped now-unused `internal/config` import.
- cmd/installation_acceptance_test.go:193 — inlined `copyFile` as a local closure (`os.ReadFile`+`os.WriteFile`).

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` (from cli/): no remaining `cmd/...` entries (AC #3).
- `go test ./...` (from cli/): all packages pass except 7 pre-existing failures in `cmd/` that are unrelated to this change (TestReviewEvidenceApproveAttributesToTier and TestAcceptance_US028..US034 / TestInstallationPerformance — confirmed failing on base rev 6edf4539 prior to any edit).

## Pending follow-ups

None. Every listed symbol resolved (no `// wiring:pending` annotations needed).
