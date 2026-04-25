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
