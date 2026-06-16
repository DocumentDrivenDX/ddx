package agent

import "testing"

func TestIsLockContentionError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Unable to create '.git/index.lock': File exists.", true},
		{"fatal: another git process seems to be running in this repository", true},
		{".git/index.lock: file exists", true},
		{"fatal: update_ref failed for ref 'refs/heads/main': cannot lock ref 'refs/heads/main': is at 1111111 but expected 2222222", true},
		{"staging-tracker lock held by another process", true},
		{"tracker lock held: try again later", true},
		{"build failed: undefined: foo.Bar", false},
		{"401 unauthorized", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			if got := IsLockContentionError(tc.msg); got != tc.want {
				t.Fatalf("IsLockContentionError(%q)=%v want %v", tc.msg, got, tc.want)
			}
		})
	}
}
