package config

import (
	"fmt"
	"os"
	"testing"
)

// TestMain prevents config tests from inheriting an operator's global DDx
// configuration. Tests that exercise global layering install their own HOME.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ddx-config-test-home-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create config test home: %v\n", err)
		os.Exit(1)
	}
	previousHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "set config test HOME: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("HOME", previousHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	_ = os.RemoveAll(home)
	os.Exit(code)
}
