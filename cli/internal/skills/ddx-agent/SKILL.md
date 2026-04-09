---
skill:
  name: ddx-agent
  description: Dispatch AI agents via DDx with the right harness, model, and effort for the task.
  args:
    - name: task
      description: Description of the task to dispatch
      required: false
---

# DDx Agent: Dispatch AI Agents

DDx agent dispatch lets you invoke AI agents through configured harnesses. Each
harness wraps a specific AI provider and model configuration. This skill guides
you through selecting the right harness and assembling the dispatch command.

## When to Use

- Running an AI agent against a prompt or task
- Selecting the right model and effort level for a task
- Dispatching multiple agents for consensus on a decision
- Checking which harnesses and models are available

## Steps

### 1. List Available Harnesses

```bash
ddx agent list
```

This shows all configured harnesses with their names and descriptions. Identify
candidates for your task type.

### 2. Check Harness Capabilities

```bash
ddx agent capabilities <harness>
```

Shows available models and effort levels for the harness. Use this to understand
the cost/quality tradeoff options before dispatching.

### 3. Select Model and Effort

Match effort to task complexity:

| Task Type | Effort | Rationale |
|-----------|--------|-----------|
| Simple lookup, formatting | `low` | Fast, cheap; no deep reasoning needed |
| Typical implementation task | `medium` | Balanced; handles most work |
| Architecture, complex reasoning | `high` | Full context window, extended thinking |

### 4. Dispatch the Agent

Single harness:

```bash
ddx agent run \
  --harness=<name> \
  --effort=medium \
  --prompt <path/to/prompt.md>
```

Multi-agent consensus (majority vote across harnesses):

```bash
ddx agent run \
  --quorum=majority \
  --harnesses=harness-a,harness-b,harness-c \
  --effort=medium \
  --prompt <path/to/prompt.md>
```

### 5. Review Session Log

```bash
ddx agent log
```

Shows history of agent sessions including harness used, effort, and outcome.

## Config Overrides

Harness defaults are set in `.ddx/config.yaml`. You can override per-invocation
with flags, or adjust defaults by editing the config:

```yaml
agent:
  default_harness: claude-sonnet
  default_effort: medium
```

Check `ddx agent doctor` if a harness is unavailable or returns errors — it
diagnoses configuration and credential issues.

## References

- Full flag list: `ddx agent --help`, `ddx agent run --help`
- Agent service feature spec: `docs/helix/01-frame/features/FEAT-006-agent-service.md`
