package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTrackerLockRuntimePathsIgnoredWithoutGitignore proves the never-deleted
// main-git guard sidecar is excluded from synthesize-commit pathspecs and
// ignored by pre-dispatch and checkout-sync dirt matching even when .gitignore
// has no applicable rule. Ordinary untracked implementation files remain
// visible. All three surfaces must reference trackerStaleLockBreakGuardPath
// rather than duplicating the sidecar literal.
func TestTrackerLockRuntimePathsIgnoredWithoutGitignore(t *testing.T) {
	root, _ := newScriptHarnessRepo(t, 0)

	// Simulate a stale project whose versioned ignore file does not protect
	// main-git lock coordination paths — code-level matching must still hold.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), nil, 0o644))
	runGitInteg(t, root, "add", ".gitignore")
	runGitInteg(t, root, "commit", "-m", "remove tracker-lock ignore coverage")

	guardRel := filepath.ToSlash(trackerStaleLockBreakGuardPath(filepath.Join(".ddx", ".git-tracker.lock")))
	require.Equal(t, ".ddx/.git-tracker.lock.stale-break.lock", guardRel)
	const implRel = "cli/internal/agent/feature.go"

	// --- synthesizeCommitExcludePathspecs ---
	specs := synthesizeCommitExcludePathspecs(root)
	assert.Contains(t, specs, ":(exclude)"+guardRel,
		"sidecar must be excluded by synthesize pathspecs when not gitignored")
	for _, spec := range specs {
		assert.False(t, strings.Contains(spec, implRel),
			"ordinary implementation path must not appear in exclude pathspecs: %s", spec)
	}

	// Full synthesize: sidecar on disk must not land in the commit; impl must.
	guardAbs := filepath.Join(root, filepath.FromSlash(guardRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(guardAbs), 0o755))
	require.NoError(t, os.WriteFile(guardAbs, []byte("advisory\n"), 0o600))
	implAbs := filepath.Join(root, filepath.FromSlash(implRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(implAbs), 0o755))
	require.NoError(t, os.WriteFile(implAbs, []byte("package agent\n"), 0o644))

	committed, err := (&RealGitOps{}).SynthesizeCommit(root, "feat: real work without sidecar")
	require.NoError(t, err)
	require.True(t, committed)
	tree := runGitInteg(t, root, "ls-tree", "-r", "--name-only", "HEAD")
	assert.Contains(t, tree, implRel, "ordinary untracked implementation file must remain commit-visible")
	assert.NotContains(t, tree, guardRel, "guard sidecar must not be staged by SynthesizeCommit")

	// --- preDispatchCheckpointIgnoredPath ---
	assert.True(t, preDispatchCheckpointIgnoredPath(guardRel),
		"pre-dispatch must ignore the guard sidecar without .gitignore help")
	assert.False(t, preDispatchCheckpointIgnoredPath(implRel),
		"pre-dispatch must still surface ordinary implementation paths")

	// --- checkoutSyncDeferralIgnoredPath ---
	assert.True(t, checkoutSyncDeferralIgnoredPath(guardRel),
		"checkout-sync must ignore the guard sidecar without .gitignore help")
	assert.False(t, checkoutSyncDeferralIgnoredPath(implRel),
		"checkout-sync must still surface ordinary implementation paths")

	// --- structural: each surface references the shared helper, not a literal ---
	surfaces := []string{
		"gitops_real.go",
		"predispatch_dirty.go",
		"execute_bead_land.go",
	}
	const helperName = "trackerStaleLockBreakGuardPath"
	const forbiddenLiteral = `".ddx/.git-tracker.lock.stale-break.lock"`
	const forbiddenPathspec = `:(exclude).ddx/.git-tracker.lock.stale-break.lock`
	for _, name := range surfaces {
		src, readErr := os.ReadFile(name)
		require.NoError(t, readErr, name)
		body := string(src)
		assert.Contains(t, body, helperName,
			"%s must reference %s rather than a duplicated sidecar literal", name, helperName)
		assert.NotContains(t, body, forbiddenLiteral,
			"%s must not hardcode the full sidecar path as a string literal", name)
		assert.NotContains(t, body, forbiddenPathspec,
			"%s must not hardcode the sidecar exclude pathspec", name)
	}
}
