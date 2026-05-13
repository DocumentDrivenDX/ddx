WIRE cli/internal/evidence/read.go:24 OversizeError.Error — runtime hard-fail prompt reads surface the wrapped oversize class through prompt_file_read.go.
WIRE cli/internal/evidence/read.go:29 OversizeError.Unwrap — runtime hard-fail prompt reads expose ErrOversize for errors.Is/As in prompt_file_read.go and tests.
WIRE cli/internal/evidence/read.go:70 ReadFileHardFail — production prompt-file ingestion uses the hard-fail reader in cli/internal/agent/prompt_file_read.go.
WIRE cli/internal/evidence/sections.go:51 FitSections — production review prompt assembly uses AssembleInline in cli/internal/server/review_session_prompt.go.
WIRE cli/internal/evidence/sections.go:139 capContent — helper used by FitSections in the live prompt assembly path.
WIRE cli/internal/evidence/sections.go:153 trimToLineBudget — helper used by FitSections when fitting sections under budget.
DELETE internal/evidence/strategy.go:19 AssembleRefOnly — no current production caller or symbol remains in the tree; obsolete ref-only assembly path is already removed.
WIRE internal/evidence/strategy.go:50 AssembleInline — production review prompt assembly uses it in cli/internal/server/review_session_prompt.go and cli/internal/agent/execute_bead_review.go.
