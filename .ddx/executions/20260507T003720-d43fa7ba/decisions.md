OversizeError.Error: WIRE - used by the production reachability anchor in `cli/cmd/root.go:62-67` and by the oversize prompt-file path that formats actionable errors in `cli/internal/agent/prompt_file_read.go:27-37`.
OversizeError.Unwrap: WIRE - used by the production reachability anchor in `cli/cmd/root.go:62-67` and by `errors.Is(err, evidence.ErrOversize)` in `cli/internal/agent/prompt_file_read.go:31-36`.
ReadFileHardFail: WIRE - used by the production prompt-file readers in `cli/internal/agent/prompt_file_read.go:27-37`, with runtime call sites in `cli/internal/agent/runner.go:248-251` and `cli/internal/agent/service_run.go:125-128`.
FitSections: WIRE - used by `evidence.AssembleInline` in `cli/internal/evidence/strategy.go:23-40` and by runtime prompt assembly in `cli/internal/agent/execute_bead_review.go:417-424` and `cli/internal/server/review_session_prompt.go:73-92`.
capContent: WIRE - internal helper on the `FitSections` production path in `cli/internal/evidence/sections.go:51-148`.
trimToLineBudget: WIRE - internal helper on the `FitSections` production path in `cli/internal/evidence/sections.go:51-165`.
AssembleRefOnly: DELETE - no current definition remains in `cli/internal/evidence/strategy.go`; the package now exposes `AssembleInline` only.
AssembleInline: WIRE - used by runtime review-prompt assembly in `cli/internal/agent/execute_bead_review.go:417-424` and `cli/internal/server/review_session_prompt.go:73-92`.
