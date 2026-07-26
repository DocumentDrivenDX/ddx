package git

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DefaultBranchPreflightKind classifies the network-free default-branch diagnostic.
// Consumers (queue pre-claim, ddx doctor, CLI surfaces) render consistent messages
// from this structured status without re-deriving git state.
type DefaultBranchPreflightKind string

const (
	// DefaultBranchOK: current branch upstream branch name matches origin/HEAD
	// and local is not behind/ahead/diverged relative to the observed upstream tip.
	DefaultBranchOK DefaultBranchPreflightKind = "ok"
	// DefaultBranchMismatch: configured upstream branch differs from origin/HEAD target.
	DefaultBranchMismatch DefaultBranchPreflightKind = "mismatch"
	// DefaultBranchDetachedHEAD: HEAD is not attached to a branch.
	DefaultBranchDetachedHEAD DefaultBranchPreflightKind = "detached_head"
	// DefaultBranchMissingOriginHEAD: refs/remotes/origin/HEAD is not set locally.
	DefaultBranchMissingOriginHEAD DefaultBranchPreflightKind = "missing_origin_head"
	// DefaultBranchMissingUpstream: current branch has no configured upstream.
	DefaultBranchMissingUpstream DefaultBranchPreflightKind = "missing_upstream"
	// DefaultBranchBehind: local tip is a strict ancestor of the observed upstream tip.
	DefaultBranchBehind DefaultBranchPreflightKind = "behind"
	// DefaultBranchAhead: observed upstream tip is a strict ancestor of local tip.
	DefaultBranchAhead DefaultBranchPreflightKind = "ahead"
	// DefaultBranchDiverged: local and observed upstream tips have diverged.
	DefaultBranchDiverged DefaultBranchPreflightKind = "diverged"
	// DefaultBranchNoOrigin: no origin remote is configured (single-machine / no remote).
	DefaultBranchNoOrigin DefaultBranchPreflightKind = "no_origin"
)

// DefaultBranchPreflightSeverity is the action class for the diagnostic.
type DefaultBranchPreflightSeverity string

const (
	// SeverityPass: safe to proceed.
	SeverityPass DefaultBranchPreflightSeverity = "pass"
	// SeverityHardFail: refuse drain / claim until the operator remediates.
	SeverityHardFail DefaultBranchPreflightSeverity = "hard_fail"
	// SeverityWarn: non-blocking advisory (e.g. local behind upstream).
	SeverityWarn DefaultBranchPreflightSeverity = "warn"
)

// DefaultBranchPreflight is the structured result of CheckDefaultBranchPreflight.
// All fields are populated from already-observed local refs and config only;
// the check never performs network I/O (reliability principle P9).
type DefaultBranchPreflight struct {
	Kind           DefaultBranchPreflightKind
	Severity       DefaultBranchPreflightSeverity
	CurrentBranch  string // empty when detached
	UpstreamRef    string // e.g. "refs/heads/main" from branch.<name>.merge, or "refs/remotes/origin/main"
	UpstreamBranch string // short branch name implied by the configured upstream
	DefaultBranch  string // short branch name origin/HEAD points at
	OriginHEADRef  string // full symref, e.g. "refs/remotes/origin/master"
	LocalSHA       string
	UpstreamSHA    string
	Message        string
}

// HardFail reports whether the diagnostic should block queue claim / drain.
func (r DefaultBranchPreflight) HardFail() bool {
	return r.Severity == SeverityHardFail
}

// Pass reports whether the checkout is on the remote default branch with no
// advisory ancestry issues.
func (r DefaultBranchPreflight) Pass() bool {
	return r.Severity == SeverityPass
}

// Warn reports whether the diagnostic is advisory only.
func (r DefaultBranchPreflight) Warn() bool {
	return r.Severity == SeverityWarn
}

