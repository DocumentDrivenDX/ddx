package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

const ExecutionWorktreeRootEnv = "DDX_EXEC_WT_DIR"

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
