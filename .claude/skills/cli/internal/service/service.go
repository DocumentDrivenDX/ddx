// Package service manages the DDx server as a platform-native user service
// (systemd user unit on Linux, launchd LaunchAgent on macOS).
package service

import (
	"fmt"
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
