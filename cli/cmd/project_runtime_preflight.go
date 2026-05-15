package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// projectRuntimePreflightResult holds the outcome of a lightweight project-local
// skill layout check run before long-running entrypoints.
type projectRuntimePreflightResult struct {
	ProjectRoot          string
	MissingBeadLifecycle bool
	CheckedPaths         []string // both candidate paths that were checked
	LegacySymlinkDirs    []string // non-empty when legacy DDx skill symlinks are detected (FEAT-015)
}

// checkProjectRuntimePreflight performs a lightweight check of the project-local
// skill layout. It verifies that the bead-lifecycle skill is present in at least
// one of the two supported skill directories and detects legacy symlink installs.
// It does not run the full ddx doctor check suite.
func checkProjectRuntimePreflight(projectRoot string) projectRuntimePreflightResult {
	checked := []string{
		filepath.Join(projectRoot, ".agents", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
		filepath.Join(projectRoot, ".claude", "skills", "ddx", "bead-lifecycle", "SKILL.md"),
	}
	missing := true
	for _, p := range checked {
		if _, err := os.Stat(p); err == nil {
			missing = false
			break
		}
	}
	return projectRuntimePreflightResult{
		ProjectRoot:          projectRoot,
		MissingBeadLifecycle: missing,
		CheckedPaths:         checked,
		LegacySymlinkDirs:    legacySkillSymlinkDirs(projectRoot),
	}
}

// emitPreflightWarning writes a warn-level preflight message to w when the
// project-local skill layout is degraded. Used by work/try before dispatch.
// Callers are responsible for emitting at most once per process via sync.Once.
func emitPreflightWarning(w io.Writer, result projectRuntimePreflightResult) {
	if !result.MissingBeadLifecycle && len(result.LegacySymlinkDirs) == 0 {
		return
	}
	fmt.Fprintf(w, "preflight warning: project-local skill layout is degraded (project root: %s)\n", result.ProjectRoot)
	if result.MissingBeadLifecycle {
		for _, p := range result.CheckedPaths {
			fmt.Fprintf(w, "  checked: %s\n", p)
		}
	}
	for _, dir := range result.LegacySymlinkDirs {
		fmt.Fprintf(w, "  legacy DDx skill symlink under %s\n", dir)
	}
	fmt.Fprintf(w, "  run: ddx update --force\n")
	fmt.Fprintf(w, "  run: ddx doctor\n")
}

// emitServerPreflightDiagnostics writes degraded startup diagnostics to w.
// Called by the server RunE before ListenAndServeTLS to surface skill layout
// issues without blocking server startup (missing bead-lifecycle is not fatal
// for the server).
func emitServerPreflightDiagnostics(w io.Writer, result projectRuntimePreflightResult) {
	if !result.MissingBeadLifecycle && len(result.LegacySymlinkDirs) == 0 {
		return
	}
	fmt.Fprintf(w, "DDx server: degraded skill layout (project root: %s)\n", result.ProjectRoot)
	if result.MissingBeadLifecycle {
		fmt.Fprintf(w, "  bead-lifecycle skill not found; lifecycle hooks will not fire\n")
		for _, p := range result.CheckedPaths {
			fmt.Fprintf(w, "  checked: %s\n", p)
		}
	}
	for _, dir := range result.LegacySymlinkDirs {
		fmt.Fprintf(w, "  legacy DDx skill symlink under %s\n", dir)
	}
	fmt.Fprintf(w, "  run: ddx update --force\n")
	fmt.Fprintf(w, "  run: ddx doctor\n")
}
