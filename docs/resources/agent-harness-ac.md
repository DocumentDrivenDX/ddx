---
ddx:
  id: AC-AGENT-001
  status: draft
  depends_on:
    - FEAT-006
---
# Agent Harness Acceptance Criteria

**ID:** AC-AGENT-001
**Status:** Draft
**Purpose:** Define mandatory and recommended acceptance criteria for each
built-in agent harness in the DDx agent service.

## Background

The DDx agent service (FEAT-006) provides a unified interface for invoking
AI coding agents via a pluggable "harness" abstraction. Each harness translates
`RunOptions` into process invocation (or in-process call for embedded harnesses)
and parses structured output (tokens, cost, response text).

This document defines the acceptance criteria each harness implementation must
satisfy before being considered complete. These criteria serve as:

1. **Implementation checklist** — harness authors verify each criterion
2. **Regression guard** — automated tests prevent accidental breakage
3. **Onboarding guide** — new harness implementations follow the pattern

## Harness Registry Acceptance

Every built-in harness must satisfy:

| Criterion | Description | Test |
|-----------|-------------|------|
| R-001 | Harness is registered in `builtinHarnesses` map | `TestRegistryBuiltinHarnesses` |
| R-002 | Harness appears in `PreferenceOrder` list | `TestRegistryNamesPreferenceOrder` |
| R-003 | `NewRegistry().Has("<name>")` returns true | `TestRegistryBuiltinHarnesses` |
| R-004 | `NewRegistry().Get("<name>")` returns correct `Harness` struct | `TestRegistryGet` |

## Harness Properties

Each harness struct must define:

| Property | Required | Description |
|----------|----------|-------------|
| `Name` | Yes | Harness identifier, e.g. `"pi"` |
| `Binary` | Yes | Executable name or sentinel for embedded (`"agent"`, `"virtual"`) |
| `PromptMode` | Yes | `"arg"` (prompt as final arg) or `"stdin"` (pipe) |
| `BaseArgs` | No | Args always present regardless of permission level |
| `PermissionArgs` | No | Extra args keyed by permission level (`"safe"`, `"supervised"`, `"unrestricted"`) |
| `ModelFlag` | No | Flag for model override, e.g. `"-m"` or `"--model"` |
| `WorkDirFlag` | No | Flag for working directory, e.g. `"-C"` or `"--cwd"` |
| `EffortFlag` | No | Flag for reasoning effort level |
| `EffortFormat` | No | `fmt.Sprintf` format for effort value, e.g. `"reasoning.effort=%s"` |
| `TokenPattern` | No | Regex with one capture group for token extraction |
| `DefaultModel` | No | Built-in model choice when no override exists |
| `ReasoningLevels` | No | Supported effort levels in preference order |

| Criterion | Description | Test |
|-----------|-------------|------|
| P-001 | `Name` matches the registry key | `Test<Harness>HarnessProperties` |
| P-002 | `Binary` is correct executable name | `Test<Harness>HarnessProperties` |
| P-003 | `PromptMode` is `"arg"` or `"stdin"` | `Test<Harness>HarnessProperties` |
| P-004 | Properties match harness definition in registry | `Test<Harness>HarnessProperties` |

## Arg Construction (BuildArgs)

`BuildArgs` constructs the argument array for a harness invocation. Tests verify:

| Criterion | Description | Test |
|-----------|-------------|------|
| A-001 | Basic invocation produces expected `BaseArgs` | `TestBuildArgs<Harness>Basic` |
| A-002 | `PromptMode="arg"` includes prompt as final argument | `TestBuildArgs<Harness>Basic` |
| A-003 | `PromptMode="stdin"` does NOT include prompt in args | `TestBuildArgs<Harness>Stdin` |
| A-004 | Model flag appears when model is provided | `TestBuildArgs<Harness>WithModel` |
| A-005 | Model flag does NOT appear when model is empty | `TestBuildArgsNoModelFlagWhenEmpty` |
| A-006 | WorkDir flag appears when `WorkDirFlag` is set and `WorkDir` is provided | `TestBuildArgs<Harness>AllFlags` |
| A-007 | Effort flag appears when `EffortFlag` is set and `Effort` is provided | `TestBuildArgs<Harness>AllFlags` |
| A-008 | Permission args are appended correctly for each level | `TestBuildArgsPermissionsDefault`, `TestBuildArgsPermissionsUnrestricted` |
| A-009 | EffortFormat is applied when defined | `TestBuildArgs<Harness>AllFlags` (codex) |

## Execution (Run)

`Runner.Run` invokes the harness and processes results. Tests verify:

