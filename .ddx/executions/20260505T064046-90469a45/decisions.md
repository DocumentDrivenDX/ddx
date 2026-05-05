DELETE internal/git/git.go:161 HasUncommittedChanges — no production references; replaced by direct git command usage elsewhere.
DELETE internal/git/git.go:191 GetCurrentBranch — no production references; branch reads are not part of runtime call graph.
DELETE internal/git/git.go:226 CommitChanges — no production references; commits are performed directly at call sites.
DELETE internal/git/git.go:319 validatePrefix — unused validation helper with no production callers.
DELETE internal/git/git.go:346 validateRepoURL — unused validation helper with no production callers.
DELETE internal/git/git.go:391 validateBranchName — unused validation helper with no production callers.
DELETE internal/git/git.go:417 validateCommitMessage — unused validation helper with no production callers.
DELETE internal/git/git.go:435 sanitizeInput — unused sanitization helper with no production callers.
DELETE internal/git/git.go:451 sanitizeCommitMessage — unused sanitization helper with no production callers.
