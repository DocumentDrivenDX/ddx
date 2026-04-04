package bead

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runHook executes a validation hook if it exists.
// Hook receives bead JSON on stdin. Exit codes:
//
//	0 = ok, 1 = hard error (stderr = message), 2 = warning (stderr printed, proceeds)
func (s *Store) runHook(name string, b *Bead) error {
	hookPath := filepath.Join(s.Dir, "hooks", name)

	info, err := os.Stat(hookPath)
	if os.IsNotExist(err) {
		return nil // no hook installed
	}
	if err != nil {
		return fmt.Errorf("bead: hook stat: %w", err)
	}
	if info.IsDir() {
		return nil
	}
	// Check executable
	if info.Mode()&0o111 == 0 {
		return nil // not executable
	}

	data, err := marshalBead(*b)
	if err != nil {
		return fmt.Errorf("bead: hook marshal: %w", err)
	}

	cmd := exec.Command(hookPath)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr == nil {
		return nil // exit 0 = ok
	}

	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		return fmt.Errorf("bead: hook %s: %w", name, runErr)
	}

	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = "validation failed"
	}

	switch exitErr.ExitCode() {
	case 1:
		return fmt.Errorf("bead: hook %s: %s", name, msg)
	case 2:
		// Warning — print but don't block
		fmt.Fprintf(os.Stderr, "bead: hook %s warning: %s\n", name, msg)
		return nil
	default:
		return fmt.Errorf("bead: hook %s exit %d: %s", name, exitErr.ExitCode(), msg)
	}
}

