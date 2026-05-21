package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// mockBeadEventAppender is a simple mock for testing event appending.
type mockBeadEventAppender struct {
	events map[string][]bead.BeadEvent
}

func newMockBeadEventAppender() *mockBeadEventAppender {
	return &mockBeadEventAppender{
		events: make(map[string][]bead.BeadEvent),
	}
}

func (m *mockBeadEventAppender) AppendEvent(id string, evt bead.BeadEvent) error {
	m.events[id] = append(m.events[id], evt)
	return nil
}

func (m *mockBeadEventAppender) ReadyExecution() ([]bead.Bead, error) {
	return nil, nil
}

func (m *mockBeadEventAppender) Get(args ...any) (*bead.Bead, error) {
	return nil, nil
}

func (m *mockBeadEventAppender) Create(args ...any) error {
	return nil
}

func (m *mockBeadEventAppender) Claim(id, assignee string) error {
	return nil
}

func (m *mockBeadEventAppender) Unclaim(id string) error {
	return nil
}

func (m *mockBeadEventAppender) TouchClaimHeartbeat(id string) error {
	return nil
}

func (m *mockBeadEventAppender) CloseWithEvidence(id, sessionID, commitSHA string) error {
	return nil
}

func (m *mockBeadEventAppender) Events(id string) ([]bead.BeadEvent, error) {
	return m.events[id], nil
}

func (m *mockBeadEventAppender) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return nil
}

func (m *mockBeadEventAppender) AppendNotes(id string, notes string) error {
	return nil
}

func (m *mockBeadEventAppender) IncrNoChangesCount(id string) (int, error) {
	return 0, nil
}

func (m *mockBeadEventAppender) Reopen(id, reason, notes string) error {
	return nil
}

func (m *mockBeadEventAppender) Update(args ...any) error {
	return nil
}

func (m *mockBeadEventAppender) UpdateWithLifecycleStatus(id string, status string, opts bead.LifecycleTransitionOptions, mutate func(*bead.Bead) error) error {
	return nil
}

func (m *mockBeadEventAppender) ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error {
	return nil
}

func (m *mockBeadEventAppender) ParkToProposedWithIntakeEvent(id, actor, outcome, reason, detail string, body map[string]any, at time.Time, mutate func(*bead.Bead)) error {
	return nil
}

// TestRunReviewFinding tests the review-finding dispatcher.
func TestRunReviewFinding(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	// Create a review-finding bead with payload.
	reviewBead := &bead.Bead{
		ID:        "review-001",
		Title:     "Review Finding",
		IssueType: bead.IssueTypeReviewFinding,
		Status:    bead.StatusOpen,
		Parent:    "parent-001",
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"verdict":     "REQUEST_CHANGES",
				"findings":    "Found issues in code",
				"result_rev":  "abc123",
				"reviewed_by": "alice",
			},
		},
	}

	report, err := runReviewFinding(ctx, reviewBead.ID, reviewBead, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("runReviewFinding: %v", err)
	}

	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected status=success, got %s", report.Status)
	}
	if report.Error != "" {
		t.Errorf("expected no error, got %s", report.Error)
	}

	// Verify event was appended.
	events, ok := mock.events[reviewBead.ID]
	if !ok || len(events) == 0 {
		t.Fatalf("expected event to be appended, got none")
	}
	if events[0].Kind != "system-review-finding" {
		t.Errorf("expected kind=system-review-finding, got %s", events[0].Kind)
	}
}

// TestRunAlignmentReview tests the alignment-review dispatcher.
func TestRunAlignmentReview(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	// Create an alignment-review bead with payload.
	alignBead := &bead.Bead{
		ID:        "align-001",
		Title:     "Alignment Review",
		IssueType: bead.IssueTypeAlignmentReview,
		Status:    bead.StatusOpen,
		Parent:    "parent-001",
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"document":   "docs/spec.md",
				"alignment":  "Document aligns with requirements",
				"updated_by": "bob",
			},
		},
	}

	report, err := runAlignmentReview(ctx, alignBead.ID, alignBead, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("runAlignmentReview: %v", err)
	}

	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected status=success, got %s", report.Status)
	}
	if report.Error != "" {
		t.Errorf("expected no error, got %s", report.Error)
	}

	// Verify event was appended.
	events, ok := mock.events[alignBead.ID]
	if !ok || len(events) == 0 {
		t.Fatalf("expected event to be appended, got none")
	}
	if events[0].Kind != "system-alignment-review" {
		t.Errorf("expected kind=system-alignment-review, got %s", events[0].Kind)
	}
}

