package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestWorkSelfRefreshOptOut covers the --no-self-refresh opt-out: self-refresh
// is on by default in watch mode, off outside it, and disableable via either
// --no-self-refresh (the discoverable opt-out) or --self-refresh=false (legacy).
func TestWorkSelfRefreshOptOut(t *testing.T) {
	mk := func(args ...string) *cobra.Command {
		c := &cobra.Command{Use: "work"}
		c.Flags().Bool("watch", false, "")
		c.Flags().Bool("self-refresh", false, "")
		c.Flags().Bool("no-self-refresh", false, "")
		if err := c.ParseFlags(args); err != nil {
			t.Fatalf("ParseFlags(%v): %v", args, err)
		}
		return c
	}

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"non-watch is off", nil, false},
		{"watch defaults on", []string{"--watch"}, true},
		{"no-self-refresh disables", []string{"--watch", "--no-self-refresh"}, false},
		{"legacy self-refresh=false disables", []string{"--watch", "--self-refresh=false"}, false},
		{"explicit self-refresh still on", []string{"--watch", "--self-refresh"}, true},
		{"no-self-refresh wins over self-refresh", []string{"--watch", "--self-refresh", "--no-self-refresh"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workSelfRefreshEnabled(mk(tc.args...)); got != tc.want {
				t.Fatalf("workSelfRefreshEnabled(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