| Criterion | Description | Test |
|-----------|-------------|------|
| E-001 | Correct binary is invoked | `TestRun<Harness>WithMockExecutor` |
| E-002 | Correct args are passed | `TestRun<Harness>WithMockExecutor` |
| E-003 | `PromptMode="stdin"` sends prompt via stdin | `TestRun<Harness>WithMockExecutor` |
| E-004 | `PromptMode="arg"` sends prompt as argument | `TestRun<Harness>WithMockExecutor` (arg mode harnesses) |
| E-005 | Output is captured correctly | `TestRun<Harness>WithMockExecutor` |
| E-006 | Exit code is propagated | `TestRun<Harness>WithMockExecutor` |
| E-007 | WorkDir is used when `WorkDirFlag` is set | `TestRun<Harness>WorkDir` |
| E-008 | WorkDir is passed to `ExecuteInDir` when `WorkDirFlag` is empty | `TestRun<Harness>WorkDir` (subprocess without flag) |

## Output Processing

### ExtractUsage

Parses token usage from harness output. Behavior depends on `TokenPattern` and harness-specific logic.

| Criterion | Description | Test |
|-----------|-------------|------|
| U-001 | Valid structured output returns correct `UsageData` | `TestExtractUsage<Harness>WithUsage` |
| U-002 | Output with preamble (spinner, etc.) parses last valid line | `TestExtractUsage<Harness>LastLine` |
| U-003 | No usage in output returns `UsageData{}` | `TestExtractUsage<Harness>NoUsage` |
| U-004 | Malformed output returns `UsageData{}` | `TestExtractUsage<Harness>Garbage` |
| U-005 | Harness without token support returns `UsageData{}` | `TestExtractUsagePi` |

### ExtractOutput

Extracts clean text from raw harness output.

| Criterion | Description | Test |
|-----------|-------------|------|
| O-001 | Known harness returns extracted text | `TestExtractOutput<Harness>` (codex, claude, opencode) |
| O-002 | Unknown harness returns raw output as-is | `TestExtractOutputPi` |
| O-003 | Harness with structured JSON returns `result` field | `TestExtractOutputClaude` |

## Capabilities

`Runner.Capabilities` reports harness capabilities.

| Criterion | Description | Test |
|-----------|-------------|------|
| C-001 | Returns correct harness name | `TestCapabilities<Harness>` |
| C-002 | Returns `Available: true` (or embedded harness status) | `TestCapabilities<Harness>` |
| C-003 | Returns binary name | `TestCapabilities<Harness>` |
| C-004 | Returns reasoning levels from harness definition | `TestCapabilities<Harness>` |
| C-005 | Returns model from config or default | `TestCapabilities<Harness>` (claude) |

## Integration Tests

Real invocation against the actual harness binary (skipped if binary not available).

| Criterion | Description | Test |
|-----------|-------------|------|
| I-001 | Harness responds to simple prompt | `TestIntegration_<Harness>Echo` |

## Per-Harness Acceptance Status

| Harness | Registry | Properties | BuildArgs | Execution | ExtractUsage | ExtractOutput | Capabilities | Integration |
|---------|----------|------------|-----------|-----------|-------------|--------------|--------------|-------------|
| codex | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| claude | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| gemini | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| opencode | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| agent | ✅ | ✅ | ✅ | ✅ | N/A | ✅ | ✅ | N/A |
| pi | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| virtual | ✅ | ✅ | ✅ | ✅ | N/A | N/A | N/A | N/A |

**Notes:**
- cursor was removed (doesn't run on Linux)
- All harnesses with JSON output (pi, gemini) now have cost tracking
- pi: parses cost from JSONL intermediate events
- gemini: no cost in JSON output (external tracking required)
- agent: embedded harness, cost tracked directly from the agent library result
- N/A for ExtractUsage/ExtractOutput: embedded harnesses use direct API tracking

## Adding a New Harness

1. **Register** in `cli/internal/agent/registry.go`:
   - Add to `builtinHarnesses` map
   - Add to `PreferenceOrder` list (position by preference)

2. **Define properties** in the harness struct:
   - Set `PromptMode` correctly (`"arg"` vs `"stdin"`)
   - Define flags for model, workdir, effort as applicable
   - Set `ReasoningLevels` if supported

3. **Add output extraction** in `ExtractUsage` and `ExtractOutput` if harness has structured output

4. **Write tests** following the pattern:
   - `Test<Harness>HarnessProperties`
   - `TestBuildArgs<Harness>Basic`
   - `TestBuildArgs<Harness>Stdin` (for stdin mode)
   - `TestBuildArgs<Harness>AllFlags`
   - `TestRun<Harness>WithMockExecutor`
   - `TestExtractUsage<Harness>*` (if applicable)
   - `TestExtractOutput<Harness>` (if applicable)
   - `TestCapabilities<Harness>`
   - `TestIntegration_<Harness>Echo` (optional, requires binary)

5. **Verify** all criteria: `go test ./internal/agent/... -run "<Harness>"`

## Open Gaps

- [ ] Consider file organization refactor (split harness tests into separate files)
- [ ] Document pi `--thinking` effort levels mapping to standardized levels
