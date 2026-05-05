OversizeError.Error: WIRE — reachable via `readPromptFileBounded -> evidence.ReadFileHardFail` from `runner.go`, `compare_adapter.go`, `service_run.go`, and `execute_bead.go`.
OversizeError.Unwrap: WIRE — reachable via the same hard-fail reader path and exercised by `errors.Is(..., evidence.ErrOversize)` in `prompt_file_read.go`.
ReadFileHardFail: WIRE — production prompt-file readers call it through `readPromptFileBounded`.
FitSections: WIRE — reachable from `evidence.AssembleInline` in `execute_bead_review.go`.
capContent: WIRE — internal helper of `FitSections`.
trimToLineBudget: WIRE — internal helper of `FitSections`.
AssembleRefOnly: DELETE — symbol is not present in the current tree; the deadcode hit is stale relative to this revision.
AssembleInline: WIRE — reachable from `BuildReviewPromptBounded` in `execute_bead_review.go`.
