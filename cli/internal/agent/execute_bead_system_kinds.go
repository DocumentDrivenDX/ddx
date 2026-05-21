package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// runReviewFinding handles execution of review-finding beads.
// It extracts the review findings from the bead payload and writes evidence.
func runReviewFinding(
	ctx context.Context,
	beadID string,
	b *bead.Bead,
	store ExecuteBeadLoopStore,
	projectRoot string,
	assignee string,
	now func() time.Time,
) (ExecuteBeadReport, error) {
	report := ExecuteBeadReport{
		BeadID: beadID,
		Status: ExecuteBeadStatusSuccess,
	}

	if b == nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = "bead not found"
		return report, nil
	}

	// Extract payload from bead Extra.
	payload, err := extractReviewFindingPayload(b)
	if err != nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = fmt.Sprintf("extract payload: %v", err)
		return report, nil
	}

	// Record the review finding as an event.
	evt := bead.BeadEvent{
		Kind:      "system-review-finding",
		Summary:   fmt.Sprintf("Review verdict: %s", payload.Verdict),
		Body:      payload.Findings,
		Actor:     assignee,
		Source:    "ddx execute-loop",
		CreatedAt: now().UTC(),
	}

	if err := store.AppendEvent(beadID, evt); err != nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = fmt.Sprintf("append event: %v", err)
		return report, nil
	}

	report.Detail = fmt.Sprintf("Review findings recorded: %s from %s", payload.Verdict, payload.ReviewedBy)
	return report, nil
}

// runAlignmentReview handles execution of alignment-review beads.
// It extracts the alignment findings from the bead payload and writes evidence.
func runAlignmentReview(
	ctx context.Context,
	beadID string,
	b *bead.Bead,
	store ExecuteBeadLoopStore,
	projectRoot string,
	assignee string,
	now func() time.Time,
) (ExecuteBeadReport, error) {
	report := ExecuteBeadReport{
		BeadID: beadID,
		Status: ExecuteBeadStatusSuccess,
	}

	if b == nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = "bead not found"
		return report, nil
	}

	// Extract payload from bead Extra.
	payload, err := extractAlignmentReviewPayload(b)
	if err != nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = fmt.Sprintf("extract payload: %v", err)
		return report, nil
	}

	// Record the alignment review as an event.
	evt := bead.BeadEvent{
		Kind:      "system-alignment-review",
		Summary:   fmt.Sprintf("Alignment review of %s", payload.Document),
		Body:      payload.Alignment,
		Actor:     assignee,
		Source:    "ddx execute-loop",
		CreatedAt: now().UTC(),
	}

	if err := store.AppendEvent(beadID, evt); err != nil {
		report.Status = ExecuteBeadStatusExecutionFailed
		report.Error = fmt.Sprintf("append event: %v", err)
		return report, nil
	}

	report.Detail = fmt.Sprintf("Alignment review completed by %s for %s", payload.UpdatedBy, payload.Document)
	return report, nil
}

// extractReviewFindingPayload extracts a ReviewFindingPayload from a bead's Extra field.
func extractReviewFindingPayload(b *bead.Bead) (*bead.ReviewFindingPayload, error) {
	if b == nil || b.Extra == nil {
		return nil, fmt.Errorf("bead or extra is nil")
	}

	payloadData, ok := b.Extra["payload"]
	if !ok {
		return nil, fmt.Errorf("payload not found in bead extra")
	}

	// Type assertion from map[string]interface{} (JSON unmarshaling result).
	payloadMap, ok := payloadData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("payload has unexpected type: %T", payloadData)
	}

	payload := &bead.ReviewFindingPayload{}

	if v, ok := payloadMap["verdict"]; ok {
		if s, ok := v.(string); ok {
			payload.Verdict = s
		}
	}
	if v, ok := payloadMap["findings"]; ok {
		if s, ok := v.(string); ok {
			payload.Findings = s
		}
	}
	if v, ok := payloadMap["result_rev"]; ok {
		if s, ok := v.(string); ok {
			payload.ResultRev = s
		}
	}
	if v, ok := payloadMap["reviewed_by"]; ok {
		if s, ok := v.(string); ok {
			payload.ReviewedBy = s
		}
	}

	return payload, nil
}

// extractAlignmentReviewPayload extracts an AlignmentReviewPayload from a bead's Extra field.
func extractAlignmentReviewPayload(b *bead.Bead) (*bead.AlignmentReviewPayload, error) {
	if b == nil || b.Extra == nil {
		return nil, fmt.Errorf("bead or extra is nil")
	}

	payloadData, ok := b.Extra["payload"]
	if !ok {
		return nil, fmt.Errorf("payload not found in bead extra")
	}

	// Type assertion from map[string]interface{} (JSON unmarshaling result).
	payloadMap, ok := payloadData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("payload has unexpected type: %T", payloadData)
	}

	payload := &bead.AlignmentReviewPayload{}

	if v, ok := payloadMap["document"]; ok {
		if s, ok := v.(string); ok {
			payload.Document = s
		}
	}
	if v, ok := payloadMap["alignment"]; ok {
		if s, ok := v.(string); ok {
			payload.Alignment = s
		}
	}
	if v, ok := payloadMap["updated_by"]; ok {
		if s, ok := v.(string); ok {
			payload.UpdatedBy = s
		}
	}

	return payload, nil
}

// systemKindDispatcher dispatches system-generated kinds (review-finding, alignment-review).
// Returns true if the bead kind was handled by this dispatcher.
func systemKindDispatcher(
	ctx context.Context,
	beadID string,
	b *bead.Bead,
	store ExecuteBeadLoopStore,
	projectRoot string,
	assignee string,
	now func() time.Time,
) (ExecuteBeadReport, bool, error) {
	if b == nil || b.IssueType == "" {
		return ExecuteBeadReport{}, false, nil
	}

	switch b.IssueType {
	case bead.IssueTypeReviewFinding:
		report, err := runReviewFinding(ctx, beadID, b, store, projectRoot, assignee, now)
		return report, true, err
	case bead.IssueTypeAlignmentReview:
		report, err := runAlignmentReview(ctx, beadID, b, store, projectRoot, assignee, now)
		return report, true, err
	default:
		return ExecuteBeadReport{}, false, nil
	}
}
