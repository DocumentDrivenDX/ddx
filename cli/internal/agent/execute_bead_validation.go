package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// AttemptIntegrityPreserveReason is the canonical landing Reason written when
// DDx rejects an otherwise-clean attempt because it violated the execute-bead
// integrity contract (post-commit mutation, empty pre-commit gate evidence, or
// uncommitted changes after the implementation commit). ClassifyExecuteBeadStatus
// and classifyLandingFailureMode match on this exact string to route the
// attempt to preserved_needs_review + FailureModeAttemptIntegrity rather than
// letting it land as success. The specific human explanation lives in the
// result's Error field; this constant is the stable machine marker.
const AttemptIntegrityPreserveReason = "execute-bead integrity violation"

// CommitEvent is one HEAD-moving entry parsed from a worktree's reflog. Action
// is the leading verb of the reflog message ("commit", "commit (amend)",
// "commit (initial)", "rebase (finish)", "checkout", ...); SHA is the commit
// the entry pointed HEAD at.
type CommitEvent struct {
	SHA     string
	Action  string
	Subject string
}

// PreCommitGateRun captures one invocation of a pre-commit gate (e.g.
// `lefthook run pre-commit`) and the output it produced, so DDx can tell a
// meaningful staged-file gate from a no-op "no staged files" run.
type PreCommitGateRun struct {
	Command string
	Output  string
}

// AttemptIntegrityInput is the structured snapshot the post-agent validation
// step compares before marking an attempt successful: the commit events the
// agent produced, the final implementation revision, the worktree dirty state,
// and the pre-commit gate evidence. See ValidateAttemptIntegrity.
type AttemptIntegrityInput struct {
	BaseRev           string
	ImplementationRev string
	CommitEvents      []CommitEvent
	DirtyPaths        []string
	// CodeChanging is true when the attempt produced an implementation commit,
	// i.e. the bead changed tracked files and the gate/dirty contracts apply.
	CodeChanging bool
	GateRuns     []PreCommitGateRun
}

// AttemptIntegrityVerdict is the result of ValidateAttemptIntegrity. OK is true
// when the attempt satisfies the integrity contract. When OK is false, Reason
// is a short machine code and Detail is an operator-facing explanation that
// makes clear the rejection came from DDx validation, not from an
// implementation/agent failure.
type AttemptIntegrityVerdict struct {
	OK     bool
	Reason string
	Detail string
}

// Integrity reason codes (the Reason field of AttemptIntegrityVerdict).
const (
	IntegrityReasonPostCommitMutation = "post_commit_mutation"
	IntegrityReasonEmptyGateEvidence  = "empty_gate_evidence"
	IntegrityReasonDirtyAfterCommit   = "dirty_after_commit"
)

// reflogLineRe matches a `git reflog show HEAD` line:
//
//	<sha> HEAD@{N}: <message>
var reflogLineRe = regexp.MustCompile(`^(\S+)\s+HEAD@\{\d+\}:\s+(.*)$`)

// ParseHeadReflog parses `git reflog show HEAD` output lines (newest-first, as
// git emits them) into CommitEvents ordered oldest-first, so callers can reason
// about the first commit an agent made versus later rewrites. Lines that do not
// match the reflog shape are skipped.
func ParseHeadReflog(lines []string) []CommitEvent {
	var newestFirst []CommitEvent
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		m := reflogLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		sha := m[1]
		message := m[2]
		action := message
		subject := ""
		if idx := strings.Index(message, ": "); idx >= 0 {
			action = message[:idx]
			subject = message[idx+2:]
		}
		newestFirst = append(newestFirst, CommitEvent{
			SHA:     sha,
			Action:  strings.TrimSpace(action),
			Subject: strings.TrimSpace(subject),
		})
	}
	// Reverse to oldest-first.
	out := make([]CommitEvent, 0, len(newestFirst))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out = append(out, newestFirst[i])
	}
	return out
}

// classifyLefthookOutput buckets a single pre-commit gate run's output as
// "meaningful" (at least one hook actually executed against staged files),
// "no_staged_files" (the run was a no-op because nothing was staged), or
// "unknown" (neither signal present). The no_staged_files markers mirror the
// phrasing lefthook and the execute-bead prompt use for an empty staged set.
func classifyLefthookOutput(output string) string {
	lower := strings.ToLower(output)
	noStaged := containsAny(lower,
		"no files for inspection",
		"no staged files",
		"no matching staged files",
		"no-staged-files",
		"no files to inspect",
		"no files matched",
	)
	// "executed in" / the success glyphs / "running hook" only appear when a
	// hook actually ran. Note: "summary: (skip)" is NOT a meaningful marker —
	// lefthook prints it for no-op runs — so it is deliberately excluded.
	meaningful := containsAny(lower,
		"executed in",
		"✔",
		"✓",
		"❯",
		"running hook",
	)
	switch {
	case meaningful:
		return "meaningful"
	case noStaged:
		return "no_staged_files"
	default:
		return "unknown"
	}
}

