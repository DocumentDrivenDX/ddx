//go:build linux

package workerstatus

import "testing"

func TestLooksLikeDDxWorkerExcludesWorkHelperSubcommands(t *testing.T) {
	workers := []string{
		"ddx work",
		"ddx work --once",
		"ddx try ddx-12345678",
	}
	for _, cmdline := range workers {
		if !looksLikeDDxWorker(cmdline) {
			t.Fatalf("%q should be classified as a live worker", cmdline)
		}
	}

	helpers := []string{
		"ddx work analyze",
		"ddx work clear-cooldowns",
		"ddx work focus",
		"ddx work metrics",
		"ddx work plan",
		"ddx work status",
	}
	for _, cmdline := range helpers {
		if looksLikeDDxWorker(cmdline) {
			t.Fatalf("%q should not be classified as a live worker", cmdline)
		}
	}
}
