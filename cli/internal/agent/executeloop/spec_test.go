package executeloop_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
)

func TestExecuteLoopSpec_RoundTrip_AllFields(t *testing.T) {
	original := executeloop.ExecuteLoopSpec{
		ProjectRoot:        "/projects/myapp",
		Harness:            "claude-code",
		Model:              "claude-opus-4-7",
		Profile:            "default",
		Provider:           "anthropic",
		Effort:             "high",
		LabelFilter:        "area:agent",
		Mode:               executeloop.ModeWatch,
		IdleInterval:       executeloop.Duration{Duration: 45 * time.Second},
		NoReview:           true,
		ReviewHarness:      "reviewer-harness",
		ReviewModel:        "claude-sonnet-4-6",
		OpaquePassthrough:  true,
		MaxCostUSD:         12.50,
		MaxRecoveryCostUSD: 2.75,
		RequestTimeout:     executeloop.Duration{Duration: 2 * time.Minute},
		RateLimitMaxWait:   executeloop.Duration{Duration: 90 * time.Second},
		MinPower:           2,
		MaxPower:           5,
		FromRev:            "abc123def456",
		SpecVersion:        executeloop.SpecCurrentVersion,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got executeloop.ExecuteLoopSpec
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ProjectRoot != original.ProjectRoot {
		t.Errorf("ProjectRoot: got %q, want %q", got.ProjectRoot, original.ProjectRoot)
	}
	if got.Harness != original.Harness {
		t.Errorf("Harness: got %q, want %q", got.Harness, original.Harness)
	}
	if got.Model != original.Model {
		t.Errorf("Model: got %q, want %q", got.Model, original.Model)
	}
	if got.Profile != original.Profile {
		t.Errorf("Profile: got %q, want %q", got.Profile, original.Profile)
	}
	if got.Provider != original.Provider {
		t.Errorf("Provider: got %q, want %q", got.Provider, original.Provider)
	}
	if got.Effort != original.Effort {
		t.Errorf("Effort: got %q, want %q", got.Effort, original.Effort)
	}
	if got.LabelFilter != original.LabelFilter {
		t.Errorf("LabelFilter: got %q, want %q", got.LabelFilter, original.LabelFilter)
	}
	if got.Mode != original.Mode {
		t.Errorf("Mode: got %q, want %q", got.Mode, original.Mode)
	}
	if got.IdleInterval != original.IdleInterval {
		t.Errorf("IdleInterval: got %v, want %v", got.IdleInterval, original.IdleInterval)
	}
	if got.NoReview != original.NoReview {
		t.Errorf("NoReview: got %v, want %v", got.NoReview, original.NoReview)
	}
	if got.ReviewHarness != original.ReviewHarness {
		t.Errorf("ReviewHarness: got %q, want %q", got.ReviewHarness, original.ReviewHarness)
	}
	if got.ReviewModel != original.ReviewModel {
		t.Errorf("ReviewModel: got %q, want %q", got.ReviewModel, original.ReviewModel)
	}
	if got.OpaquePassthrough != original.OpaquePassthrough {
		t.Errorf("OpaquePassthrough: got %v, want %v", got.OpaquePassthrough, original.OpaquePassthrough)
	}
	if got.MaxCostUSD != original.MaxCostUSD {
		t.Errorf("MaxCostUSD: got %v, want %v", got.MaxCostUSD, original.MaxCostUSD)
	}
	if got.MaxRecoveryCostUSD != original.MaxRecoveryCostUSD {
		t.Errorf("MaxRecoveryCostUSD: got %v, want %v", got.MaxRecoveryCostUSD, original.MaxRecoveryCostUSD)
	}
	if got.RequestTimeout != original.RequestTimeout {
		t.Errorf("RequestTimeout: got %v, want %v", got.RequestTimeout, original.RequestTimeout)
	}
	if got.RateLimitMaxWait != original.RateLimitMaxWait {
		t.Errorf("RateLimitMaxWait: got %v, want %v", got.RateLimitMaxWait, original.RateLimitMaxWait)
	}
	if got.MinPower != original.MinPower {
		t.Errorf("MinPower: got %d, want %d", got.MinPower, original.MinPower)
	}
	if got.MaxPower != original.MaxPower {
		t.Errorf("MaxPower: got %d, want %d", got.MaxPower, original.MaxPower)
	}
	if got.FromRev != original.FromRev {
		t.Errorf("FromRev: got %q, want %q", got.FromRev, original.FromRev)
	}
	if got.SpecVersion != original.SpecVersion {
		t.Errorf("SpecVersion: got %d, want %d", got.SpecVersion, original.SpecVersion)
	}
}

func TestExecuteLoopSpec_RoundTrip_NumericNanoseconds(t *testing.T) {
	// Verify Duration unmarshals numeric nanoseconds produced by older clients.
	raw := `{"idle_interval":30000000000,"request_timeout":120000000000,"rate_limit_max_wait":90000000000}`
	var s executeloop.ExecuteLoopSpec
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal numeric nanoseconds: %v", err)
	}
	if s.IdleInterval.Duration != 30*time.Second {
		t.Errorf("IdleInterval: got %v, want 30s", s.IdleInterval.Duration)
	}
	if s.RequestTimeout.Duration != 2*time.Minute {
		t.Errorf("RequestTimeout: got %v, want 2m0s", s.RequestTimeout.Duration)
	}
	if s.RateLimitMaxWait.Duration != 90*time.Second {
		t.Errorf("RateLimitMaxWait: got %v, want 90s", s.RateLimitMaxWait.Duration)
	}
}

