package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// SupervisorRegistry owns one WorkerSupervisor per registered project root.
// The server's reconcile loop walks this registry so each project can manage
// its own desired.json independently.
type SupervisorRegistry struct {
	mu sync.Mutex

	supervisors map[string]*WorkerSupervisor
	state       *ServerState

	// managerFor constructs the WorkerManager used for a project root.
	// Tests replace this so they can inject deterministic factories.
	managerFor func(projectRoot string) *WorkerManager

	managedLaunch bool
}

func newSupervisorRegistry(state *ServerState, managerFor func(projectRoot string) *WorkerManager) *SupervisorRegistry {
	if managerFor == nil {
		managerFor = func(projectRoot string) *WorkerManager {
			return NewWorkerManager(projectRoot)
		}
	}
	return &SupervisorRegistry{
		supervisors: map[string]*WorkerSupervisor{},
		state:       state,
		managerFor:  managerFor,
	}
}

// EnableManagedLaunch applies the subprocess-backed launch path to existing
// and future managers created by the registry.
func (r *SupervisorRegistry) EnableManagedLaunch() {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.managedLaunch = true
	supervisors := make([]*WorkerSupervisor, 0, len(r.supervisors))
	for _, sup := range r.supervisors {
		supervisors = append(supervisors, sup)
	}
	r.mu.Unlock()

	for _, sup := range supervisors {
		if sup == nil || sup.manager == nil {
			continue
		}
		sup.manager.enableManagedLaunch()
	}
}

func (r *SupervisorRegistry) getOrCreate(projectRoot string) *WorkerSupervisor {
	if r == nil {
		return nil
	}

	canonical := canonicalizePath(projectRoot)
	if canonical == "" {
		canonical = projectRoot
	}

	r.mu.Lock()
	if sup, ok := r.supervisors[canonical]; ok {
		r.mu.Unlock()
		return sup
	}
	managedLaunch := r.managedLaunch
	managerFor := r.managerFor
	r.mu.Unlock()

	manager := managerFor(canonical)
	if manager == nil {
		return nil
	}
	if managedLaunch {
		manager.enableManagedLaunch()
	}

	sup := NewWorkerSupervisor(manager)

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.supervisors[canonical]; ok {
		return existing
	}
	r.supervisors[canonical] = sup
	return sup
}

// ReconcileAll runs each registered project's supervisor independently so one
// project's error does not stop the others.
func (r *SupervisorRegistry) ReconcileAll() error {
	if r == nil || r.state == nil {
		return nil
	}

	projects := r.state.GetProjects()
	var errs []error
	for _, proj := range projects {
		sup := r.getOrCreate(proj.Path)
		if sup == nil {
			errs = append(errs, fmt.Errorf("supervisor registry: no supervisor for %s", proj.Path))
			continue
		}
		if err := sup.Reconcile(); err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", proj.Path, err))
		}
	}
	return errors.Join(errs...)
}

// Shutdown stops every cached project's manager.
func (r *SupervisorRegistry) Shutdown() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	supervisors := make([]*WorkerSupervisor, 0, len(r.supervisors))
	for _, sup := range r.supervisors {
		supervisors = append(supervisors, sup)
	}
	r.supervisors = map[string]*WorkerSupervisor{}
	r.mu.Unlock()

	var firstErr error
	for _, sup := range supervisors {
		if sup == nil || sup.manager == nil {
			continue
		}
		if err := sup.manager.Shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// seedSupervisedProjects registers any project roots that already have a
// desired.json, using the current registry plus any explicit env search roots.
func seedSupervisedProjects(state *ServerState) {
	if state == nil {
		return
	}

	var searchRoots []string
	for _, proj := range state.GetProjects(true) {
		parent := filepath.Dir(proj.Path)
		if parent != "" && parent != "." {
			searchRoots = append(searchRoots, parent)
		}
	}
	searchRoots = append(searchRoots, parseSupervisedProjectsEnv(os.Getenv("DDX_SUPERVISED_PROJECTS"))...)

	seen := map[string]struct{}{}
	for _, root := range searchRoots {
		canonical := canonicalizePath(root)
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		seedSupervisedProjectRoot(state, canonical)
	}
}

func seedSupervisedProjectRoot(state *ServerState, root string) {
	maybeRegister := func(projectRoot string) {
		if supervisedDesiredStateExists(projectRoot) {
			state.RegisterProject(projectRoot)
		}
	}

	maybeRegister(root)

	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		maybeRegister(filepath.Join(root, entry.Name()))
	}
}

func supervisedDesiredStateExists(projectRoot string) bool {
	path, ok := ddxroot.ExistingJoinProject(context.Background(), projectRoot, "workers", "desired.json")
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func parseSupervisedProjectsEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