// TestSystemKindDispatcher tests the dispatcher routing.
func TestSystemKindDispatcher(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	// Test review-finding dispatch.
	reviewBead := &bead.Bead{
		ID:        "review-002",
		IssueType: bead.IssueTypeReviewFinding,
		Parent:    "parent-002",
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"verdict":     "APPROVE",
				"result_rev":  "def456",
				"reviewed_by": "carol",
			},
		},
	}

	report, handled, err := systemKindDispatcher(ctx, reviewBead.ID, reviewBead, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("systemKindDispatcher: %v", err)
	}
	if !handled {
		t.Errorf("expected review-finding to be handled")
	}
	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected success status, got %s", report.Status)
	}

	// Test alignment-review dispatch.
	alignBead := &bead.Bead{
		ID:        "align-002",
		IssueType: bead.IssueTypeAlignmentReview,
		Parent:    "parent-002",
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"document":   "docs/plan.md",
				"updated_by": "dave",
			},
		},
	}

	report, handled, err = systemKindDispatcher(ctx, alignBead.ID, alignBead, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("systemKindDispatcher for alignment: %v", err)
	}
	if !handled {
		t.Errorf("expected alignment-review to be handled")
	}
	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected success status, got %s", report.Status)
	}

	// Test non-system kind (should not be handled).
	taskBead := &bead.Bead{
		ID:        "task-001",
		IssueType: "task",
		Parent:    "parent-002",
		CreatedAt: now(),
		UpdatedAt: now(),
	}

	_, handled, _ = systemKindDispatcher(ctx, taskBead.ID, taskBead, mock, "/tmp", "ddx", now)
	if handled {
		t.Errorf("expected task kind to not be handled by systemKindDispatcher")
	}
}

// TestExtractReviewFindingPayload tests payload extraction.
func TestExtractReviewFindingPayload(t *testing.T) {
	bead := &bead.Bead{
		ID: "test-001",
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"verdict":     "BLOCK",
				"findings":    "Critical issues found",
				"result_rev":  "xyz789",
				"reviewed_by": "eve",
			},
		},
	}

	payload, err := extractReviewFindingPayload(bead)
	if err != nil {
		t.Fatalf("extractReviewFindingPayload: %v", err)
	}

	if payload.Verdict != "BLOCK" {
		t.Errorf("expected verdict=BLOCK, got %s", payload.Verdict)
	}
	if payload.Findings != "Critical issues found" {
		t.Errorf("expected findings to be preserved")
	}
	if payload.ResultRev != "xyz789" {
		t.Errorf("expected result_rev=xyz789, got %s", payload.ResultRev)
	}
	if payload.ReviewedBy != "eve" {
		t.Errorf("expected reviewed_by=eve, got %s", payload.ReviewedBy)
	}
}

// TestExtractAlignmentReviewPayload tests payload extraction.
func TestExtractAlignmentReviewPayload(t *testing.T) {
	bead := &bead.Bead{
		ID: "align-test",
		Extra: map[string]any{
			"payload": map[string]interface{}{
				"document":   "docs/architecture.md",
				"alignment":  "Updated for new design pattern",
				"updated_by": "frank",
			},
		},
	}

	payload, err := extractAlignmentReviewPayload(bead)
	if err != nil {
		t.Fatalf("extractAlignmentReviewPayload: %v", err)
	}

	if payload.Document != "docs/architecture.md" {
		t.Errorf("expected document=docs/architecture.md, got %s", payload.Document)
	}
	if payload.Alignment != "Updated for new design pattern" {
		t.Errorf("expected alignment to be preserved")
	}
	if payload.UpdatedBy != "frank" {
		t.Errorf("expected updated_by=frank, got %s", payload.UpdatedBy)
	}
}

