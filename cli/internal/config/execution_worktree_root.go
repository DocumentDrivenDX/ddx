package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

const ExecutionWorktreeRootEnv = "DDX_EXEC_WT_DIR"
const DefaultExecutionTempSubdir = "ddx-exec-wt"

// ExecutionWorktreeRoot returns the configured base directory for isolated
// execute-bead worktrees. Resolution order matches the operator-facing config
// priority: environment override, project config, global config, then caller
// fallback.
func ExecutionWorktreeRoot(projectRoot string) string {
	if env := strings.TrimSpace(os.Getenv(ExecutionWorktreeRootEnv)); env != "" {
		return normalizeExecutionWorktreeRoot(env, projectRoot)
	}
	if projectRoot != "" {
		projectConfig := ddxroot.JoinProject(projectRoot, "config.yaml")
		if root := executionWorktreeRootFromFile(projectConfig, projectRoot); root != "" {
			return root
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		globalConfig := ddxroot.JoinHome(home, "config.yaml")
		if root := executionWorktreeRootFromFile(globalConfig, home); root != "" {
			return root
		}
	}
	return ""
}

// ExecutionTempRoot returns the effective base directory for DDx-owned
// execution worktrees. It applies the configured worktree root when present and
// otherwise falls back to the user's cache directory, not the process temp dir.
func ExecutionTempRoot(projectRoot string) string {
	if root := ExecutionWorktreeRoot(projectRoot); root != "" {
		return root
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "ddx", "exec-wt")
	}
	return LegacyExecutionTempRoot()
}

// LegacyExecutionTempRoot is the pre-centralization $TMPDIR root. New
// execution paths should not use it directly; cleanup and worktree detection
// keep scanning it so older leaked resources are not stranded.
func LegacyExecutionTempRoot() string {
	return filepath.Join(os.TempDir(), DefaultExecutionTempSubdir)
}

// ExecutionScratchRoot returns the directory for short-lived helper scratch
// paths associated with execution. Scratch lives beside the worktree root so a
// configured root such as /var/tmp/ddx-exec-wt keeps all DDx-owned temporary
// paths away from /tmp while still grouping attempt worktrees under one child.
func ExecutionScratchRoot(projectRoot string) string {
	root := filepath.Dir(filepath.Clean(ExecutionTempRoot(projectRoot)))
	if root == "" || root == "." {
		return os.TempDir()
	}
	return root
}

// MkdirExecutionScratch creates a DDx-owned scratch directory under the
// configured execution scratch root.
func MkdirExecutionScratch(projectRoot, pattern string) (string, error) {
	root := ExecutionScratchRoot(projectRoot)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, pattern)
}

// CreateExecutionScratch creates a DDx-owned scratch file under the configured
// execution scratch root.
func CreateExecutionScratch(projectRoot, pattern string) (*os.File, error) {
	root := ExecutionScratchRoot(projectRoot)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return os.CreateTemp(root, pattern)
}

func executionWorktreeRootFromFile(path, relativeBase string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	cfg, err := LoadFromFile(path)
	if err != nil || cfg == nil || cfg.Executions == nil {
		return ""
	}
	return normalizeExecutionWorktreeRoot(cfg.Executions.TempWorktreeRoot, relativeBase)
}

func normalizeExecutionWorktreeRoot(path, relativeBase string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = home
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(relativeBase) != "" {
		path = filepath.Join(relativeBase, path)
	}
	return filepath.Clean(path)
}
