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
	defer func() {
		// Deregister the seeded worktree (no-op/ignored if the fallback empty
		// repo was used) before removing the directory, so it does not linger
		// in the project's worktree registry.
		_ = internalgit.Command(context.Background(), projectRoot, "worktree", "remove", "--force", scratchDir).Run()
		_ = os.RemoveAll(scratchDir)
	}()

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
	dir, err := config.MkdirExecutionScratch(projectRoot, lifecycleScratchDirPrefix)
	if err != nil {
		return "", err
	}
	// Seed the scratch workdir from the project's HEAD so the readiness-check
	// classifier sees the project's real source files rather than an empty repo
	// — otherwise file-presence checks block every bead (ddx-efadca32). A
	// detached worktree keeps the classifier isolated from the master worktree
	// (it cannot mutate tracked files there). git worktree add must create the
	// directory itself, so clear the placeholder MkdirExecutionScratch made.
	if rmErr := os.RemoveAll(dir); rmErr != nil {
		return "", fmt.Errorf("reset lifecycle scratch dir: %w", rmErr)
	}
	if out, addErr := internalgit.Command(context.Background(), projectRoot, "worktree", "add", "--detach", dir, "HEAD").CombinedOutput(); addErr != nil {
		// HEAD may be unborn (a project with no commits yet); readiness has
		// nothing to check there, so fall back to an empty scratch repo.
		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			return "", fmt.Errorf("recreate lifecycle scratch dir after worktree add failed (%s): %w", strings.TrimSpace(string(out)), mkErr)
		}
		if out2, initErr := internalgit.Command(context.Background(), dir, "init", "-q").CombinedOutput(); initErr != nil {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("initialize lifecycle scratch git repo: %s: %w", strings.TrimSpace(string(out2)), initErr)
		}
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
