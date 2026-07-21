package agent

import "testing"

// TestImplementerPermissionsHonoursSealedConfig pins the contract that
// ddx-642a9f26 regressed: an explicit agent.permissions must reach the
// implementation dispatch unchanged, while an unset value must fall back to
// unrestricted so the attempt can write and commit in its isolated worktree
// instead of inheriting the native harness read-only toolset.
func TestImplementerPermissionsHonoursSealedConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sealed string
		want   string
	}{
		{"unset falls back to unrestricted", "", "unrestricted"},
		{"blank falls back to unrestricted", "   ", "unrestricted"},
		{"explicit setting is inherited", "sealed-permission", ""},
		{"explicit unrestricted is inherited", "unrestricted", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := implementerPermissions(tc.sealed); got != tc.want {
				t.Fatalf("implementerPermissions(%q) = %q, want %q", tc.sealed, got, tc.want)
			}
		})
	}
}
