package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

const (
	// FizeauDisableClaudeTUIDefaultEnv is Fizeau's documented routing switch
	// for disabling the interactive claude-tui preference on the shared Claude
	// surface.
	FizeauDisableClaudeTUIDefaultEnv = "FIZEAU_DISABLE_CLAUDE_TUI_DEFAULT"
	claudeTUIHarness                 = "claude-tui"
)

var interactiveTerminalDetector = stdioHasInteractiveTerminal

// InteractiveTerminalAvailable reports whether any standard stream is
// attached to a terminal. A process with no terminal-backed standard stream is
// treated as having no controlling terminal for routing purposes.
func InteractiveTerminalAvailable() bool {
	return interactiveTerminalDetector()
}

func stdioHasInteractiveTerminal() bool {
	for _, stream := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		fd := stream.Fd()
		if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
			return true
		}
	}
	return false
}

// ConfigureHeadlessFizeauRouting disables Fizeau's claude-tui default only
// for the lifetime of a headless work invocation. The returned closure restores
// both the prior value and whether the variable was set at all.
func ConfigureHeadlessFizeauRouting(terminalAvailable bool) (func(), error) {
	if terminalAvailable {
		return func() {}, nil
	}

	prior, wasSet := os.LookupEnv(FizeauDisableClaudeTUIDefaultEnv)
	if err := os.Setenv(FizeauDisableClaudeTUIDefaultEnv, "1"); err != nil {
		return nil, fmt.Errorf("agent: configure headless Fizeau routing: %w", err)
	}

	return func() {
		if wasSet {
			_ = os.Setenv(FizeauDisableClaudeTUIDefaultEnv, prior)
			return
		}
		_ = os.Unsetenv(FizeauDisableClaudeTUIDefaultEnv)
	}, nil
}

// ValidateHarnessTerminal rejects the PTY-only claude-tui harness before
// dispatch when no controlling terminal is attached. Other explicit harness
// pins remain valid in headless workers.
func ValidateHarnessTerminal(harness string, terminalAvailable bool) error {
	if terminalAvailable || !strings.EqualFold(strings.TrimSpace(harness), claudeTUIHarness) {
		return nil
	}
	return fmt.Errorf("agent: harness %q requires a PTY-capable interactive terminal; no controlling terminal is attached", claudeTUIHarness)
}
