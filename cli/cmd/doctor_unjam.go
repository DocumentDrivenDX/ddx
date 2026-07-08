package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

type doctorUnjamSummary struct {
	ProjectRoot string                  `json:"project_root"`
	Checkpoint  []doctorUnjamCheckpoint `json:"checkpoint,omitempty"`
	Stashes     []doctorUnjamStash      `json:"stashes,omitempty"`
	Reported    []doctorUnjamDirtyPath  `json:"reported,omitempty"`
	Remaining   []doctorUnjamDirtyPath  `json:"remaining,omitempty"`
	Clean       bool                    `json:"clean"`
	ScannedAt   time.Time               `json:"scanned_at"`
}

type doctorUnjamCheckpoint struct {
	Commit string   `json:"commit"`
	Paths  []string `json:"paths"`
}

type doctorUnjamStash struct {
	Ref     string   `json:"ref"`
	Message string   `json:"message"`
	Paths   []string `json:"paths"`
	TopRef  string   `json:"top_ref,omitempty"`
}

type doctorUnjamDirtyPath struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Ref    string `json:"ref,omitempty"`
}

type doctorUnjamStatusEntry struct {
	Path   string
	Status string
}

func (f *CommandFactory) runDoctorUnjam(cmd *cobra.Command) error {
	projectRoot := resolveProjectRoot("", f.WorkingDir)
	if projectRoot == "" {
		projectRoot = f.WorkingDir
	}

	entries, err := doctorUnjamDirtyEntries(projectRoot)
	if err != nil {
		return err
	}

	summary := doctorUnjamSummary{
		ProjectRoot: projectRoot,
		ScannedAt:   time.Now().UTC(),
	}
	if len(entries) == 0 {
		summary.Clean = true
		return json.NewEncoder(cmd.OutOrStdout()).Encode(summary)
	}

	checkpointPaths := doctorUnjamCheckpointPaths(entries)
	if len(checkpointPaths) > 0 {
		commit, err := doctorUnjamCreateCheckpoint(projectRoot, checkpointPaths)
		if err != nil {
			return err
		}
		if commit != "" {
			summary.Checkpoint = append(summary.Checkpoint, doctorUnjamCheckpoint{
				Commit: commit,
				Paths:  append([]string(nil), checkpointPaths...),
			})
		}
	}

	entries, err = doctorUnjamDirtyEntries(projectRoot)
	if err != nil {
		return err
	}

	stashGroups, reported := doctorUnjamClassifyPreservePaths(projectRoot, entries)
	for _, group := range stashGroups {
		stashRef, err := doctorUnjamStashPaths(projectRoot, group.ref, group.paths)
		if err != nil {
			return err
		}
		summary.Stashes = append(summary.Stashes, doctorUnjamStash{
			Ref:     group.ref,
			Message: "ddx doctor --unjam preserve " + group.ref,
			Paths:   append([]string(nil), group.paths...),
			TopRef:  stashRef,
		})
	}

	entries, err = doctorUnjamDirtyEntries(projectRoot)
	if err != nil {
		return err
	}
	remaining := entries

	summary.Reported = append(summary.Reported, reported...)
	summary.Remaining = append(summary.Remaining, doctorUnjamToReported(remaining)...)
	summary.Clean = len(summary.Remaining) == 0

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func doctorUnjamCheckpointPaths(entries []doctorUnjamStatusEntry) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, 2)
	for _, entry := range entries {
		if !doctorUnjamIsCheckpointPath(entry.Path) {
			continue
		}
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		seen[entry.Path] = struct{}{}
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths
}

func doctorUnjamIsCheckpointPath(path string) bool {
	return path == ".ddx/executions" ||
		strings.HasPrefix(path, ".ddx/executions/") ||
		path == ".ddx/metrics" ||
		strings.HasPrefix(path, ".ddx/metrics/")
}

func doctorUnjamCreateCheckpoint(projectRoot string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}

	addArgs := append([]string{"add", "-A", "--"}, paths...)
	if out, err := gitpkg.Command(context.Background(), projectRoot, addArgs...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("doctor --unjam: stage checkpoint paths: %s: %w", strings.TrimSpace(string(out)), err)
	}

	msg := "chore: doctor --unjam checkpoint [ddx-b23c2a68]"
	commitArgs := append([]string{"commit", "--no-verify", "--only", "-m", msg, "--"}, paths...)
	if out, err := gitpkg.Command(context.Background(), projectRoot, commitArgs...).CombinedOutput(); err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return "", nil
		}
		return "", fmt.Errorf("doctor --unjam: commit checkpoint: %s: %w", strings.TrimSpace(string(out)), err)
	}

	shaOut, err := gitpkg.Command(context.Background(), projectRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("doctor --unjam: resolve checkpoint commit: %w", err)
	}
	return strings.TrimSpace(string(shaOut)), nil
}

type doctorUnjamRefGroup struct {
	ref   string
	paths []string
}

func doctorUnjamClassifyPreservePaths(projectRoot string, entries []doctorUnjamStatusEntry) ([]doctorUnjamRefGroup, []doctorUnjamDirtyPath) {
	if len(entries) == 0 {
		return nil, nil
	}

	refPaths := make(map[string][]string)
	var reported []doctorUnjamDirtyPath
	for _, entry := range entries {
		if doctorUnjamIsCheckpointPath(entry.Path) {
			continue
		}
		ref, ok := doctorUnjamMatchingPreserveRef(projectRoot, entry.Path)
		if !ok {
			reported = append(reported, doctorUnjamDirtyPath{Path: entry.Path, Status: entry.Status})
			continue
		}
		refPaths[ref] = append(refPaths[ref], entry.Path)
	}

	refs := make([]string, 0, len(refPaths))
	for ref := range refPaths {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	groups := make([]doctorUnjamRefGroup, 0, len(refs))
	for _, ref := range refs {
		paths := refPaths[ref]
		sort.Strings(paths)
		groups = append(groups, doctorUnjamRefGroup{ref: ref, paths: paths})
	}
	sort.SliceStable(reported, func(i, j int) bool {
		if reported[i].Path != reported[j].Path {
			return reported[i].Path < reported[j].Path
		}
		return reported[i].Status < reported[j].Status
	})
	return groups, reported
}

func doctorUnjamMatchingPreserveRef(projectRoot, relPath string) (string, bool) {
	absPath := filepath.Join(projectRoot, filepath.FromSlash(relPath))
	fileBlob, err := gitpkg.Command(context.Background(), projectRoot, "hash-object", absPath).Output()
	if err != nil {
		return "", false
	}
	wantBlob := strings.TrimSpace(string(fileBlob))
	if wantBlob == "" {
		return "", false
	}

	refsOut, err := gitpkg.Command(context.Background(), projectRoot, "for-each-ref", "--format=%(refname)", "refs/ddx/iterations").Output()
	if err != nil {
		return "", false
	}
	refs := strings.Fields(string(refsOut))
	sort.Strings(refs)

	for _, ref := range refs {
		blobOut, err := gitpkg.Command(context.Background(), projectRoot, "rev-parse", "-q", "--verify", ref+":"+relPath).Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(blobOut)) == wantBlob {
			return ref, true
		}
	}
	return "", false
}

func doctorUnjamStashPaths(projectRoot, ref string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}

	msg := "ddx doctor --unjam preserve " + ref
	args := append([]string{"stash", "push", "--include-untracked", "--message", msg, "--"}, paths...)
	if out, err := gitpkg.Command(context.Background(), projectRoot, args...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("doctor --unjam: stash preserve paths for %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	stashRefOut, err := gitpkg.Command(context.Background(), projectRoot, "rev-parse", "-q", "--verify", "refs/stash").Output()
	if err != nil {
		return "", fmt.Errorf("doctor --unjam: resolve stash ref for %s: %w", ref, err)
	}
	return strings.TrimSpace(string(stashRefOut)), nil
}

func doctorUnjamDirtyEntries(projectRoot string) ([]doctorUnjamStatusEntry, error) {
	out, err := gitpkg.Command(context.Background(), projectRoot,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching", "--", ".").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("doctor --unjam: inspect dirty paths: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	entries := make([]doctorUnjamStatusEntry, 0, 8)
	for len(out) > 0 {
		recordEnd := bytes.IndexByte(out, 0)
		if recordEnd == -1 {
			recordEnd = len(out)
		}
		record := out[:recordEnd]
		if recordEnd < len(out) {
			out = out[recordEnd+1:]
		} else {
			out = nil
		}
		if len(record) < 3 {
			continue
		}
		status := string(record[:2])
		path := filepath.ToSlash(string(record[3:]))
		if path == "" {
			continue
		}
		if status[0] == 'R' || status[0] == 'C' {
			recordEnd = bytes.IndexByte(out, 0)
			if recordEnd == -1 {
				recordEnd = len(out)
			}
			record = out[:recordEnd]
			if recordEnd < len(out) {
				out = out[recordEnd+1:]
			} else {
				out = nil
			}
			if len(record) > 0 {
				path = filepath.ToSlash(string(record))
			}
		}
		entries = append(entries, doctorUnjamStatusEntry{Path: path, Status: status})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Status < entries[j].Status
	})
	return entries, nil
}

func doctorUnjamToReported(entries []doctorUnjamStatusEntry) []doctorUnjamDirtyPath {
	reported := make([]doctorUnjamDirtyPath, 0, len(entries))
	for _, entry := range entries {
		reported = append(reported, doctorUnjamDirtyPath{
			Path:   entry.Path,
			Status: entry.Status,
		})
	}
	sort.SliceStable(reported, func(i, j int) bool {
		if reported[i].Path != reported[j].Path {
			return reported[i].Path < reported[j].Path
		}
		return reported[i].Status < reported[j].Status
	})
	return reported
}
