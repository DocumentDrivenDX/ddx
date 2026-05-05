OversizeError.Error: WIRE — reachable via `internal/agent/prompt_file_read.go:29` because `ReadFileHardFail` returns `*OversizeError` and callers format the error for the operator.
OversizeError.Unwrap: WIRE — reachable via `errors.Is(err, evidence.ErrOversize)` in `internal/agent/prompt_file_read.go:31`.
ReadFileHardFail: WIRE — production prompt-file ingress uses `readPromptFileBounded` in `internal/agent/compare_adapter.go`, `internal/agent/execute_bead.go`, `internal/agent/runner.go`, and `internal/agent/service_run.go`, which all call this helper.
FitSections: WIRE — reachable via `internal/evidence/strategy.go:23` from `AssembleInline`, and via `internal/agent/execute_bead_review.go:329` and `internal/server/review_session_prompt.go:73`.
capContent: WIRE — internal helper called only from `FitSections` in `internal/evidence/sections.go`.
trimToLineBudget: WIRE — internal helper called only from `FitSections` in `internal/evidence/sections.go`.
AssembleRefOnly: DELETE — no definition or production caller remains in the tree; this symbol is absent from the current package and should stay removed.
AssembleInline: WIRE — production review-session prompt assembly calls `internal/server/review_session_prompt.go:73` and review prompt assembly in `internal/agent/execute_bead_review.go:329`.
