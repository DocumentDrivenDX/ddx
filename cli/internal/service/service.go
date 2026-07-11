// Package service manages the DDx server as a platform-native user service
// (systemd user unit on Linux, launchd LaunchAgent on macOS).
package service

import (
	"fmt"
	"runtime"
)

// maxDefaultCPUCores bounds the default aggregate CPU quota so a single
// server-owned execution tree (server + managed workers + provider/build
// descendants) cannot saturate a large host by default.
const maxDefaultCPUCores = 4

// ResourcePolicy bounds the aggregate CPU consumption of the server-owned
// execution tree: the ddx-server process plus every managed worker,
// provider subprocess, and project build it spawns.
type ResourcePolicy struct {
	// CPUQuotaPercent caps aggregate CPU time as a percentage of one core
	// (e.g. 400 permits up to 4 cores' worth of CPU time). Zero disables
	// the systemd CPUQuota= directive (unbounded).
	CPUQuotaPercent int
	// CPUWeight sets the systemd/cgroup v2 CPU scheduling weight (1-10000,
	// systemd default 100) relative to other services. Zero leaves the
	// systemd default in effect.
	CPUWeight int
	// Nice sets the scheduling priority (-20 to 19) for the server and its
	// descendants.
	Nice int
}

// Validate reports an error if the resource policy values fall outside the
// ranges systemd and the kernel scheduler accept.
func (p ResourcePolicy) Validate() error {
	if p.CPUQuotaPercent < 0 {
		return fmt.Errorf("cpu quota percent must be >= 0, got %d", p.CPUQuotaPercent)
	}
	if p.CPUWeight != 0 && (p.CPUWeight < 1 || p.CPUWeight > 10000) {
		return fmt.Errorf("cpu weight must be between 1 and 10000, got %d", p.CPUWeight)
	}
	if p.Nice < -20 || p.Nice > 19 {
		return fmt.Errorf("nice must be between -20 and 19, got %d", p.Nice)
	}
	return nil
}

// DefaultResourcePolicy returns a conservative aggregate CPU budget bounded
// by the host's logical CPU count: at most maxDefaultCPUCores cores' worth
// of CPU quota, or fewer on smaller hosts.
func DefaultResourcePolicy() ResourcePolicy {
	cores := runtime.NumCPU()
	if cores > maxDefaultCPUCores {
		cores = maxDefaultCPUCores
	}
	if cores < 1 {
		cores = 1
	}
	return ResourcePolicy{
		CPUQuotaPercent: cores * 100,
		CPUWeight:       100,
		Nice:            10,
	}
}

// Config holds the parameters needed to install a service.
type Config struct {
	ExecPath       string
	ProjectRoot    string
	WorkDir        string
	LogPath        string
	Env            map[string]string
	ResourcePolicy ResourcePolicy
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
