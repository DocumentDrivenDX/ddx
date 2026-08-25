package agent

// unjam_classify.go wires the ddx doctor --unjam dirty-path classification
// (see cmd/doctor_unjam.go) into the pre-claim and land gates: before either
// gate parks on uncommitted/staged changes, it classifies each dirty path as
// (a) DDx-owned tracked state, (b) content already backed up under a
// preserved iteration ref, or (c) unknown/foreign, and remediates every safe
// (a)/(b) path in place so only genuinely foreign dirt ever blocks a claim
// or a land.

import (
	"bufio"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

const (
	unjamGateDDXOwnedCheckpointMessage     = "chore: checkpoint ddx-owned state (unjam gate)"
	unjamGatePreserveRefStashMessagePrefix = "ddx unjam gate preserve "
)

// remediateUnjamSafeDirt classifies dirtyPaths the same way `ddx doctor
// --unjam` does: DDx-owned tracked state (.ddx/... excluding local execution
// evidence, which is never committed per the Execution Evidence Convention)
// is checkpoint-committed in place, and any remainder that matches the tree
// of a preserved iteration ref (refs/ddx/iterations/...) is stashed under
// that ref. It returns the paths that are still dirty after remediation —
// callers must treat a non-empty result as foreign/unknown dirt and park.
func remediateUnjamSafeDirt(ctx context.Context, projectRoot string, dirtyPaths []string) ([]string, error) {
	if projectRoot == "" || len(dirtyPaths) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := internalgit.Command(ctx, projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return append([]string(nil), dirtyPaths...), nil
	}

	normalized := normalizeUnjamGatePaths(dirtyPaths)

	var ddxOwned []string
	for _, path := range normalized {
		if isUnjamGateDDXOwnedPath(path) {
			ddxOwned = append(ddxOwned, path)
		}
	}
	if len(ddxOwned) > 0 {
		if err := checkpointUnjamGateDDXOwnedPaths(ctx, projectRoot, ddxOwned); err != nil {
			return nil, err
		}
	}

	remaining, err := unjamGateDirtyPathsForPaths(ctx, projectRoot, normalized)
	if err != nil {
		return nil, err
	}
	if len(remaining) == 0 {
		return nil, nil
	}

	if err := stashUnjamGatePreserveRefBackedPaths(ctx, projectRoot, remaining); err != nil {
		return nil, err
	}

	return unjamGateDirtyPathsForPaths(ctx, projectRoot, remaining)
}

func normalizeUnjamGatePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = normalizeUnjamGatePath(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func normalizeUnjamGatePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

// isUnjamGateDDXOwnedPath classifies path as DDx-owned tracked state — safe
// to checkpoint-commit automatically. Execution evidence under
// .ddx/executions/ is explicitly excluded: it is per-machine working state
// that must never enter git history (Execution Evidence Convention).
func isUnjamGateDDXOwnedPath(path string) bool {
	path = normalizeUnjamGatePath(path)
	if path == "" {
		return false
	}
	if isExecutionEvidencePath(path) {
		return false
	}
	if path == ".ddx.yml" || path == ".ddx.yaml" {
		return true
	}
	return path == ".ddx" || strings.HasPrefix(path, ".ddx/")
}

func checkpointUnjamGateDDXOwnedPaths(ctx context.Context, projectRoot string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	addArgs := append([]string{"add", "-f", "-A", "--"}, paths...)
	if out, err := internalgit.Command(ctx, projectRoot, addArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("checkpointing ddx-owned dirt: %s: %w", strings.TrimSpace(string(out)), err)
	}

	cachedArgs := append([]string{"diff", "--cached", "--name-only", "--"}, paths...)
	cachedOut, err := internalgit.Command(ctx, projectRoot, cachedArgs...).Output()
	if err != nil {
		return fmt.Errorf("checking staged ddx-owned dirt: %w", err)
	}
	committed := splitUnjamGateLines(string(cachedOut))
	if len(committed) == 0 {
		return nil
	}

	commitArgs := append([]string{"commit", "--no-verify", "--only", "-m", unjamGateDDXOwnedCheckpointMessage, "--"}, committed...)
	if out, err := internalgit.Command(ctx, projectRoot, commitArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("committing ddx-owned dirt: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func unjamGateDirtyPathsForPaths(ctx context.Context, projectRoot string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	args := append([]string{"status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--"}, paths...)
	out, err := internalgit.Command(ctx, projectRoot, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("checking remaining dirty paths: %w", err)
	}
	return parseUnjamGateStatusPaths(string(out)), nil
}

func parseUnjamGateStatusPaths(output string) []string {
	var paths []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		path = normalizeUnjamGatePath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func splitUnjamGateLines(output string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// stashUnjamGatePreserveRefBackedPaths stashes any of dirtyPaths whose
// content overlaps a preserved iteration ref (refs/ddx/iterations/...) under
// that ref, recovering a leaked checkout fragment without discarding it.
// Paths with no matching preserve ref are left untouched.
func stashUnjamGatePreserveRefBackedPaths(ctx context.Context, projectRoot string, dirtyPaths []string) error {
	if len(dirtyPaths) == 0 {
		return nil
	}
	preserveRefs, err := listUnjamGatePreserveIterationRefs(ctx, projectRoot)
	if err != nil || len(preserveRefs) == 0 {
		return err
	}

	remaining := append([]string(nil), dirtyPaths...)
	for _, ref := range preserveRefs {
		if len(remaining) == 0 {
			break
		}
		treePaths, err := listUnjamGatePreserveRefTreePaths(ctx, projectRoot, ref)
		if err != nil {
			return err
		}
		matched := matchUnjamGatePreserveRefDirtyPaths(remaining, treePaths)
		if len(matched) == 0 {
			continue
		}
		if err := stashUnjamGateDirtyPaths(ctx, projectRoot, ref, matched); err != nil {
			return err
		}
		remaining = removeUnjamGatePaths(remaining, matched)
	}
	return nil
}

func listUnjamGatePreserveIterationRefs(ctx context.Context, projectRoot string) ([]string, error) {
	out, err := internalgit.Command(ctx, projectRoot, "for-each-ref", "--sort=-refname", "--format=%(refname)", "refs/ddx/iterations").Output()
	if err != nil {
		return nil, fmt.Errorf("listing preserve iteration refs: %w", err)
	}
	refs := splitUnjamGateLines(string(out))
	filtered := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.Contains(filepath.Base(ref), "-") {
			filtered = append(filtered, ref)
		}
	}
	return filtered, nil
}

func listUnjamGatePreserveRefTreePaths(ctx context.Context, projectRoot, preserveRef string) ([]string, error) {
	out, err := internalgit.Command(ctx, projectRoot, "ls-tree", "--full-tree", "-r", "--name-only", preserveRef).Output()
	if err != nil {
		return nil, fmt.Errorf("listing paths for preserve ref %s: %w", preserveRef, err)
	}
	return splitUnjamGateLines(string(out)), nil
}

func matchUnjamGatePreserveRefDirtyPaths(dirtyPaths, treePaths []string) []string {
	if len(dirtyPaths) == 0 || len(treePaths) == 0 {
		return nil
	}
	matched := make([]string, 0, len(dirtyPaths))
	seen := make(map[string]bool, len(dirtyPaths))
	for _, dirtyPath := range dirtyPaths {
		dirtyPath = normalizeUnjamGatePath(dirtyPath)
		if dirtyPath == "" || seen[dirtyPath] || isExecutionEvidencePath(dirtyPath) {
			continue
		}
		for _, treePath := range treePaths {
			if unjamGateDirtyPathMatchesTreePath(dirtyPath, treePath) {
				seen[dirtyPath] = true
				matched = append(matched, dirtyPath)
				break
			}
		}
	}
	return matched
}

func unjamGateDirtyPathMatchesTreePath(dirtyPath, treePath string) bool {
	dirtyPath = normalizeUnjamGatePath(dirtyPath)
	treePath = normalizeUnjamGatePath(treePath)
	if dirtyPath == "" || treePath == "" || dirtyPath == "." || treePath == "." {
		return false
	}
	if dirtyPath == treePath {
		return true
	}
	if strings.HasPrefix(treePath, dirtyPath+"/") {
		return true
	}
	return strings.HasPrefix(dirtyPath, treePath+"/")
}

func removeUnjamGatePaths(paths, remove []string) []string {
	if len(paths) == 0 || len(remove) == 0 {
		return append([]string(nil), paths...)
	}
	removeSet := make(map[string]bool, len(remove))
	for _, path := range remove {
		removeSet[normalizeUnjamGatePath(path)] = true
	}
	remaining := make([]string, 0, len(paths))
	for _, path := range paths {
		if !removeSet[normalizeUnjamGatePath(path)] {
			remaining = append(remaining, path)
		}
	}
	return remaining
}

func stashUnjamGateDirtyPaths(ctx context.Context, projectRoot, preserveRef string, dirtyPaths []string) error {
	mutablePaths := make([]string, 0, len(dirtyPaths))
	for _, path := range dirtyPaths {
		path = normalizeUnjamGatePath(path)
		if path == "" || isExecutionEvidencePath(path) {
			continue
		}
		mutablePaths = append(mutablePaths, path)
	}
	if len(mutablePaths) == 0 {
		return nil
	}

	message := unjamGatePreserveRefStashMessagePrefix + preserveRef
	args := []string{"stash", "push", "--all", "--message", message, "--"}
	args = append(args, mutablePaths...)
	if out, err := internalgit.Command(ctx, projectRoot, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("stashing preserve-derived dirt for %s: %s: %w", preserveRef, strings.TrimSpace(string(out)), err)
	}
	return nil
}
