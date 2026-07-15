package agent

import (
	"context"
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
	rt := BuildReviewExecuteRequest(impl)
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

func TestReviewerReadOnlyConstraintPassesThroughWithoutHarnessCatalog(t *testing.T) {
	tests := []struct {
		name    string
		harness string
	}{
		{name: "unpinned"},
		{name: "explicit opaque harness", harness: " opaque-reviewer-route "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &passthroughTestService{}
			rcfg := resolvedWithPassthrough(tt.harness, "", "", 0, 0)

			_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
				Prompt:              "review this",
				PermissionsOverride: PermissionsReadOnlyReviewer,
				Role:                "reviewer",
			})
			if err != nil {
				t.Fatalf("executeOnService returned error: %v", err)
			}
			if svc.lastReq.Permissions != PermissionsReadOnlyReviewer {
				t.Errorf("Permissions = %q; want %q", svc.lastReq.Permissions, PermissionsReadOnlyReviewer)
			}
			if svc.lastReq.Harness != tt.harness {
				t.Errorf("Harness = %q; want byte-identical %q", svc.lastReq.Harness, tt.harness)
			}
			if svc.listHarnessesCalled {
				t.Error("review dispatch queried Fizeau harness inventory")
			}
		})
	}
}