// ValidateAttemptIntegrity performs the structured post-agent integrity check.
// It is pure so it can be exercised directly by regression tests that simulate
// an execute-bead transcript. It rejects an attempt when:
//
//   - the implementation commit was amended or replaced after the first commit
//     (post-commit mutation), comparing the first commit event against the final
//     implementation revision;
//   - the bead changed code but every observed pre-commit gate run reported no
//     staged files, so the gate never tested the committed change;
//   - the bead changed code but tracked files remained uncommitted after the
//     implementation commit.
//
// Checks that lack evidence (no commit events observed, no gate runs captured)
// are skipped rather than failed, so the validator never rejects an attempt it
// could not actually observe.
func ValidateAttemptIntegrity(in AttemptIntegrityInput) AttemptIntegrityVerdict {
	if mutated, detail := detectPostCommitMutation(in.CommitEvents, in.ImplementationRev); mutated {
		return AttemptIntegrityVerdict{
			Reason: IntegrityReasonPostCommitMutation,
			Detail: detail,
		}
	}

	if in.CodeChanging && len(in.GateRuns) > 0 {
		sawMeaningful := false
		sawNoStaged := false
		for _, run := range in.GateRuns {
			switch classifyLefthookOutput(run.Output) {
			case "meaningful":
				sawMeaningful = true
			case "no_staged_files":
				sawNoStaged = true
			}
		}
		if !sawMeaningful && sawNoStaged {
			return AttemptIntegrityVerdict{
				Reason: IntegrityReasonEmptyGateEvidence,
				Detail: "DDx validation: every pre-commit gate run reported no staged files, so the required pre-commit gate never tested the committed change; this is not acceptance evidence. Detected by DDx, not an implementation failure.",
			}
		}
	}

	if in.CodeChanging && len(in.DirtyPaths) > 0 {
		return AttemptIntegrityVerdict{
			Reason: IntegrityReasonDirtyAfterCommit,
			Detail: fmt.Sprintf("DDx validation: tracked files remained uncommitted after the implementation commit (%s); the contract requires a clean worktree. Detected by DDx, not an implementation failure.", strings.Join(in.DirtyPaths, ", ")),
		}
	}

	return AttemptIntegrityVerdict{OK: true}
}

// detectPostCommitMutation reports whether the agent rewrote its implementation
// commit after the first commit. It returns true when more than one commit
// event is present and either an amend appears after the first commit or the
// first commit's SHA differs from the final implementation revision. A single
// commit whose SHA matches the implementation revision is clean.
func detectPostCommitMutation(events []CommitEvent, implementationRev string) (bool, string) {
	var commits []CommitEvent
	for _, ev := range events {
		if strings.HasPrefix(ev.Action, "commit") {
			commits = append(commits, ev)
		}
	}
	if len(commits) < 2 {
		return false, ""
	}
	first := commits[0]
	hasAmend := false
	for _, c := range commits[1:] {
		if strings.Contains(c.Action, "amend") {
			hasAmend = true
			break
		}
	}
	replaced := implementationRev != "" && first.SHA != "" && !shaEqual(first.SHA, implementationRev)
	if !hasAmend && !replaced {
		return false, ""
	}
	detail := fmt.Sprintf(
		"DDx validation: the implementation commit was rewritten after the first commit (%d commit events; first %s, final implementation_rev %s). The execute-bead contract requires exactly one commit with no post-commit amend/rebase. Detected by DDx, not an implementation failure; rerun the bead in a fresh attempt.",
		len(commits), shortSHA(first.SHA), shortSHA(implementationRev),
	)
	return true, detail
}

// shaEqual reports whether two git revisions refer to the same commit, allowing
// for one side being an abbreviation of the other.
func shaEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if len(a) < 7 || len(b) < 7 {
		return false
	}
	if len(a) < len(b) {
		return strings.HasPrefix(b, a)
	}
	return strings.HasPrefix(a, b)
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "(none)"
	}
	return sha
}

// readWorktreeCommitEvents reads the worktree's HEAD reflog and parses it into
// oldest-first CommitEvents. Returns nil when the reflog is unavailable (e.g. a
// non-git path or a worktree already cleaned up), in which case the post-commit
// mutation check is skipped.
func readWorktreeCommitEvents(wtPath string) []CommitEvent {
	if wtPath == "" {
		return nil
	}
	out, err := internalgit.Command(context.Background(), wtPath, "reflog", "show", "HEAD", "--no-abbrev").Output()
	if err != nil {
		return nil
	}
	return ParseHeadReflog(strings.Split(string(out), "\n"))
}

// CanonicalRootDirtyPaths returns tracked files with uncommitted staged or
// unstaged modifications in projectRoot, excluding .ddx/ paths (DDx-managed
// state, not user WIP) and untracked (??) entries. The execute-bead loop calls
// this before claiming any bead to detect a WIP canonical project root that
// would cause a churn loop of claim → workspace-prep failure → unclaim → repeat.
// Returns nil when projectRoot is empty, git is unavailable, or the root is clean.
func CanonicalRootDirtyPaths(projectRoot string) []string {
	if projectRoot == "" {
		return nil
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "status", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		if code == "??" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" || seen[path] {
			continue
		}
		// Exclude .ddx/ paths — those are DDx-managed files, not user WIP.
		if strings.HasPrefix(path, ".ddx/") {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

// integrityDirtyPaths returns the tracked files that have staged or unstaged
// modifications in the worktree (porcelain status with a non-space, non-`?`
// status code). Untracked and ignored files are excluded so harness scratch
// files and the gitignored evidence dir never trip the dirty check; only real
// uncommitted edits to tracked files are reported.
func integrityDirtyPaths(wtPath string) []string {
	if wtPath == "" {
		return nil
	}
	out, err := internalgit.Command(context.Background(), wtPath, "status", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		// Skip untracked ("??") entries; only tracked modifications matter.
		if code == "??" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}
