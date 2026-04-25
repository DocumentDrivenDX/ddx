package config

import (
	"testing"
	"time"
)

func intPtrLoop(v int) *int { return &v }

func TestNewTestConfigForLoop(t *testing.T) {
	opts := TestLoopConfigOpts{
		Assignee:                "alice@example.com",
		ReviewMaxRetries:        7,
		NoProgressCooldown:      9 * time.Hour,
		MaxNoChangesBeforeClose: 5,
		HeartbeatInterval:       17 * time.Second,
		Harness:                 "claude",
		Model:                   "claude-opus-4-7",
		Profile:                 "smart",
		MinTier:                 "smart",
		MaxTier:                 "smart",
		EvidenceCaps: EvidenceCapsConfig{
			MaxPromptBytes:       intPtrLoop(123456),
			MaxInlinedFileBytes:  intPtrLoop(2048),
			MaxDiffBytes:         intPtrLoop(4096),
			MaxGoverningDocBytes: intPtrLoop(8192),
		},
	}

	cfg := NewTestConfigForLoop(opts)
	if cfg == nil {
		t.Fatal("NewTestConfigForLoop returned nil")
	}

	rcfg := cfg.Resolve(TestLoopOverrides(opts))

	if got := rcfg.Assignee(); got != opts.Assignee {
		t.Errorf("Assignee: got %q, want %q", got, opts.Assignee)
	}
	if got := rcfg.ReviewMaxRetries(); got != opts.ReviewMaxRetries {
		t.Errorf("ReviewMaxRetries: got %d, want %d", got, opts.ReviewMaxRetries)
	}
	if got := rcfg.NoProgressCooldown(); got != opts.NoProgressCooldown {
		t.Errorf("NoProgressCooldown: got %v, want %v", got, opts.NoProgressCooldown)
	}
	if got := rcfg.MaxNoChangesBeforeClose(); got != opts.MaxNoChangesBeforeClose {
		t.Errorf("MaxNoChangesBeforeClose: got %d, want %d", got, opts.MaxNoChangesBeforeClose)
	}
	if got := rcfg.HeartbeatInterval(); got != opts.HeartbeatInterval {
		t.Errorf("HeartbeatInterval: got %v, want %v", got, opts.HeartbeatInterval)
	}
	if got := rcfg.Harness(); got != opts.Harness {
		t.Errorf("Harness: got %q, want %q", got, opts.Harness)
	}
	if got := rcfg.Model(); got != opts.Model {
		t.Errorf("Model: got %q, want %q", got, opts.Model)
	}
	if got := rcfg.Profile(); got != opts.Profile {
		t.Errorf("Profile: got %q, want %q", got, opts.Profile)
	}
	if got := rcfg.MinTier(); got != opts.MinTier {
		t.Errorf("MinTier: got %q, want %q", got, opts.MinTier)
	}
	if got := rcfg.MaxTier(); got != opts.MaxTier {
		t.Errorf("MaxTier: got %q, want %q", got, opts.MaxTier)
	}

	caps := rcfg.EvidenceCaps()
	if caps.MaxPromptBytes != *opts.EvidenceCaps.MaxPromptBytes {
		t.Errorf("EvidenceCaps.MaxPromptBytes: got %d, want %d",
			caps.MaxPromptBytes, *opts.EvidenceCaps.MaxPromptBytes)
	}
	if caps.MaxInlinedFileBytes != *opts.EvidenceCaps.MaxInlinedFileBytes {
		t.Errorf("EvidenceCaps.MaxInlinedFileBytes: got %d, want %d",
			caps.MaxInlinedFileBytes, *opts.EvidenceCaps.MaxInlinedFileBytes)
	}
	if caps.MaxDiffBytes != *opts.EvidenceCaps.MaxDiffBytes {
		t.Errorf("EvidenceCaps.MaxDiffBytes: got %d, want %d",
			caps.MaxDiffBytes, *opts.EvidenceCaps.MaxDiffBytes)
	}
	if caps.MaxGoverningDocBytes != *opts.EvidenceCaps.MaxGoverningDocBytes {
		t.Errorf("EvidenceCaps.MaxGoverningDocBytes: got %d, want %d",
			caps.MaxGoverningDocBytes, *opts.EvidenceCaps.MaxGoverningDocBytes)
	}
}

func TestNewTestConfigForLoop_ZeroOpts(t *testing.T) {
	cfg := NewTestConfigForLoop(TestLoopConfigOpts{})
	if cfg == nil {
		t.Fatal("NewTestConfigForLoop returned nil")
	}
	rcfg := cfg.Resolve(CLIOverrides{})
	// Sealed accessors must not panic on zero opts.
	_ = rcfg.Harness()
	_ = rcfg.Model()
	_ = rcfg.Assignee()
}
