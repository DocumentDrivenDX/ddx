package agent

import (
	"errors"
	"testing"
)

// TestReviewerRuntime_ReadOnlyProfilePlumbed verifies that
// BuildReviewExecuteRequest stamps PermissionsReadOnlyReviewer on the returned
// AgentRunRuntime and that Role is "reviewer".
func TestReviewerRuntime_ReadOnlyProfilePlumbed(t *testing.T) {
	impl := ImplementerRouting{
		Harness:     "claude",
		ActualPower: 5,
	}
	rt := BuildReviewExecuteRequest(impl, "", "")
	if rt.PermissionsOverride != PermissionsReadOnlyReviewer {
		t.Errorf("PermissionsOverride = %q; want %q", rt.PermissionsOverride, PermissionsReadOnlyReviewer)
	}
	if rt.Role != "reviewer" {
		t.Errorf("Role = %q; want reviewer", rt.Role)
	}
	if !rt.ClearRoutingPins {
		t.Errorf("ClearRoutingPins = false; want true")
	}
}

// TestReviewerRuntime_MutationDenied verifies that dispatching a reviewer
// runtime with the read-only profile against a "script" harness is rejected
// before the script can execute, ensuring no worktree mutation is possible.
func TestReviewerRuntime_MutationDenied(t *testing.T) {
	// Build the reviewer runtime exactly as production dispatch does.
	impl := ImplementerRouting{Harness: "claude", ActualPower: 3}
	rt := BuildReviewExecuteRequest(impl, "script", "")

	// Validate as RunWithConfigViaService does before branching.
	err := ValidateReadOnlyReviewerDispatch("script", rt.PermissionsOverride)
	if err == nil {
		t.Fatal("expected error for script harness with read-only reviewer profile; got nil")
	}
	var roErr *ReviewReadOnlyEnforcementError
	if !errors.As(err, &roErr) {
		t.Fatalf("expected *ReviewReadOnlyEnforcementError; got %T: %v", err, err)
	}
	// The dispatch was rejected: no script ran, no mutation occurred.
	if roErr.Harness != "script" {
		t.Errorf("Harness = %q; want script", roErr.Harness)
	}
}

// TestReviewerRuntime_UnsupportedReadOnlyFailsClosed verifies that any harness
// not in readOnlyCapableHarnesses produces a typed ReviewReadOnlyEnforcementError
// rather than a nil or generic error, so callers can classify it as a
// review-error without running unrestricted.
func TestReviewerRuntime_UnsupportedReadOnlyFailsClosed(t *testing.T) {
	cases := []struct {
		harness string
	}{
		{"script"},
		{"unknown_harness"},
		{""},
	}
	for _, tc := range cases {
		err := ValidateReadOnlyReviewerDispatch(tc.harness, PermissionsReadOnlyReviewer)
		if err == nil {
			t.Errorf("harness=%q: expected ReviewReadOnlyEnforcementError; got nil", tc.harness)
			continue
		}
		var roErr *ReviewReadOnlyEnforcementError
		if !errors.As(err, &roErr) {
			t.Errorf("harness=%q: expected *ReviewReadOnlyEnforcementError; got %T: %v", tc.harness, err, err)
			continue
		}
		if roErr.Harness != tc.harness {
			t.Errorf("harness=%q: Harness field = %q; want %q", tc.harness, roErr.Harness, tc.harness)
		}
	}

	// Capable harnesses must not return an error.
	for h := range readOnlyCapableHarnesses {
		if err := ValidateReadOnlyReviewerDispatch(h, PermissionsReadOnlyReviewer); err != nil {
			t.Errorf("harness=%q: expected nil for capable harness; got %v", h, err)
		}
	}

	// Non-readonly permissions must never trigger the check.
	if err := ValidateReadOnlyReviewerDispatch("script", "safe"); err != nil {
		t.Errorf("non-readonly permissions: expected nil; got %v", err)
	}
}
