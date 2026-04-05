---
title: Creating Plugins
weight: 6
---

DDx plugins extend DDx with workflow methodologies, skills, and resources.
HELIX is the first plugin. You can create your own.

## What a Plugin Provides

A plugin is a git repository that declares installable artifacts:

- **Skills** — SKILL.md files that agents discover as slash commands
- **Scripts** — CLI tools installed to `~/.local/bin/`
- **Resources** — personas, templates, patterns added to the library

## Plugin Structure

Your plugin repo needs a `package.yaml` at a known path:

```yaml
# workflows/your-plugin/package.yaml
name: my-workflow
version: 0.1.0
description: My custom development workflow
type: workflow
source: https://github.com/you/my-workflow
install:
  skills:
    source: .agents/skills/    # directory in your repo
    target: ~/.agents/skills/  # where DDx installs them
  scripts:
    source: scripts/
    target: ~/.local/bin/
```

## Skill Format

Each skill is a directory with a `SKILL.md`:

```
.agents/skills/
├── my-frame/
│   └── SKILL.md
├── my-build/
│   └── SKILL.md
└── my-review/
    └── SKILL.md
```

The SKILL.md has YAML frontmatter and markdown guidance:

```markdown
---
skill:
  name: my-build
  description: Build the current project using my methodology.
  args: [bead-id]
---

# Build

Steps the agent should follow when this skill is invoked...
```

## Registering with DDx

Currently, DDx uses a built-in registry. To add your plugin:

1. Open a PR to [ddx](https://github.com/DocumentDrivenDX/ddx) adding your
   package to `cli/internal/registry/registry.go` in the `BuiltinRegistry()` function
2. Include your `Package` struct with name, version, source URL, and install mappings

Future: a `registry.yaml` in the [ddx-library](https://github.com/DocumentDrivenDX/ddx-library) repo will replace the built-in registry.

## Example: HELIX Plugin

HELIX is registered as:

```go
Package{
    Name:        "helix",
    Version:     "0.1.0",
    Description: "Structured development workflow with AI-assisted collaboration",
    Type:        PackageTypeWorkflow,
    Source:      "https://github.com/DocumentDrivenDX/helix",
    Install: PackageInstall{
        Skills: &InstallMapping{
            Source: ".agents/skills/",
            Target: "~/.agents/skills/",
        },
    },
}
```

When a user runs `ddx install helix`, DDx:
1. Shallow-clones the helix repo
2. Copies everything under `.agents/skills/` to `~/.agents/skills/`
3. Records the installation in `~/.ddx/installed.yaml`

## Testing Your Plugin

```bash
# Install from your local repo
ddx install my-workflow

# Verify skills are installed
ls ~/.agents/skills/my-*

# Verify the agent discovers them
# (skills appear as /my-build, /my-review, etc. in Claude Code)
```
