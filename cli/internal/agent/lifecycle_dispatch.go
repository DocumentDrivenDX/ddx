package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
)

const (
	// Lifecycle classifiers run in an isolated scratch workdir and are guarded by
	// a project-root dirtiness check. Use Fizeau's supported safe permission band
	// so unpinned auto-routing can choose a route; the reviewer path keeps the
	// stricter internal readonly marker.
	PermissionsReadOnlyLifecycle = "safe"
	lifecycleScratchDirPrefix    = "ddx-lifecycle-"
)

type lifecycleProjectStatusSnapshot struct {
	available bool
	entries   map[string]string
}

func dispatchLifecycleRun(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	scratchDir, err := newLifecycleScratchDir(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("lifecycle dispatch: create scratch workdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratchDir) }()

	runtime.WorkDir = scratchDir
	runtime.PermissionsOverride = PermissionsReadOnlyLifecycle

	before, err := captureLifecycleProjectStatus(projectRoot)
	if err != nil {
		return nil, err
	}

	result, dispatchErr := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, runtime)

	if guardErr := guardLifecycleProjectStatus(projectRoot, before); guardErr != nil {
		return nil, guardErr
	}
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	return result, nil
}

func newLifecycleScratchDir(projectRoot string) (string, error) {
	base := config.ExecutionWorktreeRoot(projectRoot)
	if base == "" || (projectRoot != "" && isPathWithin(base, projectRoot)) {
		base = filepath.Join(os.TempDir(), ExecuteBeadTmpSubdir)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, lifecycleScratchDirPrefix)
	if err != nil {
		return "", err
	}
	if out, err := internalgit.Command(context.Background(), dir, "init", "-q").CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("initialize lifecycle scratch git repo: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return dir, nil
}

func captureLifecycleProjectStatus(projectRoot string) (lifecycleProjectStatusSnapshot, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return lifecycleProjectStatusSnapshot{}, nil
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return lifecycleProjectStatusSnapshot{}, nil
		}
		return lifecycleProjectStatusSnapshot{}, fmt.Errorf("lifecycle dispatch: stat project root .git: %w", err)
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return lifecycleProjectStatusSnapshot{}, fmt.Errorf("lifecycle dispatch: snapshot project root dirtiness: %w", err)
	}
	return lifecycleProjectStatusSnapshot{
		available: true,
		entries:   parseLifecycleProjectStatus(string(out)),
	}, nil
}

func parseLifecycleProjectStatus(raw string) map[string]string {
	entries := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 || strings.TrimSpace(line) == "" {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" {
			continue
		}
		entries[path] = status
	}
	return entries
}

func guardLifecycleProjectStatus(projectRoot string, before lifecycleProjectStatusSnapshot) error {
	if !before.available {
		return nil
	}
	after, err := captureLifecycleProjectStatus(projectRoot)
	if err != nil {
		return err
	}
	if lifecycleProjectStatusEqual(before.entries, after.entries) {
		return nil
	}

	newPaths, changedPaths := diffLifecycleProjectStatus(before.entries, after.entries)
	if len(newPaths) > 0 {
		if err := restoreLifecycleProjectMutations(projectRoot, after.entries, newPaths); err != nil {
			return err
		}
		restored, restoreErr := captureLifecycleProjectStatus(projectRoot)
		if restoreErr != nil {
			return restoreErr
		}
		after = restored
		newPaths, changedPaths = diffLifecycleProjectStatus(before.entries, after.entries)
	}
	if len(newPaths) == 0 && len(changedPaths) == 0 {
		return fmt.Errorf("lifecycle dispatch: project root mutation rejected")
	}

	var details []string
	if len(newPaths) > 0 {
		details = append(details, "new dirty paths: "+strings.Join(newPaths, ", "))
	}
	if len(changedPaths) > 0 {
		details = append(details, "changed existing dirty paths: "+strings.Join(changedPaths, ", "))
	}
	return fmt.Errorf("lifecycle dispatch: project root mutation rejected (%s)", strings.Join(details, "; "))
}

func lifecycleProjectStatusEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for path, status := range left {
		if right[path] != status {
			return false
		}
	}
	return true
}

func diffLifecycleProjectStatus(before, after map[string]string) (newPaths []string, changedPaths []string) {
	for path, afterStatus := range after {
		beforeStatus, ok := before[path]
		if !ok {
			newPaths = append(newPaths, path)
			continue
		}
		if beforeStatus != afterStatus {
			changedPaths = append(changedPaths, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changedPaths = append(changedPaths, path)
		}
	}
	sort.Strings(newPaths)
	sort.Strings(changedPaths)
	return newPaths, changedPaths
}

func restoreLifecycleProjectMutations(projectRoot string, after map[string]string, newPaths []string) error {
	var restorePaths []string
	var cleanPaths []string
	for _, path := range newPaths {
		if after[path] == "??" {
			cleanPaths = append(cleanPaths, path)
			continue
		}
		restorePaths = append(restorePaths, path)
	}

	if len(restorePaths) > 0 {
		args := []string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}
		args = append(args, restorePaths...)
		if out, err := internalgit.Command(context.Background(), projectRoot, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("lifecycle dispatch: restore project root mutation: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	if len(cleanPaths) > 0 {
		args := []string{"clean", "-fd", "--"}
		args = append(args, cleanPaths...)
		if out, err := internalgit.Command(context.Background(), projectRoot, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("lifecycle dispatch: clean project root mutation: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}
