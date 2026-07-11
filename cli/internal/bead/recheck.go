package bead

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

const (
	RecheckBlockerOutcomeReopened     = "reopened"
	RecheckBlockerOutcomeBlocked      = "blocked"
	RecheckBlockerOutcomeManual       = "manual"
	RecheckBlockerOutcomeUnresolvable = "unresolvable"
)

// RecheckBlockerResult describes what happened to one blocked bead during a
// blocker recheck pass.
type RecheckBlockerResult struct {
	BeadID         string `json:"bead_id"`
	Status         string `json:"status"`
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
	Repo           string `json:"repo,omitempty"`
	TargetBead     string `json:"target_bead,omitempty"`
	ObservedStatus string `json:"observed_status,omitempty"`
}

// RecheckBlockers iterates the external-blocked beads in store, resolves any
// structured cross-repo blocker refs against knownRepos, and reopens the bead
// when the referenced bead is confirmed closed.
//
// beadID is an optional filter. When non-empty, only the named blocked bead is
// rechecked. Missing or malformed structured refs are reported as manual-only
// blockers and left unchanged.
func RecheckBlockers(ctx context.Context, s *Store, knownRepos map[string]config.KnownRepoConfig, beadID string) ([]RecheckBlockerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("bead: recheck blockers requires a store")
	}

	blocked, err := s.ExternalBlocked()
	if err != nil {
		return nil, err
	}
	if beadID = strings.TrimSpace(beadID); beadID != "" {
		filtered := make([]Bead, 0, 1)
		for _, b := range blocked {
			if b.ID == beadID {
				filtered = append(filtered, b)
			}
		}
		blocked = filtered
	}

	results := make([]RecheckBlockerResult, 0, len(blocked))
	if len(blocked) == 0 {
		return results, nil
	}

	baseDir := s.workingDir()
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "."
	}

	for _, b := range blocked {
		if err := ctx.Err(); err != nil {
			return results, err
		}

		result := RecheckBlockerResult{
			BeadID:  b.ID,
			Status:  b.Status,
			Outcome: RecheckBlockerOutcomeBlocked,
		}

		var rawRef any
		hasRef := false
		if b.Extra != nil {
			rawRef, hasRef = b.Extra[ExtraLifecycleCrossRepoBlockerRef]
		}
		if !hasRef {
			result.Outcome = RecheckBlockerOutcomeManual
			result.Reason = "manual blocker: no structured cross-repo ref"
			results = append(results, result)
			continue
		}
		ref, ok := ParseCrossRepoBlockerRef(rawRef)
		if !ok {
			result.Outcome = RecheckBlockerOutcomeUnresolvable
			result.Reason = "unresolvable: malformed structured cross-repo blocker ref"
			results = append(results, result)
			continue
		}
		result.Repo = ref.Repo
		result.TargetBead = ref.Bead

		repoCfg, ok := knownRepos[strings.TrimSpace(ref.Repo)]
		if !ok {
			result.Outcome = RecheckBlockerOutcomeUnresolvable
			result.Reason = fmt.Sprintf("unresolvable: unknown known-repo %q", ref.Repo)
			results = append(results, result)
			continue
		}

		targetDDxDir, reason := resolveKnownRepoDDxDir(baseDir, ref.Repo, repoCfg)
		if reason != "" {
			result.Outcome = RecheckBlockerOutcomeUnresolvable
			result.Reason = reason
			results = append(results, result)
			continue
		}

		targetStore := NewStore(targetDDxDir)
		target, err := targetStore.GetWithArchive(ctx, ref.Bead)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				result.Outcome = RecheckBlockerOutcomeUnresolvable
				result.Reason = fmt.Sprintf("unresolvable: target bead %q not found in repo %q", ref.Bead, ref.Repo)
			} else {
				result.Outcome = RecheckBlockerOutcomeUnresolvable
				result.Reason = fmt.Sprintf("unresolvable: read target bead %q from repo %q: %v", ref.Bead, ref.Repo, err)
			}
			results = append(results, result)
			continue
		}

		result.ObservedStatus = target.Status
		if !LifecycleStatusSatisfiesDependency(target.Status) {
			result.Outcome = RecheckBlockerOutcomeBlocked
			result.Reason = fmt.Sprintf("target bead %q status=%s", ref.Bead, target.Status)
			results = append(results, result)
			continue
		}

		if err := reopenRecheckedBlockedBead(s, b.ID, ref, target.Status); err != nil {
			return results, err
		}
		result.Outcome = RecheckBlockerOutcomeReopened
		result.Status = StatusOpen
		result.Reason = fmt.Sprintf("target bead %q closed", ref.Bead)
		results = append(results, result)
	}

	return results, nil
}

