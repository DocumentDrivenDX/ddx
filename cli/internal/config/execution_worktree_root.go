package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

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
//
// When a *config-file* root sits on a different device from projectRoot (for
// example a Mac virtiofs share while the project is on local Linux disk), the
// configured path is ignored and a local cache root is used instead. Cross-
// device git worktree add of multi-GB checkouts routinely stalls and leaves
// locked orphans that cleanup cannot reclaim while the add is still running.
// An explicit DDX_EXEC_WT_DIR environment override is always honored.
func ExecutionTempRoot(projectRoot string) string {
	envOverride := strings.TrimSpace(os.Getenv(ExecutionWorktreeRootEnv)) != ""
	if root := ExecutionWorktreeRoot(projectRoot); root != "" {
		if envOverride || projectRoot == "" || sameDevicePath(projectRoot, root) {
			return root
		}
		// Fall through to a same-device local default.
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		local := filepath.Join(cacheDir, "ddx", "exec-wt")
		if projectRoot == "" || sameDevicePath(projectRoot, cacheDir) || sameDevicePath(projectRoot, local) {
			return local
		}
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
//
// When the effective temp root is cross-device relative to projectRoot,
// ExecutionTempRoot already rewrites it to a local cache path; scratch then
// sits beside that local root (e.g. ~/.cache/ddx/scratch).
func ExecutionScratchRoot(projectRoot string) string {
	root := filepath.Dir(filepath.Clean(ExecutionTempRoot(projectRoot)))
	if root == "" || root == "." {
		return os.TempDir()
	}
	return root
}

// sameDevicePath reports whether a and b resolve to the same filesystem
// device. Missing paths walk up to an existing ancestor. When device IDs
// cannot be read, the paths are treated as same-device so callers keep the
// configured root rather than silently relocating.
func sameDevicePath(a, b string) bool {
	devA, okA := deviceID(a)
	devB, okB := deviceID(b)
	if !okA || !okB {
		return true
	}
	return devA == devB
}

func deviceID(path string) (uint64, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, false
	}
	cur := filepath.Clean(path)
	for {
		info, err := os.Stat(cur)
		if err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				return uint64(st.Dev), true
			}
			return 0, false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return 0, false
		}
		cur = parent
	}
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
