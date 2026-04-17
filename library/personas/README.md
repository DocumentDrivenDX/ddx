# DDX Persona System

## Overview

Personas are reusable AI personality templates that define how AI assistants should behave when performing specific roles. They enable consistent, high-quality AI interactions across projects and team members.

## What is a Persona?

A persona is a markdown file that defines:
- **Personality**: The character and approach of the AI
- **Expertise**: Specific knowledge domains and skills
- **Principles**: Core values and methodologies
- **Communication Style**: How the AI should interact

## Using Personas

### Quick Start

```bash
# Load all personas bound to your project
ddx persona load

# Load a specific persona
ddx persona load strict-code-reviewer

# Check active personas
ddx persona status
```

### Binding Personas to Roles

Projects can bind specific personas to abstract roles:

```bash
# Bind a persona to a role
ddx persona bind code-reviewer strict-code-reviewer

# View current bindings
ddx persona bindings

# Remove a binding
ddx persona unbind code-reviewer
```

This creates entries in your `.ddx.yml`:
```yaml
persona_bindings:
  code-reviewer: strict-code-reviewer
  test-engineer: test-engineer-tdd
  architect: architect-systems
```

### Discovering Personas

```bash
# List all available personas
ddx persona list

# Find personas for a specific role
ddx persona list --role test-engineer

# View persona details
ddx persona show test-engineer-tdd
```

## Creating a New Persona

### File Structure

Create a new markdown file in `/personas/` with this structure:

```markdown
---
name: your-persona-name
roles: [role1, role2]  # Roles this persona can fulfill
description: Brief description of the persona
tags: [tag1, tag2, tag3]
---

# Persona Title

Main description of who this persona is and their expertise.

## Your Approach

How you approach problems and tasks...

## Key Principles

Core values and methodologies...

## Communication Style

How you interact and communicate...

## Expertise Areas

- Domain 1
- Domain 2
- ...

## Example Interactions

Provide examples of how this persona responds...
```

### Best Practices

1. **Be Specific**: Define clear behaviors and approaches
2. **Show Examples**: Include sample outputs or interactions
3. **Define Boundaries**: What the persona does and doesn't do
4. **Stay Focused**: One persona, one clear purpose
5. **Use Active Voice**: "You are..." not "The persona is..."

## How Personas Work

### Interactive Mode
When you run `ddx persona load`, the persona content is injected into your project's `CLAUDE.md` file. The AI assistant reads this file and adopts the specified personalities.

### Workflow Mode
Workflows can specify required roles:
```yaml
phases:
  - id: test
    required_role: test-engineer
```

When the workflow runs, DDX automatically uses the persona bound to that role.

## Cross-Harness Consumption Contract

**Personas are consumed by DDx itself, not by the underlying harness.**
When `ddx agent run --persona <name>` is invoked (or when a role is
bound in `.ddx/config.yaml` and a workflow resolves it), DDx loads the
persona Markdown file, strips the frontmatter, and injects the body as
a system-prompt addendum to the agent invocation. The harness sees a
flat system prompt, not a persona file — so the same persona file
produces equivalent behavior across Claude, Codex, Gemini, and any
other harness behind `ddx agent run`. Personas do not need per-harness
portability markers. See `docs/helix/01-frame/features/FEAT-006-agent-service.md`
(Personas section) for the authoritative contract.

## Quality Bar (for shipped personas)

Every persona in this directory must meet five criteria. The schema
check in `scripts/eval-skill.sh --validate` (Phase 4) and the
lefthook `test-engineer-persona-drift` pre-commit hook enforce parts
of this; reviewers enforce the rest:

