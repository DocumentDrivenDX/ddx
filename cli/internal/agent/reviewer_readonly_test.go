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
	if rt.ClearRoutingPins {
		t.Error("ClearRoutingPins = true; reviewer must inherit the immutable primary operator envelope")
	}
	if rt.ClearProfile {
		t.Error("ClearProfile = true; reviewer must inherit the operator's public Policy")
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

// TestReviewerPermissionsWithinFizeauVocabulary pins the reviewer permissions
// constraint to Fizeau v0.15's documented vocabulary (fizeau service.go
// SupportedPermissions: subset of {"safe", "supervised", "unrestricted"}).
// A value outside the vocabulary matches no routing candidate, so ResolveRoute
// rejects every route and each pre-land review dies instantly with
// provider_empty (ddx-822fb475: "readonly" silently killed all pre-land
// reviews while implementation dispatches kept succeeding).
func TestReviewerPermissionsWithinFizeauVocabulary(t *testing.T) {
	vocabulary := map[string]bool{
		"safe":         true,
		"supervised":   true,
		"unrestricted": true,
	}
	if !vocabulary[PermissionsReadOnlyReviewer] {
		t.Fatalf("PermissionsReadOnlyReviewer %q is outside Fizeau's permission vocabulary [safe supervised unrestricted]; every reviewer dispatch would fail route resolution", PermissionsReadOnlyReviewer)
	}
	if rt := BuildReviewExecuteRequest(ImplementerRouting{}); rt.PermissionsOverride != PermissionsReadOnlyReviewer {
		t.Fatalf("BuildReviewExecuteRequest PermissionsOverride = %q, want %q", rt.PermissionsOverride, PermissionsReadOnlyReviewer)
	}
}
