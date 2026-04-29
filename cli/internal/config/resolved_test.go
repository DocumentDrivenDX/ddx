package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// sealed returns a fully-populated, sealed ResolvedConfig used by accessor
// tests. Bead 1 of SD-024 ships only the type + sentinel; (*Config).Resolve
// arrives in bead 2. Until then, in-package tests construct sealed values
// directly via the unexported sentinel.
func sealedFixture() ResolvedConfig {
	return ResolvedConfig{
		sealed:                  true,
		assignee:                "bot",
		reviewMaxRetries:        7,
		noProgressCooldown:      11 * time.Second,
		maxNoChangesBeforeClose: 4,
		heartbeatInterval:       2 * time.Minute,
		harness:                 "claude",
		model:                   "claude-opus-4-7",
		provider:                "anthropic",
		modelRef:                "anthropic/claude-opus-4-7",
		profile:                 "default",
		minTier:                 "cheap",
		maxTier:                 "smart",
		effort:                  "high",
		permissions:             "elevated",
		timeout:                 30 * time.Second,
		wallClock:               6 * time.Hour,
		contextBudget:           "200k",
		evidenceCaps:            evidence.DefaultCaps(),
		sessionLogDir:           "/tmp/sessions",
		mirrorConfig:            &ExecutionsMirrorConfig{Kind: "fs", Path: "/tmp/mirror"},
		reasoningLevels:         map[string][]string{"smart": {"high"}},
	}
}

// expectSealedPanic asserts that fn panics with a message naming
// LoadAndResolve.
func expectSealedPanic(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("%s: expected panic, got none", label)
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("%s: panic value not a string: %T %v", label, r, r)
		}
		if !strings.Contains(msg, "LoadAndResolve") {
			t.Fatalf("%s: panic message %q does not name LoadAndResolve", label, msg)
		}
	}()
	fn()
}

func TestResolvedConfigZeroValuePanicsOnFirstAccessor(t *testing.T) {
	var rcfg ResolvedConfig
	expectSealedPanic(t, "var rcfg", func() { _ = rcfg.Assignee() })

	zero := ResolvedConfig{}
	expectSealedPanic(t, "ResolvedConfig{}", func() { _ = zero.ReviewMaxRetries() })
}

func TestResolvedConfigZeroValuePanicsOnEveryAccessor(t *testing.T) {
	cases := map[string]func(ResolvedConfig){
		"Assignee":                func(r ResolvedConfig) { _ = r.Assignee() },
		"ReviewMaxRetries":        func(r ResolvedConfig) { _ = r.ReviewMaxRetries() },
		"NoProgressCooldown":      func(r ResolvedConfig) { _ = r.NoProgressCooldown() },
		"MaxNoChangesBeforeClose": func(r ResolvedConfig) { _ = r.MaxNoChangesBeforeClose() },
		"HeartbeatInterval":       func(r ResolvedConfig) { _ = r.HeartbeatInterval() },
		"Harness":                 func(r ResolvedConfig) { _ = r.Harness() },
		"Model":                   func(r ResolvedConfig) { _ = r.Model() },
		"Provider":                func(r ResolvedConfig) { _ = r.Provider() },
		"ModelRef":                func(r ResolvedConfig) { _ = r.ModelRef() },
		"Profile":                 func(r ResolvedConfig) { _ = r.Profile() },
		"MinTier":                 func(r ResolvedConfig) { _ = r.MinTier() },
		"MaxTier":                 func(r ResolvedConfig) { _ = r.MaxTier() },
		"Effort":                  func(r ResolvedConfig) { _ = r.Effort() },
		"Permissions":             func(r ResolvedConfig) { _ = r.Permissions() },
		"Timeout":                 func(r ResolvedConfig) { _ = r.Timeout() },
		"WallClock":               func(r ResolvedConfig) { _ = r.WallClock() },
		"ContextBudget":           func(r ResolvedConfig) { _ = r.ContextBudget() },
		"EvidenceCaps":            func(r ResolvedConfig) { _ = r.EvidenceCaps() },
		"SessionLogDir":           func(r ResolvedConfig) { _ = r.SessionLogDir() },
		"MirrorConfig":            func(r ResolvedConfig) { _ = r.MirrorConfig() },
		"ReasoningLevels":         func(r ResolvedConfig) { _ = r.ReasoningLevels() },
	}
	for name, call := range cases {
		name, call := name, call
		t.Run(name, func(t *testing.T) {
			expectSealedPanic(t, name, func() { call(ResolvedConfig{}) })
		})
	}
}

