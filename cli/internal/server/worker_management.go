package server

import (
	"errors"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// ErrManagementDisabled is the typed result returned when a managed-worker
// spawn or enable mutation is refused because server.manage_workers is off
// (Phase 3 WB-1 observer demotion). Callers should use errors.Is.
var ErrManagementDisabled = errors.New("management_disabled")

// packageUnderTest is set true only by this package's TestMain so unit tests
// that omit server.manage_workers still exercise spawn machinery. Production
// binaries leave it false: omission means disabled (observer demotion).
var packageUnderTest bool

// ManageWorkersEnabled reports whether server-managed DDx worker spawning is
// allowed for projectRoot.
//
// Explicit server.manage_workers true/false always wins. When the field is
// omitted: production defaults to disabled; this package's unit tests default
// to enabled so legacy spawn suites stay green. Callers that need the
// observer gate must write manage_workers: false.
func ManageWorkersEnabled(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Server == nil || cfg.Server.ManageWorkers == nil {
		return packageUnderTest
	}
	return *cfg.Server.ManageWorkers
}

// manageWorkersEnabled is the single gate used by spawn entry points. An
// explicit override on the manager (tests / temporary re-enable) wins over
// config; otherwise the project config authority is consulted.
func (m *WorkerManager) manageWorkersEnabled() bool {
	if m == nil {
		return false
	}
	if m.manageWorkersOverride != nil {
		return *m.manageWorkersOverride
	}
	return ManageWorkersEnabled(m.projectRoot)
}

// SetManageWorkers overrides the server.manage_workers gate for this manager.
// Pass nil to clear the override and fall back to project config. Tests use
// this to opt into spawn without rewriting config.yaml.
func (m *WorkerManager) SetManageWorkers(enabled *bool) {
	if m == nil {
		return
	}
	if enabled == nil {
		m.manageWorkersOverride = nil
		return
	}
	v := *enabled
	m.manageWorkersOverride = &v
}

// RequireManageWorkers returns ErrManagementDisabled when managed spawning is
// not allowed for this manager.
func (m *WorkerManager) RequireManageWorkers() error {
	if m != nil && m.manageWorkersEnabled() {
		return nil
	}
	return ErrManagementDisabled
}

// ZeroDesiredManagedState clears desired spawn intent (desired_count=0,
// restart disabled) for projectRoot without signaling any OS/provider
// processes. Missing desired state is a no-op. Safe while management is
// disabled — SaveDesiredState allows DesiredCount==0 without the gate.
func ZeroDesiredManagedState(projectRoot string) error {
	if projectRoot == "" {
		return fmt.Errorf("zero desired managed state: project root is required")
	}
	m := NewWorkerManager(projectRoot)
	sup := NewWorkerSupervisor(m)
	state, err := sup.LoadDesiredState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if state.DesiredCount == 0 && !state.Restart.Enabled {
		return nil
	}
	state.DesiredCount = 0
	state.Restart.Enabled = false
	return sup.SaveDesiredState(&state)
}

// DisableWorkerManagement zeros desired managed-worker state for every
// registered project when management is disabled. It never signals provider
// processes — only the durable desired.json spawn intent is cleared.
func (s *Server) applyManagementDisabledPolicy() {
	if s == nil {
		return
	}
	roots := []string{}
	if s.WorkingDir != "" {
		roots = append(roots, s.WorkingDir)
	}
	if s.state != nil {
		for _, p := range s.state.GetProjects() {
			if p.Path == "" {
				continue
			}
			roots = append(roots, p.Path)
		}
	}
	seen := map[string]struct{}{}
	for _, root := range roots {
		canonical := canonicalizePath(root)
		if canonical == "" {
			canonical = root
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		if ManageWorkersEnabled(canonical) {
			continue
		}
		_ = ZeroDesiredManagedState(canonical)
	}
}
