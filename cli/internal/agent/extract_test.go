package agent

import (
	"testing"
)

func TestExtractOutputCodex(t *testing.T) {
	raw := `{"type":"output","item":{"type":"system_message","text":"System setup"}}
{"type":"output","item":{"type":"agent_message","text":"The answer is 42."}}
{"type":"output","item":{"type":"user_message","text":"Thanks!"}}`

	got := ExtractOutput("codex", raw)
	want := "The answer is 42."
	if got != want {
		t.Errorf("ExtractOutput(codex) = %q, want %q", got, want)
	}
}

func TestExtractOutputClaude(t *testing.T) {
	raw := `{"result":"Here is the solution.","usage":{"input_tokens":10,"output_tokens":5},"total_cost_usd":0.001}`

	got := ExtractOutput("claude", raw)
	want := "Here is the solution."
	if got != want {
		t.Errorf("ExtractOutput(claude) = %q, want %q", got, want)
	}
}

func TestExtractOutputForge(t *testing.T) {
	raw := "Plain text output from forge"

	got := ExtractOutput("forge", raw)
	want := "Plain text output from forge"
	if got != want {
		t.Errorf("ExtractOutput(forge) = %q, want %q", got, want)
	}
}

func TestExtractOutputMalformed(t *testing.T) {
	raw := "garbage {{{ not valid json or anything"

	got := ExtractOutput("codex", raw)
	if got != raw {
		t.Errorf("ExtractOutput(codex, malformed) = %q, want raw output %q", got, raw)
	}

	got2 := ExtractOutput("claude", raw)
	if got2 != raw {
		t.Errorf("ExtractOutput(claude, malformed) = %q, want raw output %q", got2, raw)
	}

	got3 := ExtractOutput("unknown-harness", raw)
	if got3 != raw {
		t.Errorf("ExtractOutput(unknown, malformed) = %q, want raw output %q", got3, raw)
	}
}
