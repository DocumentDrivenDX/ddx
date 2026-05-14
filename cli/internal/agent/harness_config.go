package agent

// harnessConfig is the private compatibility shape used by DDx's legacy
// Runner path for fixture harnesses and subprocess argument tests. Production
// harness inventory comes from github.com/easel/fizeau ListHarnesses.
type harnessConfig struct {
	Name            string
	Binary          string
	Args            []string
	BaseArgs        []string
	PermissionArgs  map[string][]string
	PromptMode      string
	DefaultModel    string
	Models          []string
	ReasoningLevels []string
	ModelFlag       string
	WorkDirFlag     string
	EffortFlag      string
	EffortFormat    string
	TokenPattern    string
	Surface         string
	CostClass       string
	IsLocal         bool
	ExactPinSupport bool
	QuotaCommand    string
	TUIQuotaCommand string
	IsHTTPProvider  bool
	IsSubscription  bool
	TestOnly        bool
}

// builtinHarnessConfigs defines known harnesses and how to invoke them.
var builtinHarnessConfigs = map[string]harnessConfig{
	"codex": {
		Name:     "codex",
		Binary:   "codex",
		BaseArgs: []string{"exec", "--json"},
		PermissionArgs: map[string][]string{
			"safe":         {},
			"supervised":   {},
			"unrestricted": {"--dangerously-bypass-approvals-and-sandbox"},
		},
		PromptMode:      "arg",
		DefaultModel:    "gpt-5.4",
		Models:          nil, // models change frequently; rely on provider-side validation
		ReasoningLevels: []string{"low", "medium", "high"},
		ModelFlag:       "-m",
		WorkDirFlag:     "-C",
		EffortFlag:      "-c",
		EffortFormat:    "reasoning.effort=%s",
		Surface:         "codex",
		CostClass:       "medium",
		IsLocal:         false,
		IsSubscription:  true,
		ExactPinSupport: true,
		TUIQuotaCommand: "exec /status",
	},
	"claude": {
		Name:   "claude",
		Binary: "claude",
		// stream-json emits one JSON event per stdout line while the agent runs,
		// which lets DDx surface real-time progress (tool calls, turn counts,
		// elapsed) instead of blocking until completion. --verbose is required
		// by claude CLI when --output-format=stream-json is combined with --print.
		BaseArgs: []string{"--print", "-p", "--verbose", "--output-format", "stream-json"},
		PermissionArgs: map[string][]string{
			"safe":         {},
			"supervised":   {"--permission-mode", "default"},
			"unrestricted": {"--permission-mode", "bypassPermissions", "--dangerously-skip-permissions"},
		},
		PromptMode:      "arg",
		DefaultModel:    "claude-sonnet-4-6",
		Models:          nil, // models change frequently; rely on provider-side validation
		ReasoningLevels: []string{"low", "medium", "high"},
		ModelFlag:       "--model",
		WorkDirFlag:     "",
		EffortFlag:      "--effort",
		TokenPattern:    `(?i)total tokens[:\s]+([0-9,]+)`,
		Surface:         "claude",
		CostClass:       "medium",
		IsLocal:         false,
		IsSubscription:  true,
		ExactPinSupport: true,
		TUIQuotaCommand: "--bare --print /usage",
	},
	"gemini": {
		Name:            "gemini",
		Binary:          "gemini",
		BaseArgs:        []string{},
		PromptMode:      "stdin",
		ModelFlag:       "-m",
		ReasoningLevels: []string{"low", "medium", "high"},
		Surface:         "gemini",
		CostClass:       "medium",
		IsLocal:         false,
		ExactPinSupport: true,
	},
	"opencode": {
		Name:     "opencode",
		Binary:   "opencode",
		BaseArgs: []string{"run", "--format", "json"},
		PermissionArgs: map[string][]string{
			// opencode run auto-approves all tool permissions;
			// no separate flags needed for any permission level.
			"safe":         {},
			"supervised":   {},
			"unrestricted": {},
		},
		PromptMode:      "arg",
		ReasoningLevels: []string{"minimal", "low", "medium", "high", "max"},
		ModelFlag:       "-m",
		WorkDirFlag:     "--dir",
		EffortFlag:      "--variant",
		Surface:         "embedded-openai",
		CostClass:       "medium",
		IsLocal:         false,
		ExactPinSupport: true,
	},
	"agent": {
		Name:            "agent",
		Binary:          "fiz", // embedded — runs in-process via the Fizeau library, not as a subprocess
		PromptMode:      "arg",
		DefaultModel:    "", // uses agent config or provider default
		Surface:         "embedded-openai",
		CostClass:       "local",
		IsLocal:         true,
		ExactPinSupport: true,
	},
	"pi": {
		Name:            "pi",
		Binary:          "pi",
		BaseArgs:        []string{"--mode", "json", "--print"},
		PromptMode:      "arg",
		ModelFlag:       "--model",
		EffortFlag:      "--thinking",
		ReasoningLevels: []string{"low", "medium", "high"},
		Surface:         "pi",
		CostClass:       "medium",
		IsLocal:         false,
		ExactPinSupport: true,
	},
	"virtual": {
		Name:         "virtual",
		Binary:       "ddx-virtual-agent", // sentinel — never actually exec'd
		PromptMode:   "arg",
		DefaultModel: "recorded",
		Surface:      "virtual",
		CostClass:    "local",
		IsLocal:      true,
		TestOnly:     true, // test-only replay harness; never selected by production powerClass routing
	},
	"script": {
		Name:       "script",
		Binary:     "ddx-script-agent", // sentinel — never actually exec'd
		PromptMode: "arg",
		Surface:    "script",
		CostClass:  "local",
		IsLocal:    true,
		TestOnly:   true, // test-only directive interpreter; never selected by production powerClass routing
	},
	"openrouter": {
		Name:           "openrouter",
		Binary:         "",
		Surface:        "embedded-openai",
		CostClass:      "medium",
		IsHTTPProvider: true,
	},
	"lmstudio": {
		Name:           "lmstudio",
		Binary:         "",
		Surface:        "embedded-openai",
		CostClass:      "local",
		IsHTTPProvider: true,
	},
}

// harnessAliases maps convenience names to canonical harness names.
// "local" always routes to the embedded Fizeau harness; it must never
// fall through to a cloud harness like claude or codex.
var harnessAliases = map[string]string{
	"local": "agent",
}

// resolveHarnessAlias returns the canonical harness name for an alias,
// or the input unchanged if it is not an alias.
func resolveHarnessAlias(name string) string {
	if canonical, ok := harnessAliases[name]; ok {
		return canonical
	}
	return name
}

// harnessRegistry manages known harnesses.
type harnessRegistry struct {
	LookPath  LookPathFunc
	harnesses map[string]harnessConfig
}

// newHarnessRegistry creates a registry with builtin harnesses.
func newHarnessRegistry() *harnessRegistry {
	r := &harnessRegistry{
		LookPath:  DefaultLookPath,
		harnesses: make(map[string]harnessConfig),
	}
	for k, v := range builtinHarnessConfigs {
		r.harnesses[k] = v
	}
	return r
}

// Get returns a harness by name.
func (r *harnessRegistry) Get(name string) (harnessConfig, bool) {
	h, ok := r.harnesses[name]
	return h, ok
}

// Has returns true if the harness is registered.
func (r *harnessRegistry) Has(name string) bool {
	_, ok := r.harnesses[name]
	return ok
}
