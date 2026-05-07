package agent

import "fmt"

// PermissionsReadOnlyReviewer is the PermissionsOverride value stamped on
// every reviewer AgentRunRuntime by BuildReviewGroupExecuteRequest. The
// dispatch boundary (RunWithConfigViaService) validates that the resolved
// harness can honour this constraint; harnesses that cannot are rejected with
// ReviewReadOnlyEnforcementError rather than silently running unrestricted.
const PermissionsReadOnlyReviewer = "readonly"

// ReviewReadOnlyEnforcementError is returned when a reviewer dispatch is
// attempted with PermissionsReadOnlyReviewer against a harness that cannot
// enforce read-only tool access. Callers must surface this as a review-error
// class; they must not fall back to unrestricted dispatch.
type ReviewReadOnlyEnforcementError struct {
	Harness string
}

func (e *ReviewReadOnlyEnforcementError) Error() string {
	return fmt.Sprintf(
		"reviewer read-only enforcement: harness %q cannot enforce read-only tool access; dispatch rejected",
		e.Harness,
	)
}

// readOnlyCapableHarnesses enumerates harnesses known to honour
// PermissionsReadOnlyReviewer. All others fail closed.
var readOnlyCapableHarnesses = map[string]bool{
	"claude":  true, // Claude Code permission system enforces tool restrictions.
	"agent":   true, // Forwarded to fizeau; fizeau honours the permissions field.
	"virtual": true, // Content-addressed replay; no mutation possible by construction.
}

// ValidateReadOnlyReviewerDispatch returns ReviewReadOnlyEnforcementError when
// permissions equals PermissionsReadOnlyReviewer and harness is absent from
// readOnlyCapableHarnesses. Returns nil when enforcement is not required or
// the harness is known capable.
func ValidateReadOnlyReviewerDispatch(harness, permissions string) error {
	if permissions != PermissionsReadOnlyReviewer {
		return nil
	}
	if readOnlyCapableHarnesses[harness] {
		return nil
	}
	return &ReviewReadOnlyEnforcementError{Harness: harness}
}
