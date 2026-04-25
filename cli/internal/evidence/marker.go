package evidence

// TruncationMarker is the canonical string downstream parsers check for to
// detect that a section was clamped by an evidence primitive. Equality
// against this constant is the contract; do not redefine locally.
const TruncationMarker = "\n\n[…truncated by ddx evidence cap…]\n"

// Pre-dispatch overflow outcome classes (FEAT-022 §7, §12). These are the
// strings the reviewer and grader paths emit when bounded assembly still
// exceeds MaxPromptBytes.
const (
	OutcomeReviewContextOverflow  = "review-error: context_overflow"
	OutcomeCompareContextOverflow = "compare-error: context_overflow"
)

// Review-error taxonomy (FEAT-022 §12). The four classes the reviewer outcome
// event distinguishes. context_overflow is emitted by the pre-dispatch
// short-circuit (Stage C1); the remaining three classify post-dispatch
// failures. Operators triage on these literal strings, so they are part of
// the external contract.
const (
	OutcomeReviewProviderEmpty = "review-error: provider_empty"
	OutcomeReviewUnparseable   = "review-error: unparseable"
	OutcomeReviewTransport     = "review-error: transport"
)