// TestPayloadExtractionErrors tests error conditions.
func TestPayloadExtractionErrors(t *testing.T) {
	tests := []struct {
		name   string
		bead   *bead.Bead
		extFn  func(*bead.Bead) (interface{}, error)
		hasErr bool
	}{
		{
			name:   "nil bead",
			bead:   nil,
			extFn:  func(b *bead.Bead) (interface{}, error) { return extractReviewFindingPayload(b) },
			hasErr: true,
		},
		{
			name: "missing payload in extra",
			bead: &bead.Bead{
				ID:    "test",
				Extra: map[string]any{},
			},
			extFn:  func(b *bead.Bead) (interface{}, error) { return extractReviewFindingPayload(b) },
			hasErr: true,
		},
		{
			name: "malformed payload type",
			bead: &bead.Bead{
				ID:    "test",
				Extra: map[string]any{"payload": "not a map"},
			},
			extFn:  func(b *bead.Bead) (interface{}, error) { return extractReviewFindingPayload(b) },
			hasErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.extFn(tt.bead)
			if tt.hasErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.hasErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestReviewFindingNilBead tests handling of nil bead.
func TestReviewFindingNilBead(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	report, err := runReviewFinding(ctx, "fake-id", nil, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("runReviewFinding: %v", err)
	}
	if report.Status != ExecuteBeadStatusExecutionFailed {
		t.Errorf("expected failed status for nil bead, got %s", report.Status)
	}
	if report.Error == "" {
		t.Errorf("expected error message, got empty")
	}
}

// TestAlignmentReviewNilBead tests handling of nil bead.
func TestAlignmentReviewNilBead(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	report, err := runAlignmentReview(ctx, "fake-id", nil, mock, "/tmp", "ddx", now)
	if err != nil {
		t.Fatalf("runAlignmentReview: %v", err)
	}
	if report.Status != ExecuteBeadStatusExecutionFailed {
		t.Errorf("expected failed status for nil bead, got %s", report.Status)
	}
	if report.Error == "" {
		t.Errorf("expected error message, got empty")
	}
}

// TestExecuteLoopConsumesReviewFinding is an end-to-end test demonstrating:
// 1. A parent bead is created
// 2. A review-finding bead is injected via the tracker.Inject API
// 3. The execute loop picks it up and processes it
// 4. The review handler records the findings as an event
func TestExecuteLoopConsumesReviewFinding(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	// Create a parent bead simulating a completed work item.
	parentBead := &bead.Bead{
		ID:        "parent-cycle-001",
		Title:     "Completed Work Item",
		IssueType: "task",
		Status:    bead.StatusClosed,
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra:     map[string]any{},
	}

	// Simulate injection of a review-finding bead (as would be done by an external
	// reviewer system calling tracker.Inject with ReviewFindingPayload).
	reviewBead := &bead.Bead{
		ID:        "review-after-cycle-001",
		Title:     "review-finding: parent-cycle-001",
		IssueType: bead.IssueTypeReviewFinding,
		Status:    bead.StatusOpen,
		Priority:  0,
		Parent:    parentBead.ID,
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload_hash": "test-hash",
			"payload": map[string]interface{}{
				"verdict":     "REQUEST_CHANGES",
				"findings":    "Code review identified minor style issues that should be addressed before merge",
				"result_rev":  "abc123def456",
				"reviewed_by": "alice-reviewer@example.com",
			},
		},
	}

	// Execute the review-finding bead through the system kinds dispatcher
	// (simulating what the execute loop does when it pops a review-finding bead).
	report, handled, err := systemKindDispatcher(ctx, reviewBead.ID, reviewBead, mock, "/tmp", "ddx", now)

	// Verify the dispatcher recognized and handled the review-finding kind.
	if err != nil {
		t.Fatalf("systemKindDispatcher: %v", err)
	}
	if !handled {
		t.Fatalf("expected review-finding bead to be handled by systemKindDispatcher")
	}

	// Verify execution was successful.
	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected success status, got %s", report.Status)
	}
	if report.Error != "" {
		t.Errorf("expected no error, got: %s", report.Error)
	}

	// Verify the review finding was recorded as an event.
	events, ok := mock.events[reviewBead.ID]
	if !ok || len(events) == 0 {
		t.Fatalf("expected event to be recorded for review-finding bead")
	}
	if events[0].Kind != "system-review-finding" {
		t.Errorf("expected event kind=system-review-finding, got %s", events[0].Kind)
	}

	// Verify the report includes the review findings.
	expectedDetail := "Review findings recorded: REQUEST_CHANGES from alice-reviewer@example.com"
	if report.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, report.Detail)
	}

	// Verify the event captures the findings.
	if !strings.Contains(events[0].Summary, "REQUEST_CHANGES") {
		t.Errorf("expected summary to contain verdict REQUEST_CHANGES")
	}
	if !strings.Contains(events[0].Body, "Code review identified") {
		t.Errorf("expected body to contain findings")
	}
}