1. **Opinionated voice.** A persona must take positions. Generic
   tutorial summaries of patterns ("you can use monoliths,
   microservices, CQRS, SAGA, event sourcing...") have no value
   over a default system prompt. Pick sides.
2. **Portable content.** No references to paths or files that only
   exist in the DDx repo (e.g., `cli/internal/agent/script.go`,
   `docs/helix/01-frame/concerns.md`). Downstream projects must be
   able to use the persona as-is.
3. **Traceable lineage.** Every persona ends with a **Sources**
   section in the body (not in frontmatter) citing the external
   canon that shaped it — Anthropic Prompt Library entries, OpenAI
   Cookbook Prompt Personalities, a named community skill, or a
   canonical paper. Frontmatter stays lean; lineage lives in prose.
4. **Role binding verified.** The `roles:` field names roles
   actually referenced by some shipped workflow or skill.
5. **Schema conformance.** YAML frontmatter contains `name`, `roles`,
   `description`, `tags`. Body structure follows **Philosophy /
   Approach / Anti-patterns / Sources**.

## External Canon

Primary sources for grounding new personas:

- **[Anthropic Prompt Library](https://docs.anthropic.com/en/resources/prompt-library/)** — official curated prompts; cite entries that inspired a persona.
- **[OpenAI Cookbook: Prompt Personalities](https://developers.openai.com/cookbook/examples/gpt-5/prompt_personalities)** — GPT-5 persona examples worth mining for structure.

Specific-use sources (cite when a persona draws from one):

- **[Superpowers](https://openclawapi.org/en/blog/2026-03-14-superpowers)** — two-stage review (conformance → quality) pattern.
- **fspec** — spec-driven agent discipline; primary canon for `specification-enforcer`.

Inspiration catalogs (not canon, but useful browsing):

- **[awesome-claude-skills](https://github.com/travisvn/awesome-claude-skills)** — community catalog.

## Available Personas (shipped roster)

| Persona | Roles | Description |
|---------|-------|-------------|
| `code-reviewer` | code-reviewer, security-analyst | Security-first reviewer with structured verdict output |
| `test-engineer` | test-engineer, quality-analyst | Stubs-over-mocks, real e2e, baselined performance |
| `implementer` | implementer, software-engineer | YAGNI / KISS / DOWITYTD, ships tests with code |
| `architect` | architect, technical-lead | Opinionated on when to reach for each pattern |
| `specification-enforcer` | specification-enforcer, compliance-analyst | Refuses drift from governing artifacts |

Deprecated (scheduled for removal after one release):
`strict-code-reviewer` → `code-reviewer`;
`test-engineer-tdd` → `test-engineer`;
`architect-systems` → `architect`;
`pragmatic-implementer` → `implementer`.

Dropped (no replacement, use the 5 above or ship your own):
`reliability-guardian`, `simplicity-architect`,
`data-driven-optimizer`, `product-discovery-analyst`,
`product-manager-minimalist`.

### Contributing Personas

1. Create your persona file following the template
2. Test it in your projects
3. Submit a PR to share with the community

## Role vs Persona

- **Role**: Abstract function (e.g., "code-reviewer")
- **Persona**: Concrete implementation (e.g., "strict-code-reviewer")

Workflows define required **roles**. Projects bind **personas** to those roles.

## Example Workflow

1. **Setup Project Bindings**:
```bash
ddx persona bind code-reviewer strict-code-reviewer
ddx persona bind test-engineer test-engineer-tdd
```

2. **Load for Interactive Session**:
```bash
ddx persona load
# Now Claude/AI has all bound personas active
```

3. **Run Workflow**:
```bash
ddx workflow run helix
# Workflow automatically uses bound personas for each phase
```

## FAQ

### Q: Can one persona fulfill multiple roles?
A: Yes, personas can declare multiple roles they can fulfill.

### Q: What happens if no persona is bound to a role?
A: DDX will prompt you to select from available personas for that role.

### Q: Can I override personas for specific workflows?
A: Yes, use the `overrides` section in `.ddx.yml`:
```yaml
persona_bindings:
  test-engineer: test-engineer-tdd

  overrides:
    helix:
      test-engineer: test-engineer-bdd
```

### Q: How do I remove all loaded personas?
A: Run `ddx persona unload` to clear all personas from CLAUDE.md.

### Q: Can I use personas with other AI tools?
A: Yes! Personas are just markdown - they work with any AI that can read instructions.

## Troubleshooting

### Persona Not Found
```
Error: Persona 'my-persona' not found
```
- Check that the persona file exists in `/personas/`
- Verify the name matches exactly (case-sensitive)

### No Binding for Role
```
Warning: No persona bound to role 'architect'
```
- Run `ddx persona bind architect <persona-name>`
- Or select a persona when prompted

### Invalid Persona Format
```
Warning: Skipping invalid persona 'broken.md'
```
- Check YAML frontmatter syntax
- Ensure required fields (name, roles, description) are present

## Learn More

- See existing personas in `/personas/` for examples
- Read the [Feature Specification](../../docs/helix/01-frame/features/FEAT-011-persona-system.md)
- Check the [Solution Design](../../docs/helix/02-design/solution-designs/SD-011-persona-system.md)

---

*Personas enable consistent, high-quality AI interactions by defining reusable personality templates that can be shared across teams and projects.*