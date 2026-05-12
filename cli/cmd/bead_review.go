package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadReviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <id>",
		Short: "Generate a review prompt for a bead's implementation",
		Long: "Generate a review-ready prompt for a bead implementation.\n\n" +
			"The prompt includes:\n" +
			"  - Bead title, description, and acceptance criteria\n" +
			"  - Full content of governing documents (spec-id refs from the bead)\n" +
			"  - Git diff of the reviewed commit (git show)\n" +
			"  - Review instructions with the expected APPROVE/REQUEST_CHANGES/BLOCK output contract\n\n" +
			"Use --prose to add an advisory prose-quality section derived from the bead\n" +
			"text, using the same deterministic prose checker as `ddx doc prose`.\n\n" +
			"By default the commit is taken from the bead's closing_commit_sha field.\n" +
			"Use --from-rev to override.\n\n" +
			"Pipe the output into `ddx run`:\n\n" +
			"  ddx bead review <id> | ddx run --prompt @-\n" +
			"  ddx bead review <id> --output /tmp/review.md && ddx run --prompt /tmp/review.md",
		Args: cobra.ExactArgs(1),
		RunE: f.runBeadReview,
	}
	cmd.Flags().String("from-rev", "", "Commit SHA to review (default: closing_commit_sha from bead)")
	cmd.Flags().Int("iter", 1, "Review iteration number (shown in prompt header and grade table)")
	cmd.Flags().String("output", "", "Write prompt to file instead of stdout")
	cmd.Flags().Bool("prose", false, "Include advisory prose findings derived from the bead text")
	return cmd
}

func (f *CommandFactory) runBeadReview(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	fromRev, _ := cmd.Flags().GetString("from-rev")
	iter, _ := cmd.Flags().GetInt("iter")
	outputFile, _ := cmd.Flags().GetString("output")
	includeProse, _ := cmd.Flags().GetBool("prose")

	s := f.beadStore()
	b, err := s.Get(beadID)
	if err != nil {
		return err
	}

	// Resolve the commit SHA to review.
	// Priority: --from-rev flag > closing_commit_sha on the bead.
	rev := strings.TrimSpace(fromRev)
	if rev == "" {
		if sha, ok := b.Extra["closing_commit_sha"].(string); ok {
			rev = strings.TrimSpace(sha)
		}
	}
	if rev == "" {
		return fmt.Errorf("no commit to review: use --from-rev <sha>, or close the bead with a commit SHA first (ddx bead close --commit <sha>)")
	}

	// Project root is used for git operations and governing doc file reads.
	projectRoot := gitpkg.FindProjectRoot(f.WorkingDir)
	if projectRoot == "" {
		projectRoot = f.WorkingDir
	}

	// Fetch the git diff for the commit.
	diff, err := beadReviewGitShow(projectRoot, rev)
	if err != nil {
		return fmt.Errorf("git show %s: %w", rev, err)
	}

	// Resolve governing document references from the bead's spec-id field.
	refs := agent.ResolveGoverningRefs(projectRoot, b)

	// Delegate prompt assembly to the single source of truth in the agent
	// package so the CLI handler and the post-merge reviewer cannot drift.
	var prompt string
	if includeProse {
		prompt = agent.BuildReviewPromptWithProse(b, iter, rev, diff, projectRoot, refs)
	} else {
		prompt = agent.BuildReviewPrompt(b, iter, rev, diff, projectRoot, refs)
	}

	if outputFile != "" {
		return os.WriteFile(outputFile, []byte(prompt), 0o644)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), prompt)
	return err
}

// beadReviewGitShow runs `git show <rev>` with pathspec exclusions for
// execution-evidence noise so the review prompt's <diff> section stays
// bounded. See agent.EvidenceReviewExcludePathspecs and ddx-39e27896.
func beadReviewGitShow(projectRoot, rev string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append([]string{"show", rev, "--", "."}, agent.EvidenceReviewExcludePathspecs()...)
	out, err := gitpkg.Command(ctx, projectRoot, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
