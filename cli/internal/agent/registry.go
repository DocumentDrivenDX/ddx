package agent

import "os/exec"

// PreferenceOrder defines the default harness preference when multiple are available.
var PreferenceOrder = []string{"codex", "claude", "gemini", "opencode", "forge", "pi"}

// builtinHarnesses defines known harnesses and how to invoke them.
var builtinHarnesses = map[string]Harness{
	"codex": {
		Name:     "codex",
		Binary:   "codex",
		BaseArgs: []string{"exec", "--ephemeral", "--json"},
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
	},
	"claude": {
		Name:     "claude",
		Binary:   "claude",
		BaseArgs: []string{"--no-session-persistence", "--print", "-p", "--output-format", "json"},
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
	},
	"gemini": {
		Name:            "gemini",
		Binary:          "gemini",
		BaseArgs:        []string{},
		PromptMode:      "stdin",
		ModelFlag:       "-m",
		ReasoningLevels: []string{"low", "medium", "high"},
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
	},
	"forge": {
		Name:         "forge",
		Binary:       "ddx", // embedded — runs in-process via forge.Run(), not as a subprocess
		PromptMode:   "arg",
		DefaultModel: "",    // uses forge config or provider default
	},
	"pi": {
		Name:            "pi",
		Binary:          "pi",
		BaseArgs:        []string{"--mode", "json", "--print"},
		PromptMode:      "arg",
		ModelFlag:       "--model",
		EffortFlag:      "--thinking",
		ReasoningLevels: []string{"low", "medium", "high"},
	},
	"virtual": {
		Name:         "virtual",
		Binary:       "ddx-virtual-agent", // sentinel — never actually exec'd
		PromptMode:   "arg",
		DefaultModel: "recorded",
	},
}

// Registry manages known harnesses.
type Registry struct {
	harnesses map[string]Harness
}

// NewRegistry creates a registry with builtin harnesses.
func NewRegistry() *Registry {
	r := &Registry{harnesses: make(map[string]Harness)}
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
	for _, name := range r.Names() {
		h := r.harnesses[name]
		status := HarnessStatus{
			Name:   name,
			Binary: h.Binary,
		}
		// Embedded harnesses are always available — no binary lookup needed.
		if name == "virtual" || name == "forge" {
			status.Available = true
			status.Path = "(embedded)"
		} else if path, err := exec.LookPath(h.Binary); err != nil {
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
