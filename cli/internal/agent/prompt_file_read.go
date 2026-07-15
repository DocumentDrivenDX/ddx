package agent

// prompt_file_read.go centralizes the bounded read for user-supplied
// --prompt files (FEAT-022 §8). All agent-side prompt-file readers
// route through readPromptFileBounded so that oversize inputs hard-fail
// with an actionable error rather than silently truncating or being
// shipped to a provider as-is.

import (
	"errors"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// promptFileCapBytes is the byte cap applied to user-supplied --prompt
// files. It exists as a package-level variable (rather than a literal)
// so tests can lower the cap to keep oversize fixtures small. Production
// callers must not mutate it.
var promptFileCapBytes = evidence.DefaultMaxPromptBytes

// readPromptFileBounded reads path as a user-supplied prompt file,
// hard-failing when the file exceeds the configured prompt cap.
// On oversize the returned error names the source path, observed size,
// configured cap, and points the operator at the config key that
// adjusts the cap (.ddx/config.yaml:evidence_caps.max_prompt_bytes).
func readPromptFileBounded(path string) ([]byte, error) {
	return readPromptFileBoundedWithCap(path, promptFileCapBytes)
}

// readPromptFileBoundedWithCap applies the cap resolved for the caller's DDx
// semantic prompt role. It is kept separate from readPromptFileBounded so
// legacy/default callers and focused tests retain the binary default.
func readPromptFileBoundedWithCap(path string, cap int) ([]byte, error) {
	content, err := evidence.ReadFileHardFail(path, cap, path)
	if err != nil {
		if errors.Is(err, evidence.ErrOversize) {
			return nil, fmt.Errorf(
				"prompt file %q exceeds cap: %v (configure evidence_caps.max_prompt_bytes or evidence_caps.per_role.<role>.max_prompt_bytes in .ddx/config.yaml)",
				path, err)
		}
		return nil, err
	}
	return content, nil
}

func validateInlinePromptCap(source, prompt string, cap int) error {
	if len(prompt) <= cap {
		return nil
	}
	if source == "" {
		source = "inline prompt"
	}
	return fmt.Errorf(
		"%s exceeds prompt cap: observed=%d bytes cap=%d bytes (configure evidence_caps.max_prompt_bytes or evidence_caps.per_role.<role>.max_prompt_bytes in .ddx/config.yaml)",
		source, len(prompt), cap)
}
