package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var versionCommitPattern = regexp.MustCompile(`(?m)^Commit:\s*(\S+)\s*$`)

func (f *CommandFactory) buildWorkBinaryRefreshCheck(cmd *cobra.Command, projectRoot, targetBeadID string, enabled bool) func(context.Context) (bool, error) {
	if targetBeadID != "" {
		return nil
	}
	if !enabled {
		return nil
	}
	currentCommit := normalizeVersionCommit(f.Commit)
	if currentCommit == "" {
		return nil
	}
	exe := f.resolveWorkBinaryPath()
	return func(ctx context.Context) (bool, error) {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		installedCommit, err := installedDDXCommit(checkCtx, exe)
		if err != nil {
			return false, err
		}
		if !shouldRefreshDDXBinary(currentCommit, installedCommit) {
			return false, nil
		}
		wd, wdErr := os.Getwd()
		if wdErr != nil || wd == "" {
			wd = projectRoot
		}
		argv := append([]string(nil), os.Args...)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ddx work: installed ddx commit changed from %s to %s; self-refreshing before the next bead\n", currentCommit, installedCommit)
		if err := f.reexecWorkBinary(exe, argv, os.Environ(), wd); err != nil {
			return false, err
		}
		return true, nil
	}
}

func installedDDXCommit(ctx context.Context, exe string) (string, error) {
	out, err := exec.CommandContext(ctx, exe, "version", "--no-check").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("probe installed ddx version: %w", err)
	}
	return parseDDXVersionCommit(string(out)), nil
}

func parseDDXVersionCommit(output string) string {
	matches := versionCommitPattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return ""
	}
	return normalizeVersionCommit(matches[1])
}

func normalizeVersionCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	switch strings.ToLower(commit) {
	case "", "unknown", "dev", "none":
		return ""
	default:
		return commit
	}
}

// binarySnapshot captures the on-disk identity of an executable so tests can
// verify reinstall detection behavior independently from the refresh hook.
type binarySnapshot struct {
	size    int64
	modTime time.Time
}

func snapshotBinary(path string) (binarySnapshot, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return binarySnapshot{}, false
	}
	return binarySnapshot{size: info.Size(), modTime: info.ModTime()}, true
}

func (b binarySnapshot) equal(o binarySnapshot) bool {
	return b.size == o.size && b.modTime.Equal(o.modTime)
}

func shouldRefreshDDXBinary(currentCommit, installedCommit string) bool {
	currentCommit = normalizeVersionCommit(currentCommit)
	installedCommit = normalizeVersionCommit(installedCommit)
	if currentCommit == "" || installedCommit == "" {
		return false
	}
	if currentCommit == installedCommit {
		return false
	}
	if len(currentCommit) >= 7 && len(installedCommit) >= 7 {
		if strings.HasPrefix(currentCommit, installedCommit) || strings.HasPrefix(installedCommit, currentCommit) {
			return false
		}
	}
	return true
}

func resolveReplacementDDXPath() string {
	if len(os.Args) > 0 && os.Args[0] != "" {
		if path, err := exec.LookPath(os.Args[0]); err == nil {
			return path
		}
	}
	if path, err := os.Executable(); err == nil {
		return path
	}
	if len(os.Args) > 0 {
		return os.Args[0]
	}
	return "ddx"
}

func workBinaryRefreshEnabled(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return filepath.Base(args[0]) == "ddx"
}

func (f *CommandFactory) resolveWorkBinaryPath() string {
	if f.workBinaryPathOverride != nil {
		return f.workBinaryPathOverride()
	}
	return resolveReplacementDDXPath()
}

func (f *CommandFactory) reexecWorkBinary(exe string, argv []string, env []string, dir string) error {
	if f.workBinaryReexecOverride != nil {
		return f.workBinaryReexecOverride(exe, argv, env, dir)
	}
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return err
		}
	}
	return syscall.Exec(exe, argv, env)
}