func TestExecuteLoopSpec_ApplyDefaults(t *testing.T) {
	t.Run("zero value gets drain mode and spec version", func(t *testing.T) {
		var s executeloop.ExecuteLoopSpec
		s.ApplyDefaults()
		if s.Mode != executeloop.ModeDrain {
			t.Errorf("Mode: got %q, want %q", s.Mode, executeloop.ModeDrain)
		}
		if s.SpecVersion != executeloop.SpecCurrentVersion {
			t.Errorf("SpecVersion: got %d, want %d", s.SpecVersion, executeloop.SpecCurrentVersion)
		}
		if s.MaxRecoveryCostUSD != 2.0 {
			t.Errorf("MaxRecoveryCostUSD: got %v, want 2.0", s.MaxRecoveryCostUSD)
		}
		if s.IdleInterval.Duration != 0 {
			t.Errorf("IdleInterval should be zero for drain mode, got %v", s.IdleInterval.Duration)
		}
	})

	t.Run("watch mode gets idle interval default", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{Mode: executeloop.ModeWatch}
		s.ApplyDefaults()
		if s.IdleInterval.Duration != 30*time.Second {
			t.Errorf("IdleInterval: got %v, want 30s", s.IdleInterval.Duration)
		}
	})

	t.Run("watch mode preserves explicit idle interval", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{
			Mode:         executeloop.ModeWatch,
			IdleInterval: executeloop.Duration{Duration: 60 * time.Second},
		}
		s.ApplyDefaults()
		if s.IdleInterval.Duration != 60*time.Second {
			t.Errorf("IdleInterval: got %v, want 60s", s.IdleInterval.Duration)
		}
	})

	t.Run("once mode does not set idle interval", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{Mode: executeloop.ModeOnce}
		s.ApplyDefaults()
		if s.IdleInterval.Duration != 0 {
			t.Errorf("IdleInterval should be zero for once mode, got %v", s.IdleInterval.Duration)
		}
	})

	t.Run("existing spec version preserved", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{SpecVersion: executeloop.SpecCurrentVersion}
		s.ApplyDefaults()
		if s.SpecVersion != executeloop.SpecCurrentVersion {
			t.Errorf("SpecVersion: got %d, want %d", s.SpecVersion, executeloop.SpecCurrentVersion)
		}
	})
}

func TestExecuteLoopSpec_Validate(t *testing.T) {
	t.Run("valid drain spec", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{Mode: executeloop.ModeDrain, SpecVersion: executeloop.SpecCurrentVersion}
		if err := s.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("unknown mode rejected", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{Mode: "continuous"}
		if err := s.Validate(); err == nil {
			t.Error("expected error for unknown mode")
		}
	})

	t.Run("idle interval on non-watch rejected", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{
			Mode:         executeloop.ModeDrain,
			IdleInterval: executeloop.Duration{Duration: 10 * time.Second},
		}
		if err := s.Validate(); err == nil {
			t.Error("expected error for idle_interval on drain mode")
		}
	})

	t.Run("unknown spec version rejected", func(t *testing.T) {
		s := executeloop.ExecuteLoopSpec{Mode: executeloop.ModeDrain, SpecVersion: 99}
		if err := s.Validate(); err == nil {
			t.Error("expected error for unknown spec_version")
		}
	})
}

func TestExecuteLoopSpec_NoPollInterval(t *testing.T) {
	// Confirm no PollInterval field survives in marshaled JSON.
	s := executeloop.ExecuteLoopSpec{Mode: executeloop.ModeDrain}
	s.ApplyDefaults()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(data, "poll_interval") {
		t.Errorf("marshaled JSON must not contain poll_interval, got: %s", data)
	}
}

func contains(data []byte, substr string) bool {
	return len(data) > 0 && string(data) != "" && jsonContains(data, substr)
}

func jsonContains(data []byte, substr string) bool {
	s := string(data)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
