---
name: ddx-agent
description: Dispatch AI agents via DDx with profile-first routing and explicit overrides only when needed.
argument-hint: '"task description" [--profile=cheap|fast|smart] [--harness=<name>]'
---

# DDx Agent: Dispatch AI Agents

DDx agent dispatch lets you invoke AI agents through configured harnesses. Each
harness wraps a specific AI provider and model configuration. This skill guides
you through selecting the right harness and assembling the dispatch command.

## When to Use

- Running an AI agent against a prompt or task
- Selecting the right routing profile for a task
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

### 3. Select a Routing Profile

Use profiles as the default policy surface:

| Task Type | Profile | Rationale |
|-----------|---------|-----------|
| Simple lookup, formatting | `cheap` | Prefer low-cost/default local routing |
| Typical implementation task | `smart` | Balanced default for most real work |
| Latency-sensitive iteration | `fast` | Prefer quicker turnaround |

Only add `--model` or `--effort` when you are intentionally overriding profile
resolution for a specific regression, provider test, or controlled comparison.

### 4. Dispatch the Agent

Default routed run:

```bash
ddx agent run \
  --profile=smart \
  --prompt <path/to/prompt.md>
```

Pinned harness/model only when the task specifically requires it:

```bash
ddx agent run \
  --harness=<name> \
  --model=<exact-model> \
  --effort=high \
  --prompt <path/to/prompt.md>
```

Multi-agent consensus (majority vote across harnesses):

```bash
ddx agent run \
  --quorum=majority \
  --harnesses=harness-a,harness-b,harness-c \
  --prompt <path/to/prompt.md>
```

### 5. Review Session Log

```bash
ddx agent log
```

Shows history of agent sessions including harness used, effort, and outcome.

## Config Overrides

Harness defaults are set in `.ddx/config.yaml`. Use `--profile` for normal
dispatch. Add `--harness`, `--model`, or `--effort` only when you explicitly
want to override the profile/config decision for one run.

You can also adjust defaults by editing the config:

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
