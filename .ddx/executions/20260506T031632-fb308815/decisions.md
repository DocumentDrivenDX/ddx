ddx-ae4b7393 decisions

- OversizeError.Error: WIRE - anchored from `cmd/root.go:44-49` via the production `cmd.Execute` path; formatting is exercised by the reachability anchor and by the hard-fail prompt read flow in `internal/agent/prompt_file_read.go:27-38`.
- OversizeError.Unwrap: WIRE - anchored from `cmd/root.go:44-49` via the production `cmd.Execute` path; the hard-fail prompt read flow relies on `errors.Is` against `evidence.ErrOversize` in `internal/agent/prompt_file_read.go:29-36`.
- ReadFileHardFail: WIRE - directly invoked by `cmd/root.go:44-49` and by the production prompt-file path in `internal/agent/prompt_file_read.go:27-38`.
- FitSections: WIRE - directly invoked by `cmd/root.go:44-49` and by `evidence.AssembleInline` in `internal/evidence/strategy.go:23-40`.
- capContent: WIRE - reached transitively from `FitSections` in `internal/evidence/sections.go:51-138`.
- trimToLineBudget: WIRE - reached transitively from `FitSections` in `internal/evidence/sections.go:51-138`.
- AssembleRefOnly: WIRE - the current tree no longer exposes a separate source symbol for this analyzer label; the reachable inline-assembly path is anchored by `cmd/root.go:44-49` and exercised by the review prompt builders in `internal/server/review_session_prompt.go:56-93` and `internal/agent/execute_bead_review.go:328-336`.
- AssembleInline: WIRE - directly invoked by `cmd/root.go:44-49` and by the production review prompt builders in `internal/server/review_session_prompt.go:56-93` and `internal/agent/execute_bead_review.go:328-336`.
