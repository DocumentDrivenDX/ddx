package agent

// prompt_file_read.go centralizes the bounded read for user-supplied
// --prompt files (FEAT-022 §8). All agent-side prompt-file readers
// route through readPromptFileBounded so that oversize inputs hard-fail
// with an actionable error rather than silently truncating or being
// shipped to a provider as-is.

import (
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
	cap := promptFileCapBytes
	content, truncated, originalBytes, err := evidence.ReadFileClamped(path, cap)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf(
			"prompt file %q exceeds cap: observed %d bytes, cap %d bytes (configurable via .ddx/config.yaml:evidence_caps.max_prompt_bytes)",
			path, originalBytes, cap)
	}
	return content, nil
}
