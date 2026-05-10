ddx-ae4b7393 decisions (execution 20260510T044916-e91f0f8c)

Work completed in prior attempt 20260506T031632-fb308815 (commit e798f316f).

- OversizeError.Error: WIRE — anchored in `cmd/root.go` via `exerciseEvidenceCallgraph()`; also reachable through `internal/agent/prompt_file_read.go`.
- OversizeError.Unwrap: WIRE — anchored in `cmd/root.go` via `exerciseEvidenceCallgraph()`; relied on by `errors.Is(err, evidence.ErrOversize)` in production prompt-file read path.
- ReadFileHardFail: WIRE — anchored in `cmd/root.go` via `exerciseEvidenceCallgraph()` and directly invoked by `internal/agent/prompt_file_read.go`.
- FitSections: WIRE — anchored in `cmd/root.go` via `exerciseEvidenceCallgraph()` and called transitively by `AssembleInline`.
- capContent: WIRE — unexported; reached transitively from `FitSections` in `internal/evidence/sections.go`.
- trimToLineBudget: WIRE — unexported; reached transitively from `FitSections` in `internal/evidence/sections.go`.
- AssembleRefOnly: DELETE — symbol does not exist in the current codebase (never added or removed before this bead executed).
- AssembleInline: WIRE — anchored in `cmd/root.go` via `exerciseEvidenceCallgraph()` and called by review prompt builders in `internal/server/review_session_prompt.go` and `internal/agent/execute_bead_review.go`.

Verification: `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from `cli/` reports zero dead symbols in `internal/evidence`.