func resolveKnownRepoDDxDir(baseDir, repoAlias string, repoCfg config.KnownRepoConfig) (string, string) {
	if strings.TrimSpace(repoCfg.NodeID) != "" || strings.TrimSpace(repoCfg.ProjectID) != "" {
		return "", fmt.Sprintf("unresolvable: federation not yet supported for repo %q", repoAlias)
	}

	repoPath := strings.TrimSpace(repoCfg.Path)
	if repoPath == "" {
		return "", fmt.Sprintf("unresolvable: known-repo %q has no local path", repoAlias)
	}
	if !filepath.IsAbs(repoPath) {
		repoPath = filepath.Join(baseDir, repoPath)
	}
	repoPath = filepath.Clean(repoPath)

	ddxDir := repoPath
	if filepath.Base(ddxDir) != ddxroot.DirName {
		ddxDir = filepath.Join(ddxDir, ddxroot.DirName)
	}

	info, err := os.Stat(ddxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Sprintf("unresolvable: repo path missing: %s", ddxDir)
		}
		return "", fmt.Sprintf("unresolvable: repo path unreadable: %s: %v", ddxDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Sprintf("unresolvable: repo path is not a directory: %s", ddxDir)
	}
	return ddxDir, ""
}

func reopenRecheckedBlockedBead(s *Store, beadID string, ref CrossRepoBlockerRef, observedStatus string) error {
	now := time.Now().UTC()
	body, err := json.Marshal(map[string]any{
		"blocked_bead_id": beadID,
		"source_repo":     ref.Repo,
		"source_bead":     ref.Bead,
		"observed_status": observedStatus,
	})
	if err != nil {
		return fmt.Errorf("bead: marshal cross-repo recheck event: %w", err)
	}

	if err := s.TransitionLifecycle(beadID, StatusOpen, LifecycleTransitionOptions{
		ManualReopen: true,
		Actor:        "system:cross-repo-recheck",
		Source:       ref.Repo,
		Reason:       fmt.Sprintf("cross-repo blocker cleared: %s#%s", ref.Repo, ref.Bead),
	}, func(b *Bead) error {
		if b == nil {
			return fmt.Errorf("bead: cross-repo recheck requires bead")
		}
		b.Owner = ""
		clearClaimExtraKeys(b.Extra)
		if hasEventsAttachment(b) {
			if err := s.inlineEventsInPlace(b); err != nil {
				return err
			}
			_ = os.Remove(s.eventsAttachmentPath(b.ID))
		}
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		var events []BeadEvent
		if raw, ok := b.Extra["events"]; ok {
			events = decodeBeadEvents(raw)
		}
		events = append(events, BeadEvent{
			Kind:      "cross_repo_blocker_recheck",
			Summary:   fmt.Sprintf("reopened after %s#%s closed", ref.Repo, ref.Bead),
			Body:      string(body),
			Actor:     "system:cross-repo-recheck",
			Source:    ref.Repo,
			CreatedAt: now,
		})
		b.Extra["events"] = encodeEventsForExtra(events)
		return nil
	}); err != nil {
		return err
	}

	return s.RemoveClaimHeartbeat(beadID)
}
