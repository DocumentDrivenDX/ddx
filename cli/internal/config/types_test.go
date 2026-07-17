package config

import (
	"math"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestWorkersConfigResolvers covers SD-024 Stage 1: the worker-facing
// WorkersConfig fields (NoProgressCooldown,
// NoChangesVerificationTimeout, MaxNoChangesBeforeClose,
// HeartbeatInterval) resolve to the documented defaults when unset and
// to overrides when populated. The work is not yet wired to
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

	t.Run("NoChangesVerificationTimeout_default_when_nil_receiver", func(t *testing.T) {
		var w *WorkersConfig
		if got := w.ResolveNoChangesVerificationTimeout(); got != 30*time.Minute {
			t.Errorf("nil receiver = %s, want 30m", got)
		}
	})
	t.Run("NoChangesVerificationTimeout_default_when_empty", func(t *testing.T) {
		w := &WorkersConfig{}
		if got := w.ResolveNoChangesVerificationTimeout(); got != 30*time.Minute {
			t.Errorf("empty = %s, want 30m", got)
		}
	})
	t.Run("NoChangesVerificationTimeout_default_when_unparseable", func(t *testing.T) {
		w := &WorkersConfig{NoChangesVerificationTimeout: "not-a-duration"}
		if got := w.ResolveNoChangesVerificationTimeout(); got != 30*time.Minute {
			t.Errorf("unparseable = %s, want 30m", got)
		}
	})
	t.Run("NoChangesVerificationTimeout_default_when_non_positive", func(t *testing.T) {
		w := &WorkersConfig{NoChangesVerificationTimeout: "0s"}
		if got := w.ResolveNoChangesVerificationTimeout(); got != 30*time.Minute {
			t.Errorf("non-positive = %s, want 30m", got)
		}
	})
	t.Run("NoChangesVerificationTimeout_override", func(t *testing.T) {
		w := &WorkersConfig{NoChangesVerificationTimeout: "45s"}
		if got := w.ResolveNoChangesVerificationTimeout(); got != 45*time.Second {
			t.Errorf("override = %s, want 45s", got)
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

	t.Run("LoadPressureThreshold_default", func(t *testing.T) {
		var w *WorkersConfig
		if got := w.ResolveLoadPressureThreshold(); got != 2.5 {
			t.Errorf("nil receiver = %v, want 2.5", got)
		}
	})
	t.Run("LoadPressureThreshold_non_positive_uses_default", func(t *testing.T) {
		w := &WorkersConfig{LoadPressureThreshold: -1}
		if got := w.ResolveLoadPressureThreshold(); got != 2.5 {
			t.Errorf("non-positive = %v, want 2.5", got)
		}
	})
	t.Run("LoadPressureThreshold_non_finite_uses_default", func(t *testing.T) {
		for name, threshold := range map[string]float64{
			"NaN":               math.NaN(),
			"positive infinity": math.Inf(1),
			"negative infinity": math.Inf(-1),
		} {
			t.Run(name, func(t *testing.T) {
				w := &WorkersConfig{LoadPressureThreshold: threshold}
				if got := w.ResolveLoadPressureThreshold(); got != 2.5 {
					t.Errorf("non-finite = %v, want 2.5", got)
				}
			})
		}
	})
	t.Run("LoadPressureThreshold_override", func(t *testing.T) {
		w := &WorkersConfig{LoadPressureThreshold: 3.25}
		if got := w.ResolveLoadPressureThreshold(); got != 3.25 {
			t.Errorf("override = %v, want 3.25", got)
		}
	})
	t.Run("LoadPressureThreshold_flows_through_resolved_config", func(t *testing.T) {
		rcfg := (&NewConfig{Workers: &WorkersConfig{LoadPressureThreshold: 3.25}}).Resolve(CLIOverrides{})
		if got := rcfg.LoadPressureThreshold(); got != 3.25 {
			t.Errorf("resolved override = %v, want 3.25", got)
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

func TestApplyDefaultsRetainDays(t *testing.T) {
	t.Run("defaults_to_90_when_unset", func(t *testing.T) {
		cfg := &NewConfig{}
		cfg.ApplyDefaults()
		if cfg.Executions == nil || cfg.Executions.RetainDays == nil {
			t.Fatalf("ApplyDefaults() did not initialize executions retain_days")
		}
		if got := *cfg.Executions.RetainDays; got != 90 {
			t.Fatalf("RetainDays = %d, want 90", got)
		}
	})

	t.Run("preserves_explicit_zero", func(t *testing.T) {
		zero := 0
		cfg := &NewConfig{
			Executions: &ExecutionsConfig{RetainDays: &zero},
		}
		cfg.ApplyDefaults()
		if cfg.Executions == nil || cfg.Executions.RetainDays == nil {
			t.Fatalf("ApplyDefaults() cleared explicit zero retain_days")
		}
		if got := *cfg.Executions.RetainDays; got != 0 {
			t.Fatalf("RetainDays = %d, want 0", got)
		}
	})
}

func TestConfig_KnownRepos_Unmarshal(t *testing.T) {
	raw := `version: "1.0"
known_repos:
  upstream:
    path: ../upstream
  federated:
    node_id: node-123
    project_id: proj-456
`

	var cfg NewConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got := cfg.KnownRepos["upstream"]; got.Path != "../upstream" || got.NodeID != "" || got.ProjectID != "" {
		t.Fatalf("upstream = %#v, want local path only", got)
	}
	if got := cfg.KnownRepos["federated"]; got.NodeID != "node-123" || got.ProjectID != "proj-456" || got.Path != "" {
		t.Fatalf("federated = %#v, want node/project only", got)
	}

	roundTripped, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var got NewConfig
	if err := yaml.Unmarshal(roundTripped, &got); err != nil {
		t.Fatalf("round-trip yaml.Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(cfg.KnownRepos, got.KnownRepos) {
		t.Fatalf("KnownRepos round-trip mismatch:\n got: %#v\nwant: %#v", got.KnownRepos, cfg.KnownRepos)
	}
}
