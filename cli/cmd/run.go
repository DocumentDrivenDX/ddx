package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/persona"
	"github.com/spf13/cobra"
)

// newRunCommand creates the top-level "ddx run" command — the layer-1 task
// invocation primitive. One Fizeau Execute call; no queue semantics, no
// routing policy, no ResolveRoute. Power bounds and passthrough constraints
// are forwarded unchanged (CONTRACT-003).
func (f *CommandFactory) newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run",
		Short:        "Invoke a task dispatcher for a prompt (layer-1 Fizeau Execute call)",
		SilenceUsage: true,
		Long: `run dispatches a single task to the upstream agent service via one Fizeau
Execute call. It is the layer-1 primitive over which ddx try and ddx work add
bead lifecycle, queue semantics, and retry policy.

DDx passes all routing knobs (--harness, --provider, --model,
--min-power, --max-power, --profile) directly to the service without validation
or pre-resolution. The service owns route selection within those bounds
(CONTRACT-003 / FEAT-010).

Persona injection: --persona <name> loads the named persona and prepends its
body to the prompt as a system-prompt addendum before dispatch.

Examples:
  ddx run --min-power 10 --prompt task.md
  ddx run --min-power 10 --max-power 20 --prompt task.md
  ddx run --harness codex --min-power 10 --prompt task.md
  ddx run --provider openrouter --model qwen3.6-27b --prompt task.md
  ddx run --persona code-reviewer --prompt review.md
  ddx run --profile smart --prompt task.md
  ddx run --text "explain this function"`,
		RunE: f.runRun,
	}

	cmd.Flags().String("project", "", "Project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("prompt", "", "Path to prompt file")
	cmd.Flags().String("text", "", "Inline prompt text")
	cmd.Flags().String("harness", "", "Harness constraint (passthrough; not validated by ddx run)")
	cmd.Flags().String("model", "", "Model constraint (passthrough; not validated by ddx run)")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; not validated by ddx run)")
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained)")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained)")
	cmd.Flags().String("persona", "", "Persona name; body is prepended to the prompt as a system-prompt addendum")
	cmd.Flags().String("profile", "", "Routing profile: default, cheap, fast, smart")
	cmd.Flags().String("effort", "", "Reasoning effort override (e.g. low, medium, high)")
	cmd.Flags().String("permissions", "", "Permission level: safe, supervised, unrestricted")
	cmd.Flags().String("timeout", "", "Request timeout (e.g. 30s, 5m)")
	cmd.Flags().Bool("json", false, "Output as JSON (alias for --output json-result)")
	cmd.Flags().String("output", "text", "Output format: text (default), json-result, session-jsonl")

	return cmd
}

func (f *CommandFactory) runRun(cmd *cobra.Command, args []string) error {
	promptFile, _ := cmd.Flags().GetString("prompt")
	promptText, _ := cmd.Flags().GetString("text")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")
	personaName, _ := cmd.Flags().GetString("persona")
	profile, _ := cmd.Flags().GetString("profile")
	effort, _ := cmd.Flags().GetString("effort")
	permissions, _ := cmd.Flags().GetString("permissions")
	timeoutStr, _ := cmd.Flags().GetString("timeout")
	asJSON, _ := cmd.Flags().GetBool("json")
	outputFmt, _ := cmd.Flags().GetString("output")
	projectFlag, _ := cmd.Flags().GetString("project")

	workDir := resolveProjectRoot(projectFlag, f.WorkingDir)

	// Resolve prompt source.
	prompt := promptText
	promptSource := "inline"
	if prompt == "" && promptFile == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			prompt = string(data)
			promptSource = "stdin"
		}
	} else if promptFile != "" {
		promptSource = promptFile
	}

	// Persona injection: load body and prepend to prompt as system-prompt addendum.
	if personaName != "" {
		loader := persona.NewPersonaLoader(workDir)
		p, err := loader.LoadPersona(personaName)
		if err != nil {
			return fmt.Errorf("persona %q: %w", personaName, err)
		}
		if p.Content != "" {
			if promptFile != "" {
				data, err := os.ReadFile(promptFile)
				if err != nil {
					return fmt.Errorf("reading prompt file %q: %w", promptFile, err)
				}
				prompt = p.Content + "\n\n" + string(data)
				promptFile = ""
				promptSource = personaName + "+" + promptSource
			} else {
				prompt = p.Content + "\n\n" + prompt
			}
		}
	}

	var timeout time.Duration
	if timeoutStr != "" {
		var err error
		timeout, err = time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}

	profile = agent.NormalizeRoutingProfile(profile)

	overrides := config.CLIOverrides{
		Harness:           harness,
		Model:             model,
		Provider:          provider,
		Profile:           profile,
		Effort:            effort,
		Permissions:       permissions,
		MinPower:          minPower,
		MaxPower:          maxPower,
		OpaquePassthrough: true,
	}
	if timeout > 0 {
		overrides.Timeout = &timeout
	}

	rcfg, _ := config.LoadAndResolve(workDir, overrides)

	runtime := agent.AgentRunRuntime{
		Prompt:       prompt,
		PromptFile:   promptFile,
		PromptSource: promptSource,
		WorkDir:      workDir,
	}

	result, err := agent.RunWithConfigViaService(cmd.Context(), workDir, rcfg, runtime)
	if err != nil {
		return err
	}

	if asJSON {
		outputFmt = "json-result"
	}
	switch outputFmt {
	case "json-result":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
		return runResultError(result)
	case "session-jsonl":
		if result.CondensedOutput != "" {
			fmt.Fprint(cmd.OutOrStdout(), result.CondensedOutput)
		} else if result.Output != "" {
			fmt.Fprint(cmd.OutOrStdout(), result.Output)
		}
	case "text":
		text := agent.ExtractOutput(result.Harness, result.Output)
		if text != "" {
			fmt.Fprint(cmd.OutOrStdout(), text)
		}
	default:
		return fmt.Errorf("unknown --output value %q (valid: text, json-result, session-jsonl)", outputFmt)
	}

	return runResultError(result)
}

func runResultError(result *agent.Result) error {
	if result == nil {
		return nil
	}
	if result.ExitCode != 0 {
		msg := fmt.Sprintf("agent exited with code %d", result.ExitCode)
		if result.Error != "" {
			msg += "\n  error: " + result.Error
		}
		if result.Stderr != "" && result.Stderr != result.Error {
			lines := strings.SplitN(strings.TrimSpace(result.Stderr), "\n", 6)
			if len(lines) > 5 {
				lines = append(lines[:5], "  ...")
			}
			msg += "\n  stderr:\n    " + strings.Join(lines, "\n    ")
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
