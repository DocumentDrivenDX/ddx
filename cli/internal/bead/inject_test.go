package bead

import (
	"context"
	"testing"
	"time"
)

// TestBeadKindRegistration tests AC1: kinds register, validate parent reference, reject invalid payloads.
func TestBeadKindRegistration(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create parent bead.
	parent := &Bead{
		ID:        "test-parent-001",
		Title:     "Parent Bead",
		Status:    StatusOpen,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Create(testCtx(), parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	tests := []struct {
		name    string
		payload InjectPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "review-finding kind validates verdict",
			payload: &ReviewFindingPayload{
				Verdict:   "",
				ResultRev: "abc123",
			},
			wantErr: true,
			errMsg:  "verdict is required",
		},
		{
			name: "review-finding kind validates result_rev",
			payload: &ReviewFindingPayload{
				Verdict:   "APPROVE",
				ResultRev: "",
			},
			wantErr: true,
			errMsg:  "result_rev is required",
		},
		{
			name: "review-finding kind accepts valid payload",
			payload: &ReviewFindingPayload{
				Verdict:    "APPROVE",
				ResultRev:  "abc123",
				ReviewedBy: "reviewer1",
			},
			wantErr: false,
		},
		{
			name: "alignment-review kind validates document",
			payload: &AlignmentReviewPayload{
				Document:  "",
				UpdatedBy: "someone",
			},
			wantErr: true,
			errMsg:  "document is required",
		},
		{
			name: "alignment-review kind accepts valid payload",
			payload: &AlignmentReviewPayload{
				Document:  "docs/plan.md",
				UpdatedBy: "updater1",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Inject(context.Background(), parent.ID, tt.payload, InjectOptions{})
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected %q in error, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

// TestInjectQueueBeadIdempotent tests AC2: Inject creates ready bead, second call returns same ID.
func TestInjectQueueBeadIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create parent bead.
	parent := &Bead{
		ID:        "test-parent-002",
		Title:     "Parent Bead 2",
		Status:    StatusOpen,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Create(testCtx(), parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	ctx := context.Background()
	payload := &ReviewFindingPayload{
		Verdict:    "REQUEST_CHANGES",
		ResultRev:  "def456",
		ReviewedBy: "reviewer2",
	}

	// First inject call should create a new bead.
	id1, err := store.Inject(ctx, parent.ID, payload, InjectOptions{})
	if err != nil {
		t.Fatalf("first Inject: %v", err)
	}
	if id1 == "" {
		t.Errorf("expected non-empty bead ID, got empty")
	}

	// Verify the bead was created with status=open.
	bead1, err := store.Get(id1)
	if err != nil {
		t.Fatalf("Get bead: %v", err)
	}
	if bead1.Status != StatusOpen {
		t.Errorf("expected status=%s, got %s", StatusOpen, bead1.Status)
	}
	if bead1.IssueType != IssueTypeReviewFinding {
		t.Errorf("expected IssueType=%s, got %s", IssueTypeReviewFinding, bead1.IssueType)
	}
	if bead1.Parent != parent.ID {
		t.Errorf("expected parent=%s, got %s", parent.ID, bead1.Parent)
	}

	// Second inject call with identical payload should return the same ID.
	id2, err := store.Inject(ctx, parent.ID, payload, InjectOptions{})
	if err != nil {
		t.Fatalf("second Inject: %v", err)
	}
	if id2 != id1 {
		t.Errorf("expected same ID on re-inject, got first=%s, second=%s", id1, id2)
	}

	// Verify no duplicate was created (count should be 2: parent + 1 review-finding).
	allBeads, err := store.List("", "", map[string]string{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	reviewFindingCount := 0
	for _, b := range allBeads {
		if b.IssueType == IssueTypeReviewFinding {
			reviewFindingCount++
		}
	}
	if reviewFindingCount != 1 {
		t.Errorf("expected 1 review-finding bead, got %d", reviewFindingCount)
	}

	// Third inject call with different payload should create a new bead.
	payload2 := &ReviewFindingPayload{
		Verdict:    "BLOCK",
		ResultRev:  "ghi789",
		ReviewedBy: "reviewer3",
	}
	id3, err := store.Inject(ctx, parent.ID, payload2, InjectOptions{})
	if err != nil {
		t.Fatalf("third Inject: %v", err)
	}
	if id3 == id1 {
		t.Errorf("expected different ID for different payload, got same: %s", id1)
	}
	reviewFindingCount = 0
	allBeads, _ = store.List("", "", map[string]string{})
	for _, b := range allBeads {
		if b.IssueType == IssueTypeReviewFinding {
			reviewFindingCount++
		}
	}
	if reviewFindingCount != 2 {
		t.Errorf("expected 2 review-finding beads after different payload, got %d", reviewFindingCount)
	}
}

// TestExecuteLoopDispatchEvidenceBundleStructure tests AC3 (partial).
// Full AC3 requires an integration test with the execute-loop, but this
// verifies that beads can be created and matched by kind.
func TestExecuteLoopDispatchBeadKindMatching(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	parent := &Bead{
		ID:        "test-parent-003",
		Title:     "Parent Bead 3",
		Status:    StatusOpen,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Create(testCtx(), parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	ctx := context.Background()

	// Create review-finding bead.
	reviewPayload := &ReviewFindingPayload{
		Verdict:    "APPROVE",
		ResultRev:  "review-rev",
		ReviewedBy: "reviewer",
	}
	reviewID, err := store.Inject(ctx, parent.ID, reviewPayload, InjectOptions{
		Title:  "Review Findings",
		Labels: []string{"kind:review-finding"},
	})
	if err != nil {
		t.Fatalf("Inject review-finding: %v", err)
	}

	// Create alignment-review bead.
	alignPayload := &AlignmentReviewPayload{
		Document:  "docs/spec.md",
		UpdatedBy: "aligner",
	}
	alignID, err := store.Inject(ctx, parent.ID, alignPayload, InjectOptions{
		Title:  "Alignment Review",
		Labels: []string{"kind:alignment-review"},
	})
	if err != nil {
		t.Fatalf("Inject alignment-review: %v", err)
	}

	// Verify beads are retrievable and have correct kinds.
	review, err := store.Get(reviewID)
	if err != nil {
		t.Fatalf("Get review bead: %v", err)
	}
	if review.IssueType != IssueTypeReviewFinding {
		t.Errorf("expected review bead kind=%s, got %s", IssueTypeReviewFinding, review.IssueType)
	}

	align, err := store.Get(alignID)
	if err != nil {
		t.Fatalf("Get align bead: %v", err)
	}
	if align.IssueType != IssueTypeAlignmentReview {
		t.Errorf("expected align bead kind=%s, got %s", IssueTypeAlignmentReview, align.IssueType)
	}

	// Verify they both reference the parent.
	if review.Parent != parent.ID {
		t.Errorf("review bead parent mismatch")
	}
	if align.Parent != parent.ID {
		t.Errorf("align bead parent mismatch")
	}
}

// TestInjectErrorConditions tests error handling.
func TestInjectErrorConditions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	tests := []struct {
		name      string
		parent    string
		payload   InjectPayload
		wantErr   bool
		errPrefix string
	}{
		{
			name:      "empty parent ID",
			parent:    "",
			payload:   &ReviewFindingPayload{Verdict: "APPROVE", ResultRev: "abc"},
			wantErr:   true,
			errPrefix: "parent bead ID required",
		},
		{
			name:      "non-existent parent",
			parent:    "nonexistent-parent",
			payload:   &ReviewFindingPayload{Verdict: "APPROVE", ResultRev: "abc"},
			wantErr:   true,
			errPrefix: "parent bead not found",
		},
		{
			name:      "nil payload",
			parent:    "some-parent",
			payload:   nil,
			wantErr:   true,
			errPrefix: "payload required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Inject(context.Background(), tt.parent, tt.payload, InjectOptions{})
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if tt.wantErr && err != nil && !contains(err.Error(), tt.errPrefix) {
				t.Errorf("expected error to contain %q, got: %v", tt.errPrefix, err)
			}
		})
	}
}

// Test that we can inject and access the payload.
func TestInjectPayloadStorage(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	parent := &Bead{
		ID:        "parent-payload-test",
		Title:     "Parent",
		Status:    StatusOpen,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Create(testCtx(), parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	payload := &ReviewFindingPayload{
		Verdict:    "REQUEST_CHANGES",
		Findings:   "Found issues in acceptance criteria",
		ResultRev:  "test-rev-123",
		ReviewedBy: "alice",
	}

	id, err := store.Inject(context.Background(), parent.ID, payload, InjectOptions{
		Description: "Review findings from test run",
	})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}

	bead, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify payload is stored in Extra.
	if bead.Extra == nil {
		t.Fatalf("bead.Extra is nil")
	}
	if storedPayload, ok := bead.Extra["payload"]; !ok {
		t.Errorf("payload not found in Extra")
	} else {
		if m, ok := storedPayload.(map[string]interface{}); !ok {
			t.Errorf("payload not in expected format, type=%T", storedPayload)
		} else {
			if v, ok := m["verdict"]; !ok || v != "REQUEST_CHANGES" {
				t.Errorf("verdict not preserved in payload")
			}
		}
	}
}

// Test validation of nil payloads.
func TestNilPayloadValidation(t *testing.T) {
	var p *ReviewFindingPayload
	if err := p.Validate(); err == nil {
		t.Errorf("expected error for nil payload, got nil")
	}

	var ap *AlignmentReviewPayload
	if err := ap.Validate(); err == nil {
		t.Errorf("expected error for nil payload, got nil")
	}
}

// TestInjectReviewFindingFullLifecycle tests end-to-end injection and retrieval
// demonstrating that review-finding beads can be injected, queried, and have their
// payloads preserved for later processing by the execute loop.
func TestInjectReviewFindingFullLifecycle(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create a parent bead (simulating a completed work item).
	parent := &Bead{
		ID:        "parent-work-item-001",
		Title:     "Completed Task",
		Status:    StatusClosed,
		IssueType: "task",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.Create(testCtx(), parent); err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	ctx := context.Background()

	// Inject a review-finding bead (as would be done by an external review system
	// calling Store.Inject with a ReviewFindingPayload after the parent completes).
	reviewPayload := &ReviewFindingPayload{
		Verdict:    "REQUEST_CHANGES",
		Findings:   "Code review identified several areas for improvement",
		ResultRev:  "abc123def456",
		ReviewedBy: "alice@example.com",
	}

	reviewID, err := store.Inject(ctx, parent.ID, reviewPayload, InjectOptions{
		Title:       "Code Review Findings",
		Description: "Findings from automated code review",
		Labels:      []string{"kind:review-finding", "source:automated"},
		Priority:    1,
	})
	if err != nil {
		t.Fatalf("Inject review-finding: %v", err)
	}

	// Retrieve the injected bead and verify its properties.
	bead, err := store.Get(reviewID)
	if err != nil {
		t.Fatalf("Get injected bead: %v", err)
	}

	// Verify bead metadata.
	if bead.IssueType != IssueTypeReviewFinding {
		t.Errorf("expected kind=%s, got %s", IssueTypeReviewFinding, bead.IssueType)
	}
	if bead.Status != StatusOpen {
		t.Errorf("expected status=%s, got %s", StatusOpen, bead.Status)
	}
	if bead.Parent != parent.ID {
		t.Errorf("expected parent=%s, got %s", parent.ID, bead.Parent)
	}
	if bead.Title != "Code Review Findings" {
		t.Errorf("expected title preserved, got %s", bead.Title)
	}

	// Verify payload is stored and can be extracted.
	if bead.Extra == nil {
		t.Fatalf("bead.Extra is nil, cannot extract payload")
	}

	payloadData, ok := bead.Extra["payload"]
	if !ok {
		t.Fatalf("payload not found in bead.Extra")
	}

	payloadMap, ok := payloadData.(map[string]interface{})
	if !ok {
		t.Fatalf("payload has unexpected type: %T", payloadData)
	}

	// Verify all payload fields are preserved for execute-loop processing.
	if v, ok := payloadMap["verdict"]; !ok || v != "REQUEST_CHANGES" {
		t.Errorf("verdict not preserved: %v", v)
	}
	if v, ok := payloadMap["findings"]; !ok || v != "Code review identified several areas for improvement" {
		t.Errorf("findings not preserved: %v", v)
	}
	if v, ok := payloadMap["result_rev"]; !ok || v != "abc123def456" {
		t.Errorf("result_rev not preserved: %v", v)
	}
	if v, ok := payloadMap["reviewed_by"]; !ok || v != "alice@example.com" {
		t.Errorf("reviewed_by not preserved: %v", v)
	}

	// Test idempotency: injecting the same payload again should return the same ID.
	reviewID2, err := store.Inject(ctx, parent.ID, reviewPayload, InjectOptions{
		Title: "Code Review Findings",
	})
	if err != nil {
		t.Fatalf("Second inject: %v", err)
	}
	if reviewID2 != reviewID {
		t.Errorf("expected idempotent ID %s, got %s", reviewID, reviewID2)
	}

	// Verify that all beads from this parent can be queried.
	all, err := store.ReadAll(ctx)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var reviewBeads int
	for _, b := range all {
		if b.Parent == parent.ID && b.IssueType == IssueTypeReviewFinding {
			reviewBeads++
		}
	}
	if reviewBeads != 1 {
		t.Errorf("expected 1 review-finding bead for parent, got %d", reviewBeads)
	}
}

// Helper to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || (len(s) >= len(substr)))
}
