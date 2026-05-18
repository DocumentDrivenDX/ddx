// Package service manages the DDx server as a platform-native user service
// (systemd user unit on Linux, launchd LaunchAgent on macOS).
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config holds the parameters needed to install a service.
type Config struct {
	ExecPath string
	WorkDir  string
	LogPath  string
	Env      map[string]string
}

// Backend manages a service's lifecycle on a specific platform.
type Backend interface {
	Install(cfg Config) error
	Uninstall() error
	Start() error
	Stop() error
	Status() error
}

// DefaultWorkDir is the neutral cwd for the per-user ddx-server service.
// Project roots are registered in XDG state; the service process itself must
// not be bound to whichever project happened to run install.
func DefaultWorkDir() (string, error) {
	return os.UserHomeDir()
}

// DefaultLogPath is the user-scoped log path for the per-user service.
func DefaultLogPath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", "ddx-server", "ddx-server.log"), nil
	default:
		return filepath.Join(xdgDataHome(), "ddx", "logs", "ddx-server.log"), nil
	}
}

// New returns the service backend for the current platform.
func New() (Backend, error) {
	switch runtime.GOOS {
	case "linux":
		return &systemdBackend{}, nil
	case "darwin":
		return &launchdBackend{}, nil
	default:
		return nil, fmt.Errorf("service management not supported on %s", runtime.GOOS)
	}
}

func xdgDataHome() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ddx-xdg")
	}
	return filepath.Join(home, ".local", "share")
}
