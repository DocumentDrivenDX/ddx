WIRE ReadFileHardFail is live via readPromptFileBounded in cli/internal/agent/prompt_file_read.go:27-32.
WIRE ReadFileClamped is live via cli/internal/agent/execute_bead_review.go:284-299.
WIRE OversizeError.Error is live via prompt-file oversize formatting in cli/internal/evidence/read.go:24-27 and tests in cli/internal/evidence/read_test.go:67-82.
WIRE OversizeError.Unwrap is live via errors.Is assertions in cli/internal/evidence/read_test.go:67-74 and prompt-file ingress in cli/internal/agent/prompt_file_read.go:27-35.
WIRE FitSections is live via AssembleInline in cli/internal/evidence/strategy.go:23-40 and its caller in cli/internal/agent/execute_bead_review.go:329-333.
WIRE capContent is live via FitSections in cli/internal/evidence/sections.go:51-137.
WIRE trimToLineBudget is live via FitSections in cli/internal/evidence/sections.go:96-112.
DELETE AssembleRefOnly is absent from the current tree; the symbol was removed before this pass and no production call site remains.
