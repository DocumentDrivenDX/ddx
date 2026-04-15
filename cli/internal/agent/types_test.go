package agent

import (
	"path/filepath"
	"testing"
)

func TestResolveLogDir(t *testing.T) {
	abs := func(p string) string {
		out, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", p, err)
		}
		return out
	}

	tests := []struct {
		name        string
		projectRoot string
		configured  string
		want        string
	}{
		{
			name:        "empty configured uses DefaultLogDir anchored at projectRoot",
			projectRoot: "/tmp/proj",
			configured:  "",
			want:        filepath.Join("/tmp/proj", DefaultLogDir),
		},
		{
			name:        "relative configured is anchored at projectRoot",
			projectRoot: "/tmp/proj",
			configured:  ".ddx/agent-logs",
			want:        filepath.Join("/tmp/proj", ".ddx/agent-logs"),
		},
		{
			name:        "relative configured with subdir is anchored at projectRoot",
			projectRoot: "/tmp/proj",
			configured:  "var/logs",
			want:        filepath.Join("/tmp/proj", "var/logs"),
		},
		{
			name:        "absolute configured is returned unchanged",
			projectRoot: "/tmp/proj",
			configured:  abs("/var/log/ddx"),
			want:        abs("/var/log/ddx"),
		},
		{
			name:        "empty projectRoot with relative configured returns configured unchanged",
			projectRoot: "",
			configured:  ".ddx/agent-logs",
			want:        ".ddx/agent-logs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveLogDir(tc.projectRoot, tc.configured)
			if got != tc.want {
				t.Errorf("ResolveLogDir(%q, %q) = %q; want %q", tc.projectRoot, tc.configured, got, tc.want)
			}
		})
	}
}