func TestResolvedConfigAssigneeAccessor(t *testing.T) {
	if got := sealedFixture().Assignee(); got != "bot" {
		t.Fatalf("Assignee = %q, want %q", got, "bot")
	}
	if got := (ResolvedConfig{sealed: true}).Assignee(); got != "" {
		t.Fatalf("zero-after-seal Assignee = %q, want empty", got)
	}
}

func TestResolvedConfigReviewMaxRetriesAccessor(t *testing.T) {
	if got := sealedFixture().ReviewMaxRetries(); got != 7 {
		t.Fatalf("ReviewMaxRetries = %d, want 7", got)
	}
	if got := (ResolvedConfig{sealed: true}).ReviewMaxRetries(); got != 0 {
		t.Fatalf("zero-after-seal ReviewMaxRetries = %d, want 0", got)
	}
}

func TestResolvedConfigNoProgressCooldownAccessor(t *testing.T) {
	if got := sealedFixture().NoProgressCooldown(); got != 11*time.Second {
		t.Fatalf("NoProgressCooldown = %v, want 11s", got)
	}
	if got := (ResolvedConfig{sealed: true}).NoProgressCooldown(); got != 0 {
		t.Fatalf("zero-after-seal NoProgressCooldown = %v, want 0", got)
	}
}

func TestResolvedConfigMaxNoChangesBeforeCloseAccessor(t *testing.T) {
	if got := sealedFixture().MaxNoChangesBeforeClose(); got != 4 {
		t.Fatalf("MaxNoChangesBeforeClose = %d, want 4", got)
	}
	if got := (ResolvedConfig{sealed: true}).MaxNoChangesBeforeClose(); got != 0 {
		t.Fatalf("zero-after-seal MaxNoChangesBeforeClose = %d, want 0", got)
	}
}

func TestResolvedConfigHeartbeatIntervalAccessor(t *testing.T) {
	if got := sealedFixture().HeartbeatInterval(); got != 2*time.Minute {
		t.Fatalf("HeartbeatInterval = %v, want 2m", got)
	}
	if got := (ResolvedConfig{sealed: true}).HeartbeatInterval(); got != 0 {
		t.Fatalf("zero-after-seal HeartbeatInterval = %v, want 0", got)
	}
}

func TestResolvedConfigHarnessAccessor(t *testing.T) {
	if got := sealedFixture().Harness(); got != "claude" {
		t.Fatalf("Harness = %q, want claude", got)
	}
	if got := (ResolvedConfig{sealed: true}).Harness(); got != "" {
		t.Fatalf("zero-after-seal Harness = %q, want empty", got)
	}
}

