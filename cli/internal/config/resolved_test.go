package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// sealed returns a fully-populated, sealed ResolvedConfig used by accessor
// tests. Bead 1 of SD-024 ships only the type + sentinel; (*Config).Resolve
// arrives in bead 2. Until then, in-package tests construct sealed values
// directly via the unexported sentinel.
func sealedFixture() ResolvedConfig {
	return ResolvedConfig{
		sealed:                       true,
		assignee:                     "bot",
		reviewMaxRetries:             7,
		noProgressCooldown:           11 * time.Second,
		noChangesVerificationTimeout: 13 * time.Second,
		maxNoChangesBeforeClose:      4,
		heartbeatInterval:            2 * time.Minute,
		harness:                      "claude",
		model:                        "claude-opus-4-7",
		provider:                     "anthropic",
		profile:                      "default",
		effort:                       "high",
		permissions:                  "elevated",
		timeout:                      30 * time.Second,
		wallClock:                    6 * time.Hour,
		contextBudget:                "200k",
		evidenceCaps:                 evidence.DefaultCaps(),
		sessionLogDir:                "/tmp/sessions",
		mirrorConfig:                 &ExecutionsMirrorConfig{Kind: "fs", Path: "/tmp/mirror"},
		beadQualityMode:              BeadQualityModeBlock,
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
		"Assignee":                           func(r ResolvedConfig) { _ = r.Assignee() },
		"ReviewMaxRetries":                   func(r ResolvedConfig) { _ = r.ReviewMaxRetries() },
		"NoProgressCooldown":                 func(r ResolvedConfig) { _ = r.NoProgressCooldown() },
		"NoChangesVerificationTimeout":       func(r ResolvedConfig) { _ = r.NoChangesVerificationTimeout() },
		"MaxNoChangesBeforeClose":            func(r ResolvedConfig) { _ = r.MaxNoChangesBeforeClose() },
		"HeartbeatInterval":                  func(r ResolvedConfig) { _ = r.HeartbeatInterval() },
		"Harness":                            func(r ResolvedConfig) { _ = r.Harness() },
		"Model":                              func(r ResolvedConfig) { _ = r.Model() },
		"Provider":                           func(r ResolvedConfig) { _ = r.Provider() },
		"Profile":                            func(r ResolvedConfig) { _ = r.Profile() },
		"Effort":                             func(r ResolvedConfig) { _ = r.Effort() },
		"Permissions":                        func(r ResolvedConfig) { _ = r.Permissions() },
		"Timeout":                            func(r ResolvedConfig) { _ = r.Timeout() },
		"WallClock":                          func(r ResolvedConfig) { _ = r.WallClock() },
		"ContextBudget":                      func(r ResolvedConfig) { _ = r.ContextBudget() },
		"EvidenceCaps":                       func(r ResolvedConfig) { _ = r.EvidenceCaps() },
		"EvidenceCapsForRole":                func(r ResolvedConfig) { _ = r.EvidenceCapsForRole(EvidenceRoleReviewer) },
		"SessionLogDir":                      func(r ResolvedConfig) { _ = r.SessionLogDir() },
		"MirrorConfig":                       func(r ResolvedConfig) { _ = r.MirrorConfig() },
		"BeadQualityLintBlockThresholdScore": func(r ResolvedConfig) { _ = r.BeadQualityLintBlockThresholdScore() },
		"BeadQualityMode":                    func(r ResolvedConfig) { _ = r.BeadQualityMode() },
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

func TestResolvedConfigNoChangesVerificationTimeoutAccessor(t *testing.T) {
	if got := sealedFixture().NoChangesVerificationTimeout(); got != 13*time.Second {
		t.Fatalf("NoChangesVerificationTimeout = %v, want 13s", got)
	}
	if got := (ResolvedConfig{sealed: true}).NoChangesVerificationTimeout(); got != 0 {
		t.Fatalf("zero-after-seal NoChangesVerificationTimeout = %v, want 0", got)
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

func TestResolvedConfigProfileAccessor(t *testing.T) {
	if got := sealedFixture().Profile(); got != "default" {
		t.Fatalf("Profile = %q", got)
	}
	if got := (ResolvedConfig{sealed: true}).Profile(); got != "" {
		t.Fatalf("zero-after-seal Profile = %q, want empty", got)
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

func TestResolvedConfigEvidenceCapsForRoleAccessor(t *testing.T) {
	r := (ResolvedConfig{
		sealed:                  true,
		evidenceCaps:            evidence.Caps{MaxPromptBytes: 1},
		implementerEvidenceCaps: evidence.Caps{MaxPromptBytes: 2},
		reviewerEvidenceCaps:    evidence.Caps{MaxPromptBytes: 3},
		lifecycleEvidenceCaps:   evidence.Caps{MaxPromptBytes: 4},
	})
	for role, want := range map[string]int{
		EvidenceRoleImplementer: 2,
		EvidenceRoleReviewer:    3,
		EvidenceRoleLifecycle:   4,
		"unknown":               1,
	} {
		if got := r.EvidenceCapsForRole(role).MaxPromptBytes; got != want {
			t.Errorf("EvidenceCapsForRole(%q) = %d, want %d", role, got, want)
		}
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

func TestResolveAgentWallClockFromProjectConfig(t *testing.T) {
	wallClock := 90 * time.Minute
	cfg := &NewConfig{
		Agent: &AgentConfig{
			WallClockMS: int(wallClock / time.Millisecond),
		},
	}

	rcfg := cfg.Resolve(CLIOverrides{})
	if got := rcfg.WallClock(); got != wallClock {
		t.Fatalf("WallClock = %v, want %v", got, wallClock)
	}
}

func TestResolveDeepCopy(t *testing.T) {
	mirrorAsync := true
	cfg := &NewConfig{
		Agent: &AgentConfig{
			Routing: &RoutingConfig{},
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

	// Mutating the source after Resolve must not leak into resolved view.
	cfg.Executions.Mirror.Kind = "SOURCE-MUTATED"

	if rcfg.MirrorConfig().Kind != "fs" {
		t.Fatalf("post-source-mutation mirror kind = %q", rcfg.MirrorConfig().Kind)
	}
}

func TestLoadAndResolveSuccess(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
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
  permissions: unrestricted
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rcfg, err := LoadAndResolve(tempDir, CLIOverrides{Profile: "fast"})
	if err != nil {
		t.Fatalf("LoadAndResolve: %v", err)
	}
	if got := rcfg.Harness(); got != "" {
		t.Fatalf("Harness = %q, want empty", got)
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
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	yaml := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
agent: {}
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
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
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

// TestOpaquePassthroughPreservesOnlyExplicitModel preserves the serialized compatibility
// flag while proving only explicit request constraints affect resolution.
func TestOpaquePassthroughPreservesOnlyExplicitModel(t *testing.T) {
	cfg := &NewConfig{Agent: &AgentConfig{}}

	normalRcfg := cfg.Resolve(CLIOverrides{})
	if got := normalRcfg.Harness(); got != "" {
		t.Fatalf("baseline Harness = %q, want empty", got)
	}
	if got := normalRcfg.Model(); got != "" {
		t.Fatalf("baseline Model = %q, want empty", got)
	}

	// With OpaquePassthrough the config values must be suppressed.
	opaqueRcfg := cfg.Resolve(CLIOverrides{OpaquePassthrough: true})
	if got := opaqueRcfg.Model(); got != "" {
		t.Fatalf("opaque Model = %q, want empty (config must not inject)", got)
	}

	// Explicit CLI values must still pass through even under OpaquePassthrough.
	explicitRcfg := cfg.Resolve(CLIOverrides{
		OpaquePassthrough: true,
		Harness:           "agent",
		Model:             "gpt-5",
	})
	if got := explicitRcfg.Harness(); got != "agent" {
		t.Fatalf("explicit opaque Harness = %q, want agent", got)
	}
	if got := explicitRcfg.Model(); got != "gpt-5" {
		t.Fatalf("explicit opaque Model = %q, want gpt-5", got)
	}
}

func TestResolveTracksExplicitRoutePins(t *testing.T) {
	cfg := &NewConfig{
		Version: "1.0",
		Agent:   &AgentConfig{},
	}

	configOnly := cfg.Resolve(CLIOverrides{})
	if got, ok := configOnly.ExplicitModel(); ok || got != "" {
		t.Fatalf("config-only ExplicitModel = (%q, %v), want (empty, false)", got, ok)
	}

	explicit := cfg.Resolve(CLIOverrides{
		Harness:  "codex",
		Provider: "openai",
		Model:    "gpt-5.4-mini",
	})
	if got, ok := explicit.ExplicitHarness(); !ok || got != "codex" {
		t.Fatalf("ExplicitHarness = (%q, %v), want (codex, true)", got, ok)
	}
	if got, ok := explicit.ExplicitProvider(); !ok || got != "openai" {
		t.Fatalf("ExplicitProvider = (%q, %v), want (openai, true)", got, ok)
	}
	if got, ok := explicit.ExplicitModel(); !ok || got != "gpt-5.4-mini" {
		t.Fatalf("ExplicitModel = (%q, %v), want (gpt-5.4-mini, true)", got, ok)
	}
}