// CheckDefaultBranchPreflight resolves HEAD attachment, the current branch's
// configured upstream, origin/HEAD's default-branch target, and local-vs-upstream
// ancestry using already-observed refs. It never runs git fetch.
//
// Intended consumers: queue pre-claim gates, ddx work/try startup, ddx doctor.
func CheckDefaultBranchPreflight(dir string) DefaultBranchPreflight {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Detached HEAD?
	headOut, headErr := Command(ctx, dir, "symbolic-ref", "-q", "HEAD").CombinedOutput()
	if headErr != nil {
		return DefaultBranchPreflight{
			Kind:     DefaultBranchDetachedHEAD,
			Severity: SeverityHardFail,
			Message:  "HEAD is detached; check out a branch that tracks the remote default branch before draining the queue (e.g. git switch <default-branch>)",
		}
	}
	// symbolic-ref -q HEAD returns refs/heads/<branch>
	headRef := strings.TrimSpace(string(headOut))
	currentBranch := strings.TrimPrefix(headRef, "refs/heads/")
	if currentBranch == "" || currentBranch == headRef {
		// unexpected form — treat as detached for safety
		return DefaultBranchPreflight{
			Kind:     DefaultBranchDetachedHEAD,
			Severity: SeverityHardFail,
			Message:  "HEAD is detached; check out a branch that tracks the remote default branch before draining the queue (e.g. git switch <default-branch>)",
		}
	}

	// 2. Origin remote present?
	if _, err := Command(ctx, dir, "remote", "get-url", "origin").CombinedOutput(); err != nil {
		return DefaultBranchPreflight{
			Kind:          DefaultBranchNoOrigin,
			Severity:      SeverityPass,
			CurrentBranch: currentBranch,
			Message:       "no origin remote configured; default-branch preflight skipped",
		}
	}

	// 3. origin/HEAD default branch
	originHEADRef, defaultBranch := resolveOriginHEAD(ctx, dir)
	if defaultBranch == "" {
		return DefaultBranchPreflight{
			Kind:          DefaultBranchMissingOriginHEAD,
			Severity:      SeverityWarn,
			CurrentBranch: currentBranch,
			Message:       "refs/remotes/origin/HEAD is not set; run `git remote set-head origin -a` or `ddx sync` so the remote default branch is known",
		}
	}

	// 4. Configured upstream for current branch
	upstreamRef, upstreamBranch := resolveConfiguredUpstream(ctx, dir, currentBranch)
	if upstreamBranch == "" {
		return DefaultBranchPreflight{
			Kind:          DefaultBranchMissingUpstream,
			Severity:      SeverityHardFail,
			CurrentBranch: currentBranch,
			DefaultBranch: defaultBranch,
			OriginHEADRef: originHEADRef,
			Message: fmt.Sprintf(
				"branch %q has no configured upstream; set upstream to the remote default branch %q (%s) before draining (e.g. git branch -u origin/%s)",
				currentBranch, defaultBranch, originHEADRef, defaultBranch,
			),
		}
	}

	// 5. Upstream branch name vs origin/HEAD target
	if upstreamBranch != defaultBranch {
		// Prefer naming the configured merge ref and origin/HEAD full ref so
		// operators see the exact misconfiguration (branch.<name>.merge vs origin/HEAD).
		displayUpstream := upstreamRef
		if displayUpstream == "" {
			displayUpstream = "refs/heads/" + upstreamBranch
		}
		return DefaultBranchPreflight{
			Kind:           DefaultBranchMismatch,
			Severity:       SeverityHardFail,
			CurrentBranch:  currentBranch,
			UpstreamRef:    displayUpstream,
			UpstreamBranch: upstreamBranch,
			DefaultBranch:  defaultBranch,
			OriginHEADRef:  originHEADRef,
			Message: fmt.Sprintf(
				"current branch %q upstream %s does not match remote default branch %s; refusing to drain — fix branch.%s.merge or check out the default branch (override with --allow-non-default-branch when intentional)",
				currentBranch, displayUpstream, originHEADRef, currentBranch,
			),
		}
	}

	// 6. Local vs observed upstream remote-tracking ancestry (no fetch)
	localSHA, upSHA, anc := localUpstreamAncestry(ctx, dir, currentBranch, upstreamBranch)
	base := DefaultBranchPreflight{
		CurrentBranch:  currentBranch,
		UpstreamRef:    upstreamRef,
		UpstreamBranch: upstreamBranch,
		DefaultBranch:  defaultBranch,
		OriginHEADRef:  originHEADRef,
		LocalSHA:       localSHA,
		UpstreamSHA:    upSHA,
	}

	switch anc {
	case "behind":
		base.Kind = DefaultBranchBehind
		base.Severity = SeverityWarn
		base.Message = fmt.Sprintf(
			"local branch %q is behind its upstream origin/%s (local=%s upstream=%s); run `ddx sync` or `git pull --ff-only` before draining",
			currentBranch, upstreamBranch, shortSHA(localSHA), shortSHA(upSHA),
		)
		return base
	case "ahead":
		base.Kind = DefaultBranchAhead
		base.Severity = SeverityPass
		base.Message = fmt.Sprintf(
			"local branch %q is ahead of origin/%s (local commits not yet on the observed upstream tip)",
			currentBranch, upstreamBranch,
		)
		return base
	case "diverged":
		base.Kind = DefaultBranchDiverged
		base.Severity = SeverityWarn
		base.Message = fmt.Sprintf(
			"local branch %q has diverged from origin/%s (local=%s upstream=%s); reconcile before draining",
			currentBranch, upstreamBranch, shortSHA(localSHA), shortSHA(upSHA),
		)
		return base
	default:
		// "equal", "unknown" (no observed remote-tracking tip), or empty
		base.Kind = DefaultBranchOK
		base.Severity = SeverityPass
		base.Message = fmt.Sprintf(
			"current branch %q upstream matches remote default branch %s",
			currentBranch, originHEADRef,
		)
		return base
	}
}

