package agent

// PreferenceOrder defines the default harness preference when multiple are available.
var PreferenceOrder = []string{"codex", "claude", "gemini", "opencode", "agent", "pi", "openrouter", "lmstudio"}

// builtinHarnesses defines known harnesses and how to invoke them.
var builtinHarnesses = map[string]Harness{
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
		Binary:          "ddx-agent", // embedded — runs in-process via the agent library, not as a subprocess
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
	},
	"script": {
		Name:       "script",
		Binary:     "ddx-script-agent", // sentinel — never actually exec'd
		PromptMode: "arg",
		Surface:    "script",
		CostClass:  "local",
		IsLocal:    true,
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

// Registry manages known harnesses.
type Registry struct {
	LookPath  LookPathFunc
	harnesses map[string]Harness
}

// NewRegistry creates a registry with builtin harnesses.
func NewRegistry() *Registry {
	r := &Registry{
		LookPath:  DefaultLookPath,
		harnesses: make(map[string]Harness),
	}
	for k, v := range builtinHarnesses {
		r.harnesses[k] = v
	}
	return r
}

// Get returns a harness by name.
func (r *Registry) Get(name string) (Harness, bool) {
	h, ok := r.harnesses[name]
	return h, ok
}

// Has returns true if the harness is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.harnesses[name]
	return ok
}

// Names returns all registered harness names in preference order.
func (r *Registry) Names() []string {
	var names []string
	// First add preferred harnesses that exist in registry
	for _, name := range PreferenceOrder {
		if _, ok := r.harnesses[name]; ok {
			names = append(names, name)
		}
	}
	// Then add any extras not in preference list
	for name := range r.harnesses {
		found := false
		for _, pref := range PreferenceOrder {
			if name == pref {
				found = true
				break
			}
		}
		if !found {
			names = append(names, name)
		}
	}
	return names
}

// Discover checks which harnesses are available on the system.
func (r *Registry) Discover() []HarnessStatus {
	var statuses []HarnessStatus
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = DefaultLookPath
	}
	for _, name := range r.Names() {
		h := r.harnesses[name]
		status := HarnessStatus{
			Name:   name,
			Binary: h.Binary,
		}
		// Embedded harnesses are always available — no binary lookup needed.
		if name == "virtual" || name == "agent" || name == "script" {
			status.Available = true
			status.Path = "(embedded)"
		} else if h.IsHTTPProvider {
			// HTTP-only providers: availability determined by probe, not binary.
			status.Available = true
			status.Path = "(http)"
		} else if path, err := lookPath(h.Binary); err != nil {
			status.Available = false
			status.Error = "binary not found"
		} else {
			status.Available = true
			status.Path = path
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// FirstAvailable returns the first available harness in preference order.
func (r *Registry) FirstAvailable() (string, bool) {
	for _, s := range r.Discover() {
		if s.Available {
			return s.Name, true
		}
	}
	return "", false
}
