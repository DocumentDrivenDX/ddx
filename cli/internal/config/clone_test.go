package config

import "testing"

func TestAgentConfigClone(t *testing.T) {
	if (*AgentConfig)(nil).Clone() != nil {
		t.Fatal("nil AgentConfig.Clone should be nil")
	}

	src := &AgentConfig{
		Routing: &RoutingConfig{},
		Virtual: &VirtualConfig{
			Normalize: []NormalizePattern{{Pattern: "p", Replace: "r"}},
		},
	}

	dst := src.Clone()
	dst.Virtual.Normalize[0].Pattern = "X"

	if dst.Routing == nil {
		t.Fatal("Routing was not cloned")
	}
	if src.Virtual.Normalize[0].Pattern != "p" {
		t.Fatalf("source Virtual mutated: %v", src.Virtual.Normalize)
	}
}

func TestRoutingConfigClone(t *testing.T) {
	if (*RoutingConfig)(nil).Clone() != nil {
		t.Fatal("nil RoutingConfig.Clone should be nil")
	}

	src := &RoutingConfig{}

	dst := src.Clone()
	if dst == nil {
		t.Fatal("RoutingConfig.Clone returned nil")
	}
}

func TestEvidenceCapsConfigClone(t *testing.T) {
	if (*EvidenceCapsConfig)(nil).Clone() != nil {
		t.Fatal("nil EvidenceCapsConfig.Clone should be nil")
	}

	prompt := 100
	override := 200
	src := &EvidenceCapsConfig{
		MaxPromptBytes: &prompt,
		PerRole: map[string]*EvidenceCapsOverride{
			"reviewer": {MaxPromptBytes: &override},
			"nilval":   nil,
		},
	}

	dst := src.Clone()

	// Mutate cloned int pointers and verify source untouched.
	*dst.MaxPromptBytes = 999
	*dst.PerRole["reviewer"].MaxPromptBytes = 999
	dst.PerRole["new"] = &EvidenceCapsOverride{}

	if *src.MaxPromptBytes != 100 {
		t.Fatalf("source MaxPromptBytes mutated: %d", *src.MaxPromptBytes)
	}
	if *src.PerRole["reviewer"].MaxPromptBytes != 200 {
		t.Fatalf("source PerRole[reviewer] mutated: %d", *src.PerRole["reviewer"].MaxPromptBytes)
	}
	if _, ok := src.PerRole["new"]; ok {
		t.Fatal("source PerRole gained new key")
	}
	if v, ok := dst.PerRole["nilval"]; !ok || v != nil {
		t.Fatalf("nil per-role entry not preserved: %v", v)
	}
}

func TestExecutionsMirrorConfigClone(t *testing.T) {
	if (*ExecutionsMirrorConfig)(nil).Clone() != nil {
		t.Fatal("nil ExecutionsMirrorConfig.Clone should be nil")
	}

	async := true
	src := &ExecutionsMirrorConfig{
		Kind:    "fs",
		Path:    "/tmp/m",
		Include: []string{"a.md"},
		Async:   &async,
	}

	dst := src.Clone()
	dst.Include[0] = "X"
	*dst.Async = false

	if src.Include[0] != "a.md" {
		t.Fatalf("source Include mutated: %v", src.Include)
	}
	if !*src.Async {
		t.Fatal("source Async mutated")
	}
}

func TestWorkersConfigClone(t *testing.T) {
	if (*WorkersConfig)(nil).Clone() != nil {
		t.Fatal("nil WorkersConfig.Clone should be nil")
	}

	maxCount := 4
	src := &WorkersConfig{
		MaxCount:              &maxCount,
		DefaultSpec:           &WorkerDefaultSpec{Profile: "fast"},
		LoadPressureThreshold: 3.25,
	}

	dst := src.Clone()
	*dst.MaxCount = 99
	dst.DefaultSpec.Profile = "slow"

	if *src.MaxCount != 4 {
		t.Fatalf("source MaxCount mutated: %d", *src.MaxCount)
	}
	if src.DefaultSpec.Profile != "fast" {
		t.Fatalf("source DefaultSpec.Profile mutated: %q", src.DefaultSpec.Profile)
	}
	if dst.LoadPressureThreshold != src.LoadPressureThreshold {
		t.Fatalf("LoadPressureThreshold not cloned: got %v want %v", dst.LoadPressureThreshold, src.LoadPressureThreshold)
	}
}
