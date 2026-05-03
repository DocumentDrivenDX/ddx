package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// OperatorPromptBacklinkEventKind is the BeadEvent.Kind appended to every
// bead created or modified by the execution of an operator-prompt bead. The
// event body carries the originating operator-prompt's ID so downstream
// audit tooling (UI, evidence reports) can trace any later mutation back
// to the prompt that introduced it. A summary event with the same kind is
// also appended to the originator listing every affected bead and artifact
// path so a single read on the operator-prompt bead surfaces the full
// blast radius. (FEAT operator-prompts, Story 15 §Additional security
// controls bullet 6.)
const OperatorPromptBacklinkEventKind = "origin_operator_prompt_id"

// operatorPromptAffected lists the beads created/modified and the artifact
// paths touched by an operator-prompt bead's harness during one successful
// attempt. It is the input to RecordOperatorPromptBacklinks.
type operatorPromptAffected struct {
	BeadIDs   []string
	Artifacts []string
}

// computeOperatorPromptAffected diffs base..result in projectRoot and
// classifies the changed paths. Mutations to the bead JSONL stores
// (.ddx/beads.jsonl and .ddx/beads-archive.jsonl) yield bead IDs (parsed
// out of the added-line JSON); every other changed path is treated as an
// artifact. The originator's own ID is filtered out so the originator
// does not back-link to itself. Returns nil/empty fields when base==result
// or no diff is available.
func computeOperatorPromptAffected(projectRoot, baseRev, resultRev, originatorID string) (operatorPromptAffected, error) {
	if projectRoot == "" || baseRev == "" || resultRev == "" || baseRev == resultRev {
		return operatorPromptAffected{}, nil
	}
	gitOps := RealLandingGitOps{}
	paths, err := gitOps.DiffNameOnly(projectRoot, baseRev, resultRev)
	if err != nil {
		return operatorPromptAffected{}, fmt.Errorf("operator-prompt backlinks: diff %s..%s: %w", baseRev, resultRev, err)
	}

	beadIDSet := map[string]struct{}{}
	var artifacts []string
	for _, p := range paths {
		switch p {
		case ".ddx/beads.jsonl", ".ddx/beads-archive.jsonl":
			ids, perr := extractBeadIDsFromDiff(projectRoot, baseRev, resultRev, p)
			if perr != nil {
				return operatorPromptAffected{}, perr
			}
			for _, id := range ids {
				beadIDSet[id] = struct{}{}
			}
		default:
			artifacts = append(artifacts, p)
		}
	}
	delete(beadIDSet, originatorID)

	beadIDs := make([]string, 0, len(beadIDSet))
	for id := range beadIDSet {
		beadIDs = append(beadIDs, id)
	}
	sort.Strings(beadIDs)
	sort.Strings(artifacts)
	return operatorPromptAffected{BeadIDs: beadIDs, Artifacts: artifacts}, nil
}

// extractBeadIDsFromDiff parses `git diff base tip -- path` and returns the
// IDs from every added line that decodes as a bead JSON object. Only added
// lines are considered (a deletion alone is not a "create or modify" of a
// bead — closed beads still see a status-change diff and so still appear
// as added lines). Best-effort: malformed JSON lines are silently skipped.
func extractBeadIDsFromDiff(projectRoot, baseRev, resultRev, path string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), projectRoot, "diff", "--no-color", "--unified=0", baseRev, resultRev, "--", path).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("operator-prompt backlinks: diff %s: %s: %w", path, strings.TrimSpace(string(out)), err)
	}
	var ids []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		body := strings.TrimSpace(line[1:])
		if body == "" || body[0] != '{' {
			continue
		}
		var rec struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &rec); err != nil || rec.ID == "" {
			continue
		}
		if _, ok := seen[rec.ID]; ok {
			continue
		}
		seen[rec.ID] = struct{}{}
		ids = append(ids, rec.ID)
	}
	return ids, nil
}

// recordOperatorPromptBacklinks appends `origin_operator_prompt_id` events
// to every bead in affected.BeadIDs (back-link from each affected bead to
// the originator) and one summary event on the originator listing every
// affected bead + artifact path. Idempotent across a single attempt:
// re-running with the same affected set produces an additional event per
// affected bead — callers must guard against double-invocation. Errors on
// individual AppendEvent calls are reported as a joined error string but
// do NOT short-circuit subsequent appends, so a single store hiccup does
// not lose every other backlink.
func recordOperatorPromptBacklinks(store ExecuteBeadLoopStore, originatorID string, affected operatorPromptAffected, actor string, now time.Time) error {
	if originatorID == "" {
		return fmt.Errorf("operator-prompt backlinks: originator ID is required")
	}
	if len(affected.BeadIDs) == 0 && len(affected.Artifacts) == 0 {
		return nil
	}
	var errs []string
	body := fmt.Sprintf("origin_operator_prompt_id=%s", originatorID)
	for _, id := range affected.BeadIDs {
		if err := store.AppendEvent(id, bead.BeadEvent{
			Kind:      OperatorPromptBacklinkEventKind,
			Summary:   "operator-prompt backlink",
			Body:      body,
			Actor:     actor,
			Source:    "ddx agent execute-loop",
			CreatedAt: now,
		}); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", id, err))
		}
	}
	// Summary event on the originator: lists every affected bead/artifact.
	summaryBody := fmt.Sprintf(
		"affected_beads=%s affected_artifacts=%s",
		strings.Join(affected.BeadIDs, ","),
		strings.Join(affected.Artifacts, ","),
	)
	if err := store.AppendEvent(originatorID, bead.BeadEvent{
		Kind:      OperatorPromptBacklinkEventKind,
		Summary:   "operator-prompt blast radius",
		Body:      summaryBody,
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: now,
	}); err != nil {
		errs = append(errs, fmt.Sprintf("originator %s: %v", originatorID, err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("operator-prompt backlinks: %s", strings.Join(errs, "; "))
	}
	return nil
}
