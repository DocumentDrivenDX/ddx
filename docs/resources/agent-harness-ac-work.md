---
ddx:
  id: AC-AGENT-002
  status: complete
  depends_on:
    - AC-AGENT-001
---
# Agent Harness Test Completion - COMPLETED

**ID:** AC-AGENT-002
**Status:** Complete
**Date:** 2026-04-07

## Summary

All harness acceptance tests have been implemented and verified. Both pi and gemini now have:
- Proper harness definitions with correct flags
- Complete unit tests for properties, arg construction, execution, output extraction
- Integration tests that pass
- Cost tracking where available

## Changes Made

### Cursor Support Removed
- Removed from `PreferenceOrder` and `builtinHarnesses` in registry.go
- Removed all cursor tests from agent_test.go
- Cursor doesn't run on Linux, so support was dropped

### Harness Definitions Updated

**pi** (`cli/internal/agent/registry.go`):
```go
"pi": {
    Name:            "pi",
    Binary:          "pi",
    BaseArgs:        []string{"--mode", "json", "--print"},
    PromptMode:      "arg",
    ModelFlag:       "--model",
    EffortFlag:      "--thinking",
    ReasoningLevels: []string{"low", "medium", "high"},
}
```

**gemini** (`cli/internal/agent/registry.go`):
```go
"gemini": {
    Name:            "gemini",
    Binary:          "gemini",
    BaseArgs:        []string{},
    PromptMode:      "stdin",
    ModelFlag:       "-m",
    ReasoningLevels: []string{"low", "medium", "high"},
}
```

### Output Parsing Implemented

**pi** - JSONL format with cost in intermediate events:
```json
{"type":"text_end","message":{"usage":{"input":135,"output":52,"cost":{"total":0.0003714}}}}
```
- `ExtractUsage`: Scans backwards through JSONL to find cost data
- `ExtractOutput`: Extracts `response` field from summary JSON

**gemini** - Single JSON format (no cost in output):
```json
{"session_id":"...","response":"...","stats":{"models":{"...":{"tokens":{"input":2404,"total":9367}}}}}
```
- `ExtractUsage`: Parses token counts from stats.models[].tokens
- `ExtractOutput`: Extracts `response` field
- Note: No cost data in gemini JSON output

### Test Results

```
Unit tests:     ✅ All pass
Integration:
  - pi:         ✅ PASS
  - gemini:     ✅ PASS  
  - claude:     ❌ FAIL (pre-existing auth issue, unrelated)
```

## Remaining Minor Items

1. **File organization** — Consider splitting tests into separate files per harness
2. **Pi effort levels** — Document mapping of `--thinking` levels to standardized levels

These are minor improvements and don't block the acceptance criteria.
