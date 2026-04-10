# ADR-020: Prompt Template System with Quality Gates

**Date**: 2026-04-07  
**Status**: Proposed  
**Authors**: Prompt Engineering Experiment Results  

---

## Context

Our prompt engineering experiment (see [HELIX PR results](../../tests/PROMPT-ENGINEERING-RESULTS.md)) revealed that **explicit prompts with strict conventions** produce significantly better output than vague prompts:

| Metric | Baseline (Vague) | V2 (Explicit) | Improvement |
|--------|------------------|---------------|-------------|
| HELIX-compliant artifacts | 0% | 100% | ✓ |
| Cross-references | 0 | 28 | ✓ |
| Code + Tests generated | No | Yes (37 passing) | ✓ |
| Cost | $1.36 | $1.09 | -20% |

**Key insight**: Agents will do MORE work when constraints are specific, not less. The bottleneck is our ability to define, version, and validate prompts.

## Problems with Current DDx

1. **No prompt templates**: `ddx prompts list/show` exist but no way to define templates with constraints
2. **No quality gates**: Agent runs complete without validating output structure
3. **No prompt versioning**: Can't track which prompt version produced which results
4. **No A/B testing**: `ddx agent compare` exists but no integration with prompt versions
5. **Scattered measurement**: Our measurement scripts exist in HELIX repo but not integrated into DDx

## Decision

Implement a **Prompt Template System** with integrated **Quality Gates** and **Metrics Tracking**.

### 1. Prompt Templates (`ddx prompts`)

```yaml
# .ddx/prompts/helix-frame-v2.yaml
name: helix-frame
version: "2.0"
description: Create HELIX-compliant frame artifacts with strict naming
constraints:
  - id: naming-convention
    pattern: "^(FEAT|US|TD|TP)-[0-9]{3}-"
    description: "Artifacts must use FEAT-XXX, US-XXX, TD-XXX naming"
  - id: cross-references
    pattern: "\\[\\[.*\\]\\]"
    min_count: 5
    description: "Every artifact must have [[ID]] cross-references"
  - id: directory-structure
    paths:
      - "docs/helix/01-frame/prd.md"
      - "docs/helix/01-frame/features/FEAT-*.md"
      - "docs/helix/01-frame/user-stories/US-*.md"
output:
  expected_artifacts:
    - path: "docs/helix/01-frame/prd.md"
      required: true
    - path: "docs/helix/01-frame/features/*.md"
      min_count: 2
    - path: "docs/helix/01-frame/user-stories/*.md"
      min_count: 3
quality_gates:
  - name: naming-validate
    command: "ddx prompts validate --check naming"
    pass_threshold: 100
  - name: completeness-check
    command: "bash .ddx/prompts/helix-frame/completeness.sh"
    pass_threshold: 90
```

### 2. Quality Gates (`ddx agent run --gate`)

```bash
# Run agent with quality gates
ddx agent run --harness claude --prompt "Build a temperature converter" \
  --prompt-template helix-frame-v2 \
  --gate "ddx prompts validate helix-frame-v2" \
  --gate-threshold 90
```

**Gate behavior:**
- Gates run after agent completes
- Each gate produces a score (0-100)
- Exit code 0 if score >= threshold, 1 otherwise
- Full output captured for debugging

### 3. Prompt Versioning (`ddx prompts version`)

```bash
# Create new version from current
ddx prompts version helix-frame --create v3 --from v2

# List versions
ddx prompts versions helix-frame

# Run comparison across versions
ddx agent compare \
  --scenario tests/scenarios/A \
  --prompt-versions helix-frame-v1,helix-frame-v2 \
  --post-run "bash tests/measures/completeness.sh"
```

### 4. Metrics Integration

```bash
# Run metric with prompt version tracking
ddx metric run artifact-completeness \
  --prompt-version helix-frame-v2 \
  --run-id run-001

# Compare across prompt versions
ddx metric compare artifact-completeness \
  --against helix-frame-v1
```

## Implementation Plan

### Phase 1: Prompt Templates (MVP)
- [ ] Add `ddx prompts create` command
- [ ] Add `ddx prompts validate` command
- [ ] Create HELIX-specific prompt templates
- [ ] Add `--prompt-template` flag to `ddx agent run`

### Phase 2: Quality Gates
- [ ] Add `--gate` flag to `ddx agent run`
- [ ] Implement gate scoring system
- [ ] Add gate results to session logs
- [ ] Create standard HELIX quality gates

### Phase 3: Prompt Versioning
- [ ] Add `ddx prompts version` command
- [ ] Add `ddx prompts versions` command
- [ ] Integrate with `ddx agent compare`
- [ ] Add version metadata to metrics

### Phase 4: DDx Agent Configuration for External Models
- [ ] Document bragi configuration for qwen3.5-27b
- [ ] Add DDx Agent provider examples to config docs
- [ ] Create `ddx agent doctor` checks for external models

## API Design

### New Commands

```bash
# Prompts
ddx prompts create <name> [flags]
ddx prompts versions <name>
ddx prompts validate <name> [--output <dir>]

# Agent
ddx agent run [flags] --prompt-template <name>
ddx agent compare [flags] --prompt-versions <v1>,<v2>

# Metrics
ddx metric run <id> [--prompt-version <version>]
ddx metric compare <id> --against <version>
```

### Configuration Extension

```yaml
# .ddx/config.yaml
prompts:
  templates_dir: ".ddx/prompts"
  active_version: "v2"
  quality_gates:
    enabled: true
    threshold: 80

agent:
  agent_runner:
    provider: "openai-compat"
    base_url: "http://bragi:8080/v1"  # For qwen3.5-27b
    model: "qwen3.5-27b"
```

## Consequences

### Positive
- Reproducible agent behavior through versioned prompts
- Measurable quality improvements over iterations
- Integration with existing metrics system
- Foundation for automated prompt optimization

### Negative
- Additional complexity in prompt management
- Need to maintain prompt templates as code
- Validation adds latency to agent runs

### Risks
- Prompt templates may become too rigid
- Gate thresholds may need tuning per project type
- External model endpoints (bragi) need monitoring

## References

- [HELIX Prompt Engineering Results](../../tests/PROMPT-ENGINEERING-RESULTS.md)
- [DDx Metric System](../../../../cli/internal/metric)
- [DDx Agent Compare](../../../../cli/internal/agent/compare.go)
- [Agent Runner Integration](../../../../cli/internal/agent/agent_runner.go)
