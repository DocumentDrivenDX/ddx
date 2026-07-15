package agent

// Detailed harness invocation knowledge is deliberately test-only. Fizeau is
// the production source of harness inventory and the sole concrete router.

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
		ReasoningLevels: []string{"low", "medium", "high"},
		ModelFlag:       "-m",
		WorkDirFlag:     "-C",
		EffortFlag:      "-c",
		EffortFormat:    "reasoning.effort=%s",
		Surface:         "codex",
		CostClass:       "medium",
		IsSubscription:  true,
		ExactPinSupport: true,
		TUIQuotaCommand: "exec /status",
	},
	"claude": {
		Name:     "claude",
		Binary:   "claude",
		BaseArgs: []string{"--print", "-p", "--verbose", "--output-format", "stream-json"},
		PermissionArgs: map[string][]string{
			"safe":         {},
			"supervised":   {"--permission-mode", "default"},
			"unrestricted": {"--permission-mode", "bypassPermissions", "--dangerously-skip-permissions"},
		},
		PromptMode:      "arg",
		DefaultModel:    "claude-sonnet-4-6",
		ReasoningLevels: []string{"low", "medium", "high"},
		ModelFlag:       "--model",
		EffortFlag:      "--effort",
		TokenPattern:    `(?i)total tokens[:\s]+([0-9,]+)`,
		Surface:         "claude",
		CostClass:       "medium",
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
		ExactPinSupport: true,
	},
	"opencode": {
		Name:     "opencode",
		Binary:   "opencode",
		BaseArgs: []string{"run", "--format", "json"},
		PermissionArgs: map[string][]string{
			"safe": {}, "supervised": {}, "unrestricted": {},
		},
		PromptMode:      "arg",
		ReasoningLevels: []string{"minimal", "low", "medium", "high", "max"},
		ModelFlag:       "-m",
		WorkDirFlag:     "--dir",
		EffortFlag:      "--variant",
		Surface:         "embedded-openai",
		CostClass:       "medium",
		ExactPinSupport: true,
	},
	"agent": {
		Name: "agent", Binary: "fiz", PromptMode: "arg",
		Surface: "embedded-openai", CostClass: "local", IsLocal: true,
		ExactPinSupport: true,
	},
	"pi": {
		Name: "pi", Binary: "pi", BaseArgs: []string{"--mode", "json", "--print"},
		PromptMode: "arg", ModelFlag: "--model", EffortFlag: "--thinking",
		ReasoningLevels: []string{"low", "medium", "high"}, Surface: "pi",
		CostClass: "medium", ExactPinSupport: true,
	},
	"virtual": {
		Name: "virtual", Binary: "ddx-virtual-agent", PromptMode: "arg",
		DefaultModel: "recorded", Surface: "virtual", CostClass: "local",
		IsLocal: true, TestOnly: true,
	},
	"script": {
		Name: "script", Binary: "ddx-script-agent", PromptMode: "arg",
		Surface: "script", CostClass: "local", IsLocal: true, TestOnly: true,
	},
	"openrouter": {
		Name: "openrouter", Surface: "embedded-openai", CostClass: "medium",
		IsHTTPProvider: true,
	},
	"lmstudio": {
		Name: "lmstudio", Surface: "embedded-openai", CostClass: "local",
		IsHTTPProvider: true,
	},
}

var harnessAliases = map[string]string{"local": "agent"}

func resolveHarnessAlias(name string) string {
	if canonical, ok := harnessAliases[name]; ok {
		return canonical
	}
	return name
}

type harnessRegistry struct {
	LookPath  LookPathFunc
	harnesses map[string]harnessConfig
}

func newHarnessRegistry() *harnessRegistry {
	r := &harnessRegistry{LookPath: DefaultLookPath, harnesses: make(map[string]harnessConfig)}
	for key, value := range builtinHarnessConfigs {
		r.harnesses[key] = value
	}
	return r
}

func (r *harnessRegistry) Get(name string) (harnessConfig, bool) {
	harness, ok := r.harnesses[name]
	return harness, ok
}

func (r *harnessRegistry) Has(name string) bool {
	_, ok := r.harnesses[name]
	return ok
}
