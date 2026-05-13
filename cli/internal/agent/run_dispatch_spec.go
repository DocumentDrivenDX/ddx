package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// AgentRunDispatchSpec is the transport-neutral single-run request shared by
// legacy REST/MCP agent dispatch surfaces. CLI-only orchestration modes are
// represented so those transports can reject them explicitly instead of
// silently ignoring caller intent.
type AgentRunDispatchSpec struct {
	Harness      string   `json:"harness,omitempty"`
	Model        string   `json:"model,omitempty"`
	Profile      string   `json:"profile,omitempty"`
	Effort       string   `json:"effort,omitempty"`
	Permissions  string   `json:"permissions,omitempty"`
	Timeout      string   `json:"timeout,omitempty"`
	Prompt       string   `json:"prompt,omitempty"`
	Text         string   `json:"text,omitempty"`
	PromptFile   string   `json:"prompt_file,omitempty"`
	PromptSource string   `json:"prompt_source,omitempty"`
	Output       string   `json:"output,omitempty"`
	JSON         bool     `json:"json,omitempty"`
	Quorum       string   `json:"quorum,omitempty"`
	Harnesses    string   `json:"harnesses,omitempty"`
	Worktree     string   `json:"worktree,omitempty"`
	Record       bool     `json:"record,omitempty"`
	Compare      bool     `json:"compare,omitempty"`
	Sandbox      bool     `json:"sandbox,omitempty"`
	KeepSandbox  bool     `json:"keep_sandbox,omitempty"`
	PostRun      string   `json:"post_run,omitempty"`
	Arms         []string `json:"arm,omitempty"`
}

type PreparedAgentRunDispatch struct {
	Overrides config.CLIOverrides
	Runtime   AgentRunRuntime
}

var agentRunDispatchCLIFlags = map[string]string{
	"prompt":       "supported",
	"text":         "supported",
	"harness":      "supported",
	"model":        "supported",
	"profile":      "supported",
	"effort":       "supported",
	"timeout":      "supported",
	"permissions":  "supported",
	"project":      "control",
	"json":         "unsupported",
	"output":       "unsupported",
	"quorum":       "unsupported",
	"harnesses":    "unsupported",
	"worktree":     "unsupported",
	"record":       "unsupported",
	"compare":      "unsupported",
	"sandbox":      "unsupported",
	"keep-sandbox": "unsupported",
	"post-run":     "unsupported",
	"arm":          "unsupported",
}

// AgentRunDispatchCLIFlagCategories returns the canonical classification used
// to keep REST/MCP single-run dispatch in sync with the legacy agent run surface.
func AgentRunDispatchCLIFlagCategories() map[string]string {
	return cloneStringMap(agentRunDispatchCLIFlags)
}

// AgentRunDispatchJSONFields names every JSON/MCP argument accepted by the
// single-run dispatch spec. Some fields are accepted only so validation can
// fail with a clear unsupported-mode error.
func AgentRunDispatchJSONFields() map[string]struct{} {
	return map[string]struct{}{
		"harness":       {},
		"model":         {},
		"profile":       {},
		"effort":        {},
		"permissions":   {},
		"timeout":       {},
		"prompt":        {},
		"text":          {},
		"prompt_file":   {},
		"prompt_source": {},
		"output":        {},
		"json":          {},
		"quorum":        {},
		"harnesses":     {},
		"worktree":      {},
		"record":        {},
		"compare":       {},
		"sandbox":       {},
		"keep_sandbox":  {},
		"post_run":      {},
		"arm":           {},
	}
}

func PrepareAgentRunDispatch(workDir string, spec AgentRunDispatchSpec) (PreparedAgentRunDispatch, error) {
	if err := spec.ValidateSingleRunTransport(); err != nil {
		return PreparedAgentRunDispatch{}, err
	}
	timeout, err := spec.timeoutDuration()
	if err != nil {
		return PreparedAgentRunDispatch{}, err
	}
	overrides := config.CLIOverrides{
		Harness:     spec.Harness,
		Model:       spec.Model,
		Profile:     NormalizeRoutingProfile(spec.Profile),
		Effort:      spec.Effort,
		Permissions: spec.Permissions,
	}
	if timeout > 0 {
		overrides.Timeout = &timeout
	}
	runtime := AgentRunRuntime{
		Prompt:       spec.InlinePrompt(),
		PromptFile:   spec.PromptFile,
		PromptSource: spec.resolvedPromptSource(),
		WorkDir:      workDir,
	}
	return PreparedAgentRunDispatch{Overrides: overrides, Runtime: runtime}, nil
}

func (s AgentRunDispatchSpec) ValidateSingleRunTransport() error {
	if unsupported := s.unsupportedSingleRunFields(); len(unsupported) > 0 {
		return fmt.Errorf("unsupported agent run dispatch field(s) for REST/MCP: %s", strings.Join(unsupported, ", "))
	}
	if s.Prompt != "" && s.Text != "" {
		return fmt.Errorf("prompt and text are mutually exclusive")
	}
	if s.InlinePrompt() != "" && s.PromptFile != "" {
		return fmt.Errorf("inline prompt and prompt_file are mutually exclusive")
	}
	if s.InlinePrompt() == "" && s.PromptFile == "" {
		return fmt.Errorf("prompt, text, or prompt_file is required")
	}
	if _, err := s.timeoutDuration(); err != nil {
		return err
	}
	return nil
}

func (s AgentRunDispatchSpec) InlinePrompt() string {
	if s.Text != "" {
		return s.Text
	}
	return s.Prompt
}

func (s AgentRunDispatchSpec) resolvedPromptSource() string {
	if s.PromptSource != "" {
		return s.PromptSource
	}
	if s.PromptFile != "" {
		return s.PromptFile
	}
	if s.Text != "" || s.Prompt != "" {
		return "inline"
	}
	return ""
}

func (s AgentRunDispatchSpec) timeoutDuration() (time.Duration, error) {
	if s.Timeout == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(s.Timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout: %w", err)
	}
	return timeout, nil
}

func (s AgentRunDispatchSpec) unsupportedSingleRunFields() []string {
	var fields []string
	addString := func(name, value string) {
		if value != "" {
			fields = append(fields, name)
		}
	}
	addBool := func(name string, value bool) {
		if value {
			fields = append(fields, name)
		}
	}
	addString("output", s.Output)
	addBool("json", s.JSON)
	addString("quorum", s.Quorum)
	addString("harnesses", s.Harnesses)
	addString("worktree", s.Worktree)
	addBool("record", s.Record)
	addBool("compare", s.Compare)
	addBool("sandbox", s.Sandbox)
	addBool("keep_sandbox", s.KeepSandbox)
	addString("post_run", s.PostRun)
	if len(s.Arms) > 0 {
		fields = append(fields, "arm")
	}
	sort.Strings(fields)
	return fields
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
