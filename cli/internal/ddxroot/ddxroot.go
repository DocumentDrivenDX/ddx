package ddxroot

import (
	"context"
	"path/filepath"
)

// Path returns the DDx state root for projectRoot.
//
// This accessor intentionally preserves the current in-tree `.ddx` location
// while giving follow-on beads one place to add convention-mode fallback.
func Path(_ context.Context, projectRoot string) string {
	return filepath.Join(projectRoot, ".ddx")
}
