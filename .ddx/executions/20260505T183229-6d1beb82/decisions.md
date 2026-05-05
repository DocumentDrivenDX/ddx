internal/evidence/read.go:24 OversizeError.Error - WIRE: used by `ReadFileHardFail` callers to surface actionable oversize errors; see `cli/internal/agent/prompt_file_read.go:29-34`.
internal/evidence/read.go:29 OversizeError.Unwrap - WIRE: required for `errors.Is(err, evidence.ErrOversize)` in `cli/internal/agent/prompt_file_read.go:29-34`.
internal/evidence/read.go:70 ReadFileHardFail - WIRE: production prompt-file path routes through it in `cli/internal/agent/prompt_file_read.go:25-35`.
internal/evidence/sections.go:51 FitSections - WIRE: production prompt assembly uses it via `AssembleInline` in `cli/internal/agent/execute_bead_review.go:329` and `cli/internal/server/review_session_prompt.go:73`.
internal/evidence/sections.go:139 capContent - WIRE: private helper reached from `FitSections` in the same production assembly path.
internal/evidence/sections.go:153 trimToLineBudget - WIRE: private helper reached from `FitSections` in the same production assembly path.
internal/evidence/strategy.go:19 AssembleRefOnly - DELETE: symbol is absent from the current tree; the live production path uses `AssembleInline` instead.
internal/evidence/strategy.go:50 AssembleInline - WIRE: production review assembly calls it in `cli/internal/agent/execute_bead_review.go:329` and `cli/internal/server/review_session_prompt.go:73`.
