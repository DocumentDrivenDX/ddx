package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadAutoCommitSerializesThroughMainGitLock(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	const concurrentUpdates = 4
	beadIDs := make([]string, 0, concurrentUpdates)
	for i := 0; i < concurrentUpdates; i++ {
		out, err := executeCommand(
			NewCommandFactory(env.Dir).NewRootCommand(),
			"bead", "create", fmt.Sprintf("Concurrent bead %d", i+1),
		)
		require.NoError(t, err)
		beadIDs = append(beadIDs, strings.TrimSpace(out))
	}

	// Prime each bead row to the current schema before the concurrent phase so
	// the regression isolates lock behaviour rather than first-write migrations.
	for i, beadID := range beadIDs {
		_, err := executeCommand(
			NewCommandFactory(env.Dir).NewRootCommand(),
			"bead", "update", beadID,
			"--notes", fmt.Sprintf("schema-prime-%d", i+1),
		)
		require.NoError(t, err)
	}

	baseSHA := runGitCmd(t, env.Dir, "rev-parse", "HEAD")

	prevHook := gitpkg.AutoCommitAfterAddHook
	gitpkg.AutoCommitAfterAddHook = func(gitpkg.AutoCommitHookContext) {
		time.Sleep(200 * time.Millisecond)
	}
	t.Cleanup(func() {
		gitpkg.AutoCommitAfterAddHook = prevHook
	})

	start := make(chan struct{})
	errs := make([]error, len(beadIDs))
	var wg sync.WaitGroup
	for i, beadID := range beadIDs {
		i := i
		beadID := beadID
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = executeCommand(
				NewCommandFactory(env.Dir).NewRootCommand(),
				"bead", "update", beadID,
				"--notes", fmt.Sprintf("main-git-lock-update-%d", i+1),
			)
		}()
	}

	close(start)
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "index.lock") {
			t.Fatalf("concurrent auto-commit %d hit index.lock contention: %v", i, err)
		}
		require.NoErrorf(t, err, "concurrent auto-commit %d failed unexpectedly", i)
	}

	commitSHAs := strings.Fields(runGitCmd(t, env.Dir, "rev-list", "--reverse", baseSHA+"..HEAD"))
	require.Len(t, commitSHAs, len(beadIDs), "each concurrent update should land its own serialized auto-commit")

	subjectPattern := regexp.MustCompile(`\bupdate ([^ ]+)\b`)
	for _, sha := range commitSHAs {
		subject := runGitCmd(t, env.Dir, "show", "-s", "--format=%s", sha)
		match := subjectPattern.FindStringSubmatch(subject)
		require.Lenf(t, match, 2, "commit subject should name exactly one bead update: %s", subject)

		patch := runGitCmd(t, env.Dir, "show", "--unified=0", "--format=", sha, "--", ".ddx/beads.jsonl")
		mutatedIDs := patchBeadIDs(patch, beadIDs)
		assert.Equalf(
			t,
			[]string{match[1]},
			mutatedIDs,
			"commit %s should only include tracker mutations for the bead named in subject %q",
			sha,
			subject,
		)
	}
}

func patchBeadIDs(patch string, beadIDs []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(beadIDs))
	for _, beadID := range beadIDs {
		if !strings.Contains(patch, beadID) {
			continue
		}
		if _, ok := seen[beadID]; ok {
			continue
		}
		seen[beadID] = struct{}{}
		out = append(out, beadID)
	}
	sort.Strings(out)
	return out
}
