package service

import (
	"os"
	"path/filepath"
)

// ServerRuntimeDir returns the XDG-scoped directory used for the installed
// ddx-server service's runtime files: logs, lock files, and other service-
// owned state that should not live in a project checkout.
func ServerRuntimeDir() string {
	return filepath.Join(xdgStateHome(), "ddx", "server")
}

func xdgStateHome() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ddx-state")
	}
	return filepath.Join(home, ".local", "state")
}
