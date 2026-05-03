package agentmetrics

// Bucket is a coarse outcome family aligned with the
// agent.ExecuteBeadStatus* terminal statuses. It exists so Story 11
// aggregations (success rate, wasted cost, etc.) can group attempts
// without re-deriving the status taxonomy at every call site.
type Bucket string

const (
	BucketSuccess        Bucket = "success"
	BucketNoChanges      Bucket = "no_changes"
	BucketNoEvidence     Bucket = "no_evidence"
	BucketPreserved      Bucket = "preserved"
	BucketDecomposed     Bucket = "decomposed"
	BucketReviewRejected Bucket = "review_rejected"
	BucketLandConflict   Bucket = "land_conflict"
	BucketRatchetFailed  Bucket = "ratchet_failed"
	BucketExecFailed     Bucket = "exec_failed"
	BucketUnknown        Bucket = "unknown"
)

// ClassifyBucket maps an ExecuteBeadStatus value (as written to result.json
// `status` and to FEAT-010 run records) to its bucket. Status strings here
// must stay in sync with cli/internal/agent/execute_bead_status.go.
//
// Per the Story 11 locked decision, already_satisfied counts as success.
func ClassifyBucket(status string) Bucket {
	switch status {
	case "success", "already_satisfied":
		return BucketSuccess
	case "no_changes":
		return BucketNoChanges
	case "no_evidence_produced":
		return BucketNoEvidence
	case "preserved_needs_review":
		return BucketPreserved
	case "declined_needs_decomposition":
		return BucketDecomposed
	case "review_request_changes", "review_block", "review_malfunction":
		return BucketReviewRejected
	case "land_conflict", "land_conflict_unresolvable", "land_conflict_needs_human", "push_failed", "push_conflict":
		return BucketLandConflict
	case "ratchet_failed":
		return BucketRatchetFailed
	case "execution_failed", "post_run_check_failed", "structural_validation_failed":
		return BucketExecFailed
	default:
		return BucketUnknown
	}
}

// Successful reports whether the bucket is a closed-bead success for
// success-rate computations. Only BucketSuccess (success +
// already_satisfied) qualifies.
func (b Bucket) Successful() bool {
	return b == BucketSuccess
}