func resolveOriginHEAD(ctx context.Context, dir string) (fullRef, branch string) {
	out, err := Command(ctx, dir, "symbolic-ref", "refs/remotes/origin/HEAD").CombinedOutput()
	if err != nil {
		return "", ""
	}
	ref := strings.TrimSpace(string(out))
	const prefix = "refs/remotes/origin/"
	if !strings.HasPrefix(ref, prefix) {
		return ref, ""
	}
	return ref, strings.TrimPrefix(ref, prefix)
}

// resolveConfiguredUpstream reads branch.<name>.merge / .remote (and falls back
// to @{u}) so misconfigurations like branch.master.merge=refs/heads/main are
// visible even when the remote-tracking ref for that merge target is absent.
func resolveConfiguredUpstream(ctx context.Context, dir, branch string) (upstreamRef, upstreamBranch string) {
	mergeOut, mergeErr := Command(ctx, dir, "config", "--get", "branch."+branch+".merge").CombinedOutput()
	if mergeErr == nil {
		merge := strings.TrimSpace(string(mergeOut))
		if merge != "" {
			upstreamRef = merge
			upstreamBranch = branchNameFromMergeRef(merge)
			return upstreamRef, upstreamBranch
		}
	}

	// Fallback: symbolic upstream (origin/<branch>) when config keys are unset
	// but tracking is still resolvable.
	out, err := Command(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}").CombinedOutput()
	if err != nil {
		return "", ""
	}
	abbrev := strings.TrimSpace(string(out)) // e.g. origin/main
	if abbrev == "" {
		return "", ""
	}
	if idx := strings.Index(abbrev, "/"); idx >= 0 {
		upstreamBranch = abbrev[idx+1:]
	} else {
		upstreamBranch = abbrev
	}
	// Prefer a full remote-tracking ref form when we know the remote.
	remoteOut, remoteErr := Command(ctx, dir, "config", "--get", "branch."+branch+".remote").CombinedOutput()
	remote := "origin"
	if remoteErr == nil {
		if r := strings.TrimSpace(string(remoteOut)); r != "" {
			remote = r
		}
	}
	upstreamRef = "refs/remotes/" + remote + "/" + upstreamBranch
	return upstreamRef, upstreamBranch
}

func branchNameFromMergeRef(merge string) string {
	merge = strings.TrimSpace(merge)
	if strings.HasPrefix(merge, "refs/heads/") {
		return strings.TrimPrefix(merge, "refs/heads/")
	}
	if strings.HasPrefix(merge, "refs/remotes/") {
		// refs/remotes/<remote>/<branch>
		rest := strings.TrimPrefix(merge, "refs/remotes/")
		if idx := strings.Index(rest, "/"); idx >= 0 {
			return rest[idx+1:]
		}
		return rest
	}
	// Already a short name
	return merge
}

// localUpstreamAncestry compares refs/heads/<localBranch> to the last-observed
// refs/remotes/origin/<upstreamBranch>. Returns localSHA, upstreamSHA, and one of
// "equal", "behind", "ahead", "diverged", "unknown". Never fetches.
func localUpstreamAncestry(ctx context.Context, dir, localBranch, upstreamBranch string) (localSHA, upSHA, relation string) {
	localOut, localErr := Command(ctx, dir, "rev-parse", "--verify", "refs/heads/"+localBranch).CombinedOutput()
	if localErr != nil {
		return "", "", "unknown"
	}
	localSHA = strings.TrimSpace(string(localOut))

	upOut, upErr := Command(ctx, dir, "rev-parse", "--verify", "refs/remotes/origin/"+upstreamBranch).CombinedOutput()
	if upErr != nil {
		// No observed remote-tracking tip yet — fail open on ancestry.
		return localSHA, "", "unknown"
	}
	upSHA = strings.TrimSpace(string(upOut))

	if localSHA == upSHA {
		return localSHA, upSHA, "equal"
	}

	// local ancestor of upstream? → local is behind
	if Command(ctx, dir, "merge-base", "--is-ancestor", localSHA, upSHA).Run() == nil {
		return localSHA, upSHA, "behind"
	}
	// upstream ancestor of local? → local is ahead
	if Command(ctx, dir, "merge-base", "--is-ancestor", upSHA, localSHA).Run() == nil {
		return localSHA, upSHA, "ahead"
	}
	return localSHA, upSHA, "diverged"
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
