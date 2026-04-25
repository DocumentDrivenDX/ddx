package config

import (
	"testing"
	"time"
)

// TestWorkersConfigResolvers covers SD-024 Stage 1: the three new
// WorkersConfig fields (NoProgressCooldown, MaxNoChangesBeforeClose,
// HeartbeatInterval) resolve to the documented defaults when unset and
// to overrides when populated. The execute-loop is not yet wired to
// these resolvers — that lands in beads 6/7.
func TestWorkersConfigResolvers(t *testing.T) {
	t.Run("NoProgressCooldown_default_when_nil_receiver", func(t *testing.T) {
		var w *WorkersConfig
		if got := w.ResolveNoProgressCooldown(); got != 6*time.Hour {
			t.Errorf("nil receiver = %s, want 6h", got)
		}
	})
	t.Run("NoProgressCooldown_default_when_empty", func(t *testing.T) {
		w := &WorkersConfig{}
		if got := w.ResolveNoProgressCooldown(); got != 6*time.Hour {
			t.Errorf("empty = %s, want 6h", got)
		}
	})
	t.Run("NoProgressCooldown_default_when_unparseable", func(t *testing.T) {
		w := &WorkersConfig{NoProgressCooldown: "not-a-duration"}
		if got := w.ResolveNoProgressCooldown(); got != 6*time.Hour {
			t.Errorf("unparseable = %s, want 6h", got)
		}
	})
	t.Run("NoProgressCooldown_default_when_non_positive", func(t *testing.T) {
		w := &WorkersConfig{NoProgressCooldown: "0s"}
		if got := w.ResolveNoProgressCooldown(); got != 6*time.Hour {
			t.Errorf("non-positive = %s, want 6h", got)
		}
	})
	t.Run("NoProgressCooldown_override", func(t *testing.T) {
		w := &WorkersConfig{NoProgressCooldown: "2h30m"}
		if got := w.ResolveNoProgressCooldown(); got != 2*time.Hour+30*time.Minute {
			t.Errorf("override = %s, want 2h30m", got)
		}
	})

	t.Run("MaxNoChangesBeforeClose_default_when_nil_receiver", func(t *testing.T) {
		var w *WorkersConfig
		if got := w.ResolveMaxNoChangesBeforeClose(); got != 3 {
			t.Errorf("nil receiver = %d, want 3", got)
		}
	})
	t.Run("MaxNoChangesBeforeClose_default_when_unset", func(t *testing.T) {
		w := &WorkersConfig{}
		if got := w.ResolveMaxNoChangesBeforeClose(); got != 3 {
			t.Errorf("unset = %d, want 3", got)
		}
	})
	t.Run("MaxNoChangesBeforeClose_default_when_non_positive", func(t *testing.T) {
		zero := 0
		neg := -2
		for _, v := range []*int{&zero, &neg} {
			w := &WorkersConfig{MaxNoChangesBeforeClose: v}
			if got := w.ResolveMaxNoChangesBeforeClose(); got != 3 {
				t.Errorf("non-positive (%d) = %d, want 3", *v, got)
			}
		}
	})
	t.Run("MaxNoChangesBeforeClose_override", func(t *testing.T) {
		five := 5
		w := &WorkersConfig{MaxNoChangesBeforeClose: &five}
		if got := w.ResolveMaxNoChangesBeforeClose(); got != 5 {
			t.Errorf("override = %d, want 5", got)
		}
	})

	t.Run("HeartbeatInterval_default_when_nil_receiver", func(t *testing.T) {
		var w *WorkersConfig
		if got := w.ResolveHeartbeatInterval(); got != 30*time.Second {
			t.Errorf("nil receiver = %s, want 30s", got)
		}
	})
	t.Run("HeartbeatInterval_default_when_empty", func(t *testing.T) {
		w := &WorkersConfig{}
		if got := w.ResolveHeartbeatInterval(); got != 30*time.Second {
			t.Errorf("empty = %s, want 30s", got)
		}
	})
	t.Run("HeartbeatInterval_default_when_unparseable", func(t *testing.T) {
		w := &WorkersConfig{HeartbeatInterval: "garbage"}
		if got := w.ResolveHeartbeatInterval(); got != 30*time.Second {
			t.Errorf("unparseable = %s, want 30s", got)
		}
	})
	t.Run("HeartbeatInterval_default_when_non_positive", func(t *testing.T) {
		w := &WorkersConfig{HeartbeatInterval: "0s"}
		if got := w.ResolveHeartbeatInterval(); got != 30*time.Second {
			t.Errorf("non-positive = %s, want 30s", got)
		}
	})
	t.Run("HeartbeatInterval_override", func(t *testing.T) {
		w := &WorkersConfig{HeartbeatInterval: "15s"}
		if got := w.ResolveHeartbeatInterval(); got != 15*time.Second {
			t.Errorf("override = %s, want 15s", got)
		}
	})
}

// TestEvidenceCapsContextBudget covers SD-024 Stage 3: the new
// EvidenceCapsConfig.ContextBudget field resolves to "" by default and to
// the configured value when set.
func TestEvidenceCapsContextBudget(t *testing.T) {
	t.Run("default_when_nil_receiver", func(t *testing.T) {
		var e *EvidenceCapsConfig
		if got := e.ResolveContextBudget(); got != "" {
			t.Errorf("nil receiver = %q, want empty", got)
		}
	})
	t.Run("default_when_unset", func(t *testing.T) {
		e := &EvidenceCapsConfig{}
		if got := e.ResolveContextBudget(); got != "" {
			t.Errorf("unset = %q, want empty", got)
		}
	})
	t.Run("override", func(t *testing.T) {
		e := &EvidenceCapsConfig{ContextBudget: "minimal"}
		if got := e.ResolveContextBudget(); got != "minimal" {
			t.Errorf("override = %q, want minimal", got)
		}
	})
}
