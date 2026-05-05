WIRE internal/evidence/read.go:24 OversizeError.Error — `readPromptFileBounded` now formats oversize failures through `evidence.ReadFileHardFail`, so the error string is emitted on the production prompt-file path.
WIRE internal/evidence/read.go:29 OversizeError.Unwrap — `readPromptFileBounded` checks `errors.Is(err, evidence.ErrOversize)` after `ReadFileHardFail`, so the wrapped sentinel is traversed in production.
WIRE internal/evidence/read.go:70 ReadFileHardFail — `cli/internal/agent/prompt_file_read.go:27-38` now routes prompt-file ingress through the hard-fail reader instead of the clamped reader.
WIRE internal/evidence/sections.go:51 FitSections — `cli/internal/agent/execute_bead_review.go:317-325` now routes the final review-prompt join through `evidence.AssembleInline`, which calls `FitSections`.
WIRE internal/evidence/sections.go:139 capContent — same `AssembleInline` path above makes the per-section cap helper part of the production call graph.
WIRE internal/evidence/sections.go:153 trimToLineBudget — same `AssembleInline` path above makes the line-budget trim helper reachable from production.
DELETE internal/evidence/strategy.go:19 AssembleRefOnly — removed as obsolete; no production call site existed and execute-bead prompt assembly already renders governing refs directly.
WIRE internal/evidence/strategy.go:50 AssembleInline — `cli/internal/agent/execute_bead_review.go:317-325` now uses the shared inline assembler for the final prompt join.
