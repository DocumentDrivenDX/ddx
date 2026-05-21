package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var versionCommitPattern = regexp.MustCompile(`(?m)^Commit:\s*(\S+)\s*$`)

func (f *CommandFactory) buildWorkBinaryRefreshCheck(cmd *cobra.Command, projectRoot, targetBeadID string) func(context.Context) (bool, error) {
	if targetBeadID != "" {
		return nil
	}
	if !workBinaryRefreshEnabled(os.Args) {
		return nil
	}
	currentCommit := normalizeVersionCommit(f.Commit)
	if currentCommit == "" {
		return nil
	}
	exe := resolveReplacementDDXPath()
	baseline, haveBaseline := snapshotBinary(exe)
	return func(ctx context.Context) (bool, error) {
		// Only hand off when the replacement binary file has actually been
		// replaced on disk since this worker started (an intentional
		// reinstall). Bead-only auto-commits (chore(beads): ... doc-stamp)
		// advance the ddx repo HEAD but never rewrite the binary, so they must
		// not terminate the worker (ddx-65d3ba51).
		if !haveBaseline {
			return false, nil
		}
		current, ok := snapshotBinary(exe)
		if !ok || current.equal(baseline) {
			return false, nil
		}
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
		args := append([]string(nil), os.Args[1:]...)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ddx work: detected installed ddx binary changed (current commit %s, installed commit %s); starting replacement worker and exiting\n", currentCommit, installedCommit)
		if err := startReplacementDDX(exe, args, wd); err != nil {
			return false, err
		}
		return true, nil
	}
}

func workBinaryRefreshEnabled(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return filepath.Base(args[0]) == "ddx"
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

// binarySnapshot captures the on-disk identity of an executable so a worker can
// tell whether the file was replaced (reinstalled) versus unchanged. It
// deliberately does not look at git/version metadata — repo HEAD advances from
// bead commits must not read as a binary change.
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

func startReplacementDDX(exe string, args []string, dir string) error {
	replacement := exec.Command(exe, args...)
	replacement.Dir = dir
	replacement.Env = os.Environ()
	replacement.Stdin = os.Stdin
	replacement.Stdout = os.Stdout
	replacement.Stderr = os.Stderr
	return replacement.Start()
}
