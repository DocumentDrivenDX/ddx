package agentmetrics

import "testing"

// TestClassifyBucket pins the status → bucket mapping for every
// terminal-aligned status declared in agent/execute_bead_status.go. If a
// new ExecuteBeadStatus value is added there, this test must be updated
// to keep aggregations from silently bucketing it as "unknown".
func TestClassifyBucket(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   Bucket
	}{
		{"success", "success", BucketSuccess},
		// already_satisfied counts as success per the Story 11
		// locked decision.
		{"already_satisfied", "already_satisfied", BucketSuccess},
		{"no_changes", "no_changes", BucketNoChanges},
		{"no_evidence_produced", "no_evidence_produced", BucketNoEvidence},
		{"preserved_needs_review", "preserved_needs_review", BucketPreserved},
		{"declined_needs_decomposition", "declined_needs_decomposition", BucketDecomposed},
		{"review_request_changes", "review_request_changes", BucketReviewRejected},
		{"review_block", "review_block", BucketReviewRejected},
		{"review_malfunction", "review_malfunction", BucketReviewRejected},
		{"land_conflict", "land_conflict", BucketLandConflict},
		{"land_conflict_unresolvable", "land_conflict_unresolvable", BucketLandConflict},
		{"land_conflict_needs_human", "land_conflict_needs_human", BucketLandConflict},
		{"push_failed", "push_failed", BucketLandConflict},
		{"push_conflict", "push_conflict", BucketLandConflict},
		{"ratchet_failed", "ratchet_failed", BucketRatchetFailed},
		{"execution_failed", "execution_failed", BucketExecFailed},
		{"post_run_check_failed", "post_run_check_failed", BucketExecFailed},
		{"structural_validation_failed", "structural_validation_failed", BucketExecFailed},
		{"empty", "", BucketUnknown},
		{"unrecognized", "future_status_xyz", BucketUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyBucket(tc.status)
			if got != tc.want {
				t.Fatalf("ClassifyBucket(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestBucketSuccessful(t *testing.T) {
	if !BucketSuccess.Successful() {
		t.Fatalf("BucketSuccess must be Successful()")
	}
	if !ClassifyBucket("already_satisfied").Successful() {
		t.Fatalf("already_satisfied must classify as a successful bucket")
	}
	for _, b := range []Bucket{
		BucketNoChanges, BucketNoEvidence, BucketPreserved,
		BucketDecomposed, BucketReviewRejected, BucketLandConflict,
		BucketRatchetFailed, BucketExecFailed, BucketUnknown,
	} {
		if b.Successful() {
			t.Fatalf("bucket %q must not be Successful()", b)
		}
	}
}