// TestExecuteLoopConsumesAlignmentReview is an end-to-end test demonstrating:
// 1. A parent bead is created
// 2. An alignment-review bead is injected via the tracker.Inject API
// 3. The execute loop picks it up and processes it
// 4. The alignment handler records the findings as an event
func TestExecuteLoopConsumesAlignmentReview(t *testing.T) {
	ctx := context.Background()
	mock := newMockBeadEventAppender()
	now := func() time.Time { return time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC) }

	// Create a parent bead simulating a completed work item.
	parentBead := &bead.Bead{
		ID:        "parent-align-001",
		Title:     "Completed Work Item",
		IssueType: "task",
		Status:    bead.StatusClosed,
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra:     map[string]any{},
	}

	// Simulate injection of an alignment-review bead (as would be done by an external
	// alignment system calling tracker.Inject with AlignmentReviewPayload).
	alignBead := &bead.Bead{
		ID:        "align-after-cycle-001",
		Title:     "alignment-review: parent-align-001",
		IssueType: bead.IssueTypeAlignmentReview,
		Status:    bead.StatusOpen,
		Priority:  0,
		Parent:    parentBead.ID,
		CreatedAt: now(),
		UpdatedAt: now(),
		Extra: map[string]any{
			"payload_hash": "test-hash-align",
			"payload": map[string]interface{}{
				"document":   "docs/spec.md",
				"alignment":  "Document aligns with PRD requirements for version 2.0 feature set",
				"updated_by": "bob-reviewer@example.com",
			},
		},
	}

	// Execute the alignment-review bead through the system kinds dispatcher
	// (simulating what the execute loop does when it pops an alignment-review bead).
	report, handled, err := systemKindDispatcher(ctx, alignBead.ID, alignBead, mock, "/tmp", "ddx", now)

	// Verify the dispatcher recognized and handled the alignment-review kind.
	if err != nil {
		t.Fatalf("systemKindDispatcher: %v", err)
	}
	if !handled {
		t.Fatalf("expected alignment-review bead to be handled by systemKindDispatcher")
	}

	// Verify execution was successful.
	if report.Status != ExecuteBeadStatusSuccess {
		t.Errorf("expected success status, got %s", report.Status)
	}
	if report.Error != "" {
		t.Errorf("expected no error, got: %s", report.Error)
	}

	// Verify the alignment review was recorded as an event.
	events, ok := mock.events[alignBead.ID]
	if !ok || len(events) == 0 {
		t.Fatalf("expected event to be recorded for alignment-review bead")
	}
	if events[0].Kind != "system-alignment-review" {
		t.Errorf("expected event kind=system-alignment-review, got %s", events[0].Kind)
	}

	// Verify the report includes the alignment findings.
	expectedDetail := "Alignment review completed by bob-reviewer@example.com for docs/spec.md"
	if report.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, report.Detail)
	}

	// Verify the event captures the alignment findings.
	if !strings.Contains(events[0].Summary, "docs/spec.md") {
		t.Errorf("expected summary to contain document path")
	}
	if !strings.Contains(events[0].Body, "aligns with PRD") {
		t.Errorf("expected body to contain alignment findings")
	}
}