func TestResolvedConfigModelAccessor(t *testing.T) {
	if got := sealedFixture().Model(); got != "claude-opus-4-7" {
		t.Fatalf("Model = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Model(); got != "" {
		t.Fatalf("zero-after-seal Model = %q, want empty", got)
	}
}

func TestResolvedConfigProviderAccessor(t *testing.T) {
	if got := sealedFixture().Provider(); got != "anthropic" {
		t.Fatalf("Provider = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Provider(); got != "" {
		t.Fatalf("zero-after-seal Provider = %q, want empty", got)
	}
}

func TestResolvedConfigModelRefAccessor(t *testing.T) {
	if got := sealedFixture().ModelRef(); got != "anthropic/claude-opus-4-7" {
		t.Fatalf("ModelRef = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).ModelRef(); got != "" {
		t.Fatalf("zero-after-seal ModelRef = %q, want empty", got)
	}
}

func TestResolvedConfigProfileAccessor(t *testing.T) {
	if got := sealedFixture().Profile(); got != "default" {
		t.Fatalf("Profile = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Profile(); got != "" {
		t.Fatalf("zero-after-seal Profile = %q, want empty", got)
	}
}

func TestResolvedConfigMinTierAccessor(t *testing.T) {
	if got := sealedFixture().MinTier(); got != "cheap" {
		t.Fatalf("MinTier = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).MinTier(); got != "" {
		t.Fatalf("zero-after-seal MinTier = %q, want empty", got)
	}
}

func TestResolvedConfigMaxTierAccessor(t *testing.T) {
	if got := sealedFixture().MaxTier(); got != "smart" {
		t.Fatalf("MaxTier = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).MaxTier(); got != "" {
		t.Fatalf("zero-after-seal MaxTier = %q, want empty", got)
	}
}

func TestResolvedConfigEffortAccessor(t *testing.T) {
	if got := sealedFixture().Effort(); got != "high" {
		t.Fatalf("Effort = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Effort(); got != "" {
		t.Fatalf("zero-after-seal Effort = %q, want empty", got)
	}
}

func TestResolvedConfigPermissionsAccessor(t *testing.T) {
	if got := sealedFixture().Permissions(); got != "elevated" {
		t.Fatalf("Permissions = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Permissions(); got != "" {
		t.Fatalf("zero-after-seal Permissions = %q, want empty", got)
	}
}

func TestResolvedConfigTimeoutAccessor(t *testing.T) {
	if got := sealedFixture().Timeout(); got != 30*time.Second {
		t.Fatalf("Timeout = %v", got)
	}
	if got := (ResolvedConfig{sealed: true}).Timeout(); got != 0 {
		t.Fatalf("zero-after-seal Timeout = %v, want 0", got)
	}
}

func TestResolvedConfigWallClockAccessor(t *testing.T) {
	if got := sealedFixture().WallClock(); got != 6*time.Hour {
		t.Fatalf("WallClock = %v", got)
	}
	if got := (ResolvedConfig{sealed: true}).WallClock(); got != 0 {
		t.Fatalf("zero-after-seal WallClock = %v, want 0", got)
	}
}

func TestResolvedConfigContextBudgetAccessor(t *testing.T) {
	if got := sealedFixture().ContextBudget(); got != "200k" {
		t.Fatalf("ContextBudget = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).ContextBudget(); got != "" {
		t.Fatalf("zero-after-seal ContextBudget = %q, want empty", got)
	}
}

func TestResolvedConfigEvidenceCapsAccessor(t *testing.T) {
	want := evidence.DefaultCaps()
	if got := sealedFixture().EvidenceCaps(); got != want {
		t.Fatalf("EvidenceCaps = %+v, want %+v", got, want)
	}
	got := (ResolvedConfig{sealed: true}).EvidenceCaps()
	if got != (evidence.Caps{}) {
		t.Fatalf("zero-after-seal EvidenceCaps = %+v, want zero", got)
	}
}

func TestResolvedConfigSessionLogDirAccessor(t *testing.T) {
	if got := sealedFixture().SessionLogDir(); got != "/tmp/sessions" {
		t.Fatalf("SessionLogDir = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).SessionLogDir(); got != "" {
		t.Fatalf("zero-after-seal SessionLogDir = %q, want empty", got)
	}
}

func TestResolvedConfigMirrorConfigAccessor(t *testing.T) {
	got := sealedFixture().MirrorConfig()
	if got == nil || got.Kind != "fs" || got.Path != "/tmp/mirror" {
		t.Fatalf("MirrorConfig = %+v", got)
	}
	if (ResolvedConfig{sealed: true}).MirrorConfig() != nil {
		t.Fatalf("zero-after-seal MirrorConfig should be nil")
	}
}

func TestResolveNilCfg(t *testing.T) {
	timeout := 9 * time.Second
	overrides := CLIOverrides{
		Harness:  "claude",
		Profile:  "fast",
		Assignee: "bot",
		Timeout:  &timeout,
	}

	var rcfg ResolvedConfig
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Resolve(nil, overrides) panicked: %v", r)
			}
		}()
		rcfg = (*NewConfig)(nil).Resolve(overrides)
	}()

	if got := rcfg.Harness(); got != "claude" {
		t.Fatalf("Harness = %q, want claude", got)
	}
	if got := rcfg.Assignee(); got != "bot" {
		t.Fatalf("Assignee = %q, want bot", got)
	}
	if got := rcfg.Profile(); got != "fast" {
		t.Fatalf("Profile = %q, want fast", got)
	}
	if got := rcfg.Timeout(); got != timeout {
		t.Fatalf("Timeout = %v, want %v", got, timeout)
	}
	// Defaults-resolved review retries is 3.
	if got := rcfg.ReviewMaxRetries(); got != 3 {
		t.Fatalf("ReviewMaxRetries = %d, want 3", got)
	}
	if rcfg.EvidenceCaps() != evidence.DefaultCaps() {
		t.Fatalf("EvidenceCaps = %+v, want default", rcfg.EvidenceCaps())
	}
}

func TestResolveDeepCopy(t *testing.T) {
	mirrorAsync := true
	cfg := &NewConfig{
		Agent: &AgentConfig{
			Harness: "claude",
			Models: map[string]string{
				"smart": "claude-opus",
			},
			ReasoningLevels: map[string][]string{
				"smart": {"high", "medium"},
			},
			Routing: &RoutingConfig{ProfilePriority: []string{"default"}},
		},
		Executions: &ExecutionsConfig{
			Mirror: &ExecutionsMirrorConfig{
				Kind:    "fs",
				Path:    "/tmp/mirror",
				Include: []string{"prompt.md"},
				Async:   &mirrorAsync,
			},
		},
	}

	rcfg := cfg.Resolve(CLIOverrides{Profile: "default"})

	// Mutate exposed maps from the resolved value.
	levels := rcfg.ReasoningLevels()
	levels["smart"][0] = "MUTATED"
	levels["new-key"] = []string{"x"}

	// Source cfg must be untouched.
	if got := cfg.Agent.ReasoningLevels["smart"][0]; got != "high" {
		t.Fatalf("source ReasoningLevels mutated: %q", got)
	}
	if _, ok := cfg.Agent.ReasoningLevels["new-key"]; ok {
		t.Fatalf("source ReasoningLevels gained new-key")
	}
	// Mutating the source after Resolve must not leak into resolved view.
	cfg.Agent.ReasoningLevels["smart"][0] = "SOURCE-MUTATED"
	cfg.Executions.Mirror.Kind = "SOURCE-MUTATED"

	freshLevels := rcfg.ReasoningLevels()
	if freshLevels["smart"][0] != "high" {
		t.Fatalf("post-source-mutation levels = %v", freshLevels)
	}
	if rcfg.MirrorConfig().Kind != "fs" {
		t.Fatalf("post-source-mutation mirror kind = %q", rcfg.MirrorConfig().Kind)
	}
}

func TestLoadAndResolveSuccess(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	yaml := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
agent:
  harness: claude
  permissions: unrestricted
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rcfg, err := LoadAndResolve(tempDir, CLIOverrides{Profile: "fast"})
	if err != nil {
		t.Fatalf("LoadAndResolve: %v", err)
	}
	if got := rcfg.Harness(); got != "claude" {
		t.Fatalf("Harness = %q, want claude (from .ddx/config.yaml)", got)
	}
	if got := rcfg.Permissions(); got != "unrestricted" {
		t.Fatalf("Permissions = %q, want unrestricted", got)
	}
	if got := rcfg.Profile(); got != "fast" {
		t.Fatalf("Profile = %q, want fast (override)", got)
	}
}

func TestLoadAndResolveOverridesWinOverConfig(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	yaml := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
agent:
  harness: claude
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rcfg, err := LoadAndResolve(tempDir, CLIOverrides{Harness: "codex"})
	if err != nil {
		t.Fatalf("LoadAndResolve: %v", err)
	}
	if got := rcfg.Harness(); got != "codex" {
		t.Fatalf("Harness = %q, want codex (override)", got)
	}
}

func TestLoadAndResolveLoadError(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Invalid YAML triggers a load error.
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte("::not valid yaml::\n  - [\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	timeout := 12 * time.Second
	overrides := CLIOverrides{
		Harness:  "claude",
		Profile:  "fast",
		Assignee: "bot",
		Timeout:  &timeout,
	}
	rcfg, err := LoadAndResolve(tempDir, overrides)
	if err == nil {
		t.Fatalf("LoadAndResolve: expected error from invalid yaml, got nil")
	}
	// Returned ResolvedConfig must be sealed and usable, populated from
	// defaults + overrides.
	if got := rcfg.Harness(); got != "claude" {
		t.Fatalf("Harness = %q, want claude (override)", got)
	}
	if got := rcfg.Profile(); got != "fast" {
		t.Fatalf("Profile = %q, want fast (override)", got)
	}
	if got := rcfg.Assignee(); got != "bot" {
		t.Fatalf("Assignee = %q, want bot (override)", got)
	}
	if got := rcfg.Timeout(); got != timeout {
		t.Fatalf("Timeout = %v, want %v", got, timeout)
	}
	if got := rcfg.ReviewMaxRetries(); got != 3 {
		t.Fatalf("ReviewMaxRetries = %d, want 3 (default)", got)
	}
}

func TestResolvedConfigReasoningLevelsAccessor(t *testing.T) {
	got := sealedFixture().ReasoningLevels()
	if len(got["smart"]) != 1 || got["smart"][0] != "high" {
		t.Fatalf("ReasoningLevels = %v", got)
	}
	got["smart"][0] = "MUTATED"
	got["new-key"] = []string{"x"}
	fresh := sealedFixture().ReasoningLevels()
	if fresh["smart"][0] != "high" {
		t.Fatalf("ReasoningLevels not defensively copied: %v", fresh)
	}
	if (ResolvedConfig{sealed: true}).ReasoningLevels() != nil {
		t.Fatalf("zero-after-seal ReasoningLevels should be nil")
	}
}
