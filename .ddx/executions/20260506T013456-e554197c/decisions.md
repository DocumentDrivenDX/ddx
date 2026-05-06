# Decisions for ddx-ae4b7393

- `OversizeError.Error` — WIRE; surfaced by `cli/internal/evidence/read.go:24-29` via `ReadFileHardFail` and the agent prompt-file path in `cli/internal/agent/prompt_file_read.go:27-38`.
- `OversizeError.Unwrap` — WIRE; same hard-fail path as above, enabling `errors.Is(err, evidence.ErrOversize)` in `cli/internal/agent/prompt_file_read.go:31-36`.
- `ReadFileHardFail` — WIRE; called from `cli/internal/agent/prompt_file_read.go:27-38`, which is used by the agent run paths that read `--prompt` files.
- `FitSections` — WIRE; used by `cli/internal/evidence/strategy.go:23-40`, `cli/internal/agent/execute_bead_review.go:211-337`, and `cli/internal/server/review_session_prompt.go:56-90`.
- `capContent` — WIRE; internal helper on the `FitSections` path in `cli/internal/evidence/sections.go:139-148`.
- `trimToLineBudget` — WIRE; internal helper on the `FitSections` path in `cli/internal/evidence/sections.go:150-165`.
- `AssembleRefOnly` — DELETE; no symbol exists in the current tree, and the `review`/`server` prompt builders now route through `AssembleInline` instead.
- `AssembleInline` — WIRE; used by `cli/internal/agent/execute_bead_review.go:328-337` and `cli/internal/server/review_session_prompt.go:73-90`.
