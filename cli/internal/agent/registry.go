package agent

import "os/exec"

// PreferenceOrder defines the default harness preference when multiple are available.
var PreferenceOrder = []string{"codex", "claude", "gemini", "opencode", "pi", "cursor"}

// builtinHarnesses defines known harnesses and how to invoke them.
var builtinHarnesses = map[string]Harness{
	"codex": {
		Name:            "codex",
		Binary:          "codex",
		Args:            []string{"--dangerously-bypass-approvals-and-sandbox", "exec", "--ephemeral", "--json"},
		PromptMode:      "arg",
		DefaultModel:    "o3-mini",
		Models:          nil, // models change frequently; rely on provider-side validation
		ReasoningLevels: []string{"low", "medium", "high"},
		ModelFlag:       "-m",
		WorkDirFlag:     "-C",
		EffortFlag:      "-c",
		EffortFormat:    "reasoning.effort=%s",
	},
	"claude": {
		Name:            "claude",
		Binary:          "claude",
		Args:            []string{"--no-session-persistence", "--print", "-p", "--permission-mode", "bypassPermissions", "--dangerously-skip-permissions", "--output-format", "json"},
		PromptMode:      "arg",
		DefaultModel:    "claude-sonnet-4-20250514",
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
		Args:            []string{},
		PromptMode:      "stdin",
		Models:          nil, // models change frequently; rely on provider-side validation
		ReasoningLevels: []string{"low", "medium", "high"},
	},
	"opencode": {
		Name:            "opencode",
		Binary:          "opencode",
		Args:            []string{},
		PromptMode:      "stdin",
		ReasoningLevels: []string{"low", "medium", "high"},
	},
	"pi": {
		Name:            "pi",
		Binary:          "pi",
		Args:            []string{},
		PromptMode:      "stdin",
		ReasoningLevels: []string{"low", "medium", "high"},
	},
	"cursor": {
		Name:            "cursor",
		Binary:          "cursor",
		Args:            []string{},
		PromptMode:      "stdin",
		ReasoningLevels: []string{"low", "medium", "high"},
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
		path, err := exec.LookPath(h.Binary)
		if err != nil {
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
