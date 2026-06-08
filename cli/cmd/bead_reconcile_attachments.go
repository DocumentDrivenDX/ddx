package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/gitlock"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadReconcileAttachmentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile-attachments",
		Short: "Commit dirty attachment audit files",
		Long: `Stage every dirty attachment artifact under .ddx/attachments/ and
commit them as a single bounded reconciliation checkpoint.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}
			gitDir, pathspecs := f.beadStateGitScope(".ddx/attachments")
			if gitDir == "" || len(pathspecs) == 0 {
				return fmt.Errorf("bead: resolve attachment git scope")
			}
			attachmentsPathspec := pathspecs[0]

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if out, err := internalgit.Command(ctx, gitDir, "rev-parse", "--is-inside-work-tree").Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
				return fmt.Errorf("bead: reconcile-attachments requires a git worktree")
			}

			statusArgs := []string{"status", "--short", "--untracked-files=all", "--", attachmentsPathspec}
			statusOut, err := internalgit.Command(ctx, gitDir, statusArgs...).Output()
			if err != nil {
				return fmt.Errorf("bead: attachment status: %w", err)
			}

			untrackedDirs, modified := summarizeAttachmentStatus(string(statusOut), attachmentsPathspec)
			if untrackedDirs == 0 && modified == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no attachment changes to reconcile")
				return nil
			}

			addOut, err := gitlock.RunGitWithIndexLockRecovery(ctx, gitDir, "add", "-A", "--", attachmentsPathspec)
			if err != nil {
				return fmt.Errorf("bead: stage attachment changes: %s: %w", strings.TrimSpace(string(addOut)), err)
			}

			diffOut, err := internalgit.Command(ctx, gitDir, "diff", "--cached", "--name-only", "--", attachmentsPathspec).Output()
			if err != nil {
				return fmt.Errorf("bead: inspect staged attachment changes: %w", err)
			}
			if strings.TrimSpace(string(diffOut)) == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "no attachment changes to reconcile")
				return nil
			}

			message := fmt.Sprintf(
				"chore(beads): reconcile missing attachment commits (%d dirs, %d modified)",
				untrackedDirs,
				modified,
			)
			commitArgs := []string{"commit", "--no-verify", "--only", "-m", message, "--", attachmentsPathspec}
			commitArgs = beadStateCommitArgs(workspaceRoot, gitDir, commitArgs...)
			commitOut, err := gitlock.RunGitWithIndexLockRecovery(ctx, gitDir, commitArgs...)
			if err != nil {
				return fmt.Errorf("bead: commit attachment reconciliation: %s: %w", strings.TrimSpace(string(commitOut)), err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), message)
			return nil
		},
	}
	return cmd
}

func (f *CommandFactory) beadStateGitScope(projectPathspecs ...string) (string, []string) {
	workspaceRoot := f.beadWorkspaceRoot()
	if workspaceRoot == "" {
		workspaceRoot = f.WorkingDir
	}
	if workspaceRoot == "" {
		return "", append([]string(nil), projectPathspecs...)
	}

	stateRoot := resolveBeadStoreRoot(workspaceRoot)
	if filepath.Clean(stateRoot) == filepath.Clean(ddxroot.InTree(workspaceRoot)) {
		return workspaceRoot, append([]string(nil), projectPathspecs...)
	}

	pathspecs := make([]string, 0, len(projectPathspecs))
	for _, pathspec := range projectPathspecs {
		cleaned := filepath.ToSlash(strings.TrimSpace(pathspec))
		cleaned = strings.TrimPrefix(cleaned, "./")
		cleaned = strings.TrimPrefix(cleaned, ".ddx/")
		pathspecs = append(pathspecs, cleaned)
	}
	return stateRoot, pathspecs
}

func beadStateCommitArgs(workspaceRoot, gitDir string, args ...string) []string {
	if workspaceRoot != "" && filepath.Clean(gitDir) == filepath.Clean(workspaceRoot) {
		return append([]string(nil), args...)
	}
	prefix := []string{
		"-c", "user.name=DDx State Root",
		"-c", "user.email=ddx-state-root@localhost",
		"-c", "commit.gpgsign=false",
	}
	return append(prefix, args...)
}

func summarizeAttachmentStatus(statusOutput, attachmentPathspec string) (int, int) {
	attachmentRoot := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(attachmentPathspec)), "/")
	untracked := map[string]struct{}{}
	modified := 0

	for _, line := range strings.Split(statusOutput, "\n") {
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = strings.TrimSpace(path[arrow+4:])
		}
		path = filepath.ToSlash(path)
		if path == "" {
			continue
		}
		if status == "??" {
			if dir := attachmentBeadDir(path, attachmentRoot); dir != "" {
				untracked[dir] = struct{}{}
			}
			continue
		}
		modified++
	}
	return len(untracked), modified
}

func attachmentBeadDir(path, attachmentRoot string) string {
	if attachmentRoot == "" {
		return ""
	}
	root := strings.TrimSuffix(filepath.ToSlash(attachmentRoot), "/")
	if !strings.HasPrefix(path, root+"/") {
		return ""
	}
	rel := strings.TrimPrefix(path, root+"/")
	beadID, _, ok := strings.Cut(rel, "/")
	if !ok || beadID == "" {
		return ""
	}
	return root + "/" + beadID
}
