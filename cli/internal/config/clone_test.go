package config

import "testing"

func TestAgentConfigClone(t *testing.T) {
	if (*AgentConfig)(nil).Clone() != nil {
		t.Fatal("nil AgentConfig.Clone should be nil")
	}

	src := &AgentConfig{
		Harness:         "claude",
		Model:           "claude-opus",
		Models:          map[string]string{"smart": "claude-opus"},
		ReasoningLevels: map[string][]string{"smart": {"high"}},
		Endpoints:       []AgentEndpoint{{Type: "openai", Host: "h"}},
		Routing:         &RoutingConfig{ProfilePriority: []string{"default"}},
		Virtual: &VirtualConfig{
			Normalize: []NormalizePattern{{Pattern: "p", Replace: "r"}},
		},
	}

	dst := src.Clone()
	dst.Models["smart"] = "X"
	dst.Models["new"] = "Y"
	dst.ReasoningLevels["smart"][0] = "X"
	dst.ReasoningLevels["new"] = []string{"Y"}
	dst.Endpoints[0].Host = "X"
	dst.Routing.ProfilePriority[0] = "X"
	dst.Virtual.Normalize[0].Pattern = "X"

	if src.Models["smart"] != "claude-opus" {
		t.Fatalf("source Models mutated: %v", src.Models)
	}
	if _, ok := src.Models["new"]; ok {
		t.Fatal("source Models gained new key")
	}
	if src.ReasoningLevels["smart"][0] != "high" {
		t.Fatalf("source ReasoningLevels mutated: %v", src.ReasoningLevels)
	}
	if _, ok := src.ReasoningLevels["new"]; ok {
		t.Fatal("source ReasoningLevels gained new key")
	}
	if src.Endpoints[0].Host != "h" {
		t.Fatalf("source Endpoints mutated: %v", src.Endpoints)
	}
	if src.Routing.ProfilePriority[0] != "default" {
		t.Fatalf("source Routing mutated: %v", src.Routing.ProfilePriority)
	}
	if src.Virtual.Normalize[0].Pattern != "p" {
		t.Fatalf("source Virtual mutated: %v", src.Virtual.Normalize)
	}
}

func TestRoutingConfigClone(t *testing.T) {
	if (*RoutingConfig)(nil).Clone() != nil {
		t.Fatal("nil RoutingConfig.Clone should be nil")
	}

	src := &RoutingConfig{
		ProfilePriority: []string{"a", "b"},
	}

	dst := src.Clone()
	dst.ProfilePriority[0] = "X"

	if src.ProfilePriority[0] != "a" {
		t.Fatalf("source ProfilePriority mutated: %v", src.ProfilePriority)
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
		PerHarness: map[string]*EvidenceCapsOverride{
			"claude": {MaxPromptBytes: &override},
			"nilval": nil,
		},
	}

	dst := src.Clone()

	// Mutate cloned int pointers and verify source untouched.
	*dst.MaxPromptBytes = 999
	*dst.PerHarness["claude"].MaxPromptBytes = 999
	dst.PerHarness["new"] = &EvidenceCapsOverride{}

	if *src.MaxPromptBytes != 100 {
		t.Fatalf("source MaxPromptBytes mutated: %d", *src.MaxPromptBytes)
	}
	if *src.PerHarness["claude"].MaxPromptBytes != 200 {
		t.Fatalf("source PerHarness[claude] mutated: %d", *src.PerHarness["claude"].MaxPromptBytes)
	}
	if _, ok := src.PerHarness["new"]; ok {
		t.Fatal("source PerHarness gained new key")
	}
	if v, ok := dst.PerHarness["nilval"]; !ok || v != nil {
		t.Fatalf("nil per-harness entry not preserved: %v", v)
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
		MaxCount:    &maxCount,
		DefaultSpec: &WorkerDefaultSpec{Harness: "claude", Profile: "fast"},
	}

	dst := src.Clone()
	*dst.MaxCount = 99
	dst.DefaultSpec.Harness = "X"

	if *src.MaxCount != 4 {
		t.Fatalf("source MaxCount mutated: %d", *src.MaxCount)
	}
	if src.DefaultSpec.Harness != "claude" {
		t.Fatalf("source DefaultSpec.Harness mutated: %q", src.DefaultSpec.Harness)
	}
}
