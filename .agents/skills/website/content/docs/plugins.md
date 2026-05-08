---
title: Plugins
weight: 6
---

# Plugins {{< maturity "not-started" >}}

DDx plugins extend the platform with workflow methodologies, agent skills, CLI
tools, and library resources. HELIX is the reference plugin. You can create
your own.

The plugin API itself — versioned manifests, capability negotiation, and a
stable contract for third-party plugins — has not started. The install path
described below works today for HELIX and DDx-internal plugins; it is not yet
a public, supported integration surface.

## What a Plugin Provides

A plugin is a git repository that ships some combination of:

| Artifact | Description | Install location |
|----------|-------------|-----------------|
| **Root** | The entire plugin tree | `.ddx/plugins/<name>/` |
| **Skills** | SKILL.md files agents discover as slash commands | `.agents/skills/` and `.claude/skills/` |

All install outputs are project-local — DDx writes nothing under `~/`.
Skills are real-file copies, not symlinks. The project-local tree
(`.ddx/plugins/`, `.agents/skills/`, `.claude/skills/`) is committed to
git so teammates and CI get the same skills on clone.

The plugin root at `.ddx/plugins/<name>/` can contain any DDx resource type:

| Resource | Path in plugin | Description |
|----------|---------------|-------------|
| Prompts | `prompts/` | AI prompts and system instructions |
| Templates | `templates/` | Project and artifact templates |
| Patterns | `patterns/` | Reusable code patterns |
| Personas | `personas/` | AI persona definitions |
| MCP servers | `mcp-servers/` | MCP server configurations |
| Environments | `environments/` | Development environment configs |
| Tools | `tools/` | Tool configurations |

This mirrors the DDx library structure (`.ddx/library/`). A plugin can ship
any subset — a persona-pack might only have `personas/`, while a full workflow
plugin like HELIX ships skills, a CLI, prompts, templates, and action docs.

## How Installation Works

When you run `ddx install <name>`:

1. DDx fetches the latest release tarball from the plugin's GitHub repo
2. Extracts it to a temp directory
3. Copies the **Root** (the entire plugin) to `.ddx/plugins/<name>/`
4. Copies each skill from the plugin into `.agents/skills/` and
   `.claude/skills/` as real files (no symlinks)
5. Records the installation in the project's local install manifest

Every file written is a real file under the project root. DDx creates
zero symlinks and writes nothing under `~/` — the resulting tree is
portable across Linux, macOS, and Windows and can be committed to git.

## Plugin Structure

A plugin repo mirrors the DDx resource layout. Include only the directories
you need:

```
my-plugin/
├── .agents/
│   └── skills/             # agent skills (copied to project on install)
│       ├── my-build/
│       │   └── SKILL.md
│       └── my-review/
│           └── SKILL.md
├── bin/
│   └── my-plugin           # CLI entry point
├── prompts/                # AI prompts and instructions
├── templates/              # project or artifact templates
├── patterns/               # reusable code patterns
├── personas/               # AI persona definitions
└── README.md
```

A plugin can also include its own internal resources that skills reference
at runtime. HELIX, for example, ships `workflows/` with action prompts,
phase templates, and reference docs — these are HELIX-specific, not a DDx
convention.

### Key conventions

- **Skills** live in `.agents/skills/<skill-name>/SKILL.md`
- **CLI entry point** lives in `bin/<plugin-name>`
- **Resources** (prompts, templates, patterns, personas) follow the same
  directory structure as `.ddx/library/`
- **Path references** in skills must use the installed path prefix
  `.ddx/plugins/<name>/` so agents can resolve them from any project

### Skill format

Each skill directory contains a `SKILL.md` with YAML frontmatter:

```markdown
---
name: my-build
description: Build the current project using my methodology.
argument-hint: "[scope]"
---

# Build

Steps the agent should follow when this skill is invoked.

## When to Use

- Starting implementation work
- After design is complete

## Steps

1. Load context — read governing artifacts
2. Implement the change
3. Run tests
4. Commit with traceability

## References

- Build prompt: `.ddx/plugins/my-plugin/prompts/build.md`
- Templates: `.ddx/plugins/my-plugin/templates/`
```

### CLI entry point

The `bin/<name>` script is the CLI that users invoke directly. It typically:

1. Resolves the plugin root (from `CLAUDE_PLUGIN_ROOT` in plugin mode, or
   from its own location)
2. Sets up context paths for shared resources
3. Delegates to the main script logic

```bash
#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${CLAUDE_PLUGIN_ROOT:-}" ]]; then
  PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT}"
else
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  PLUGIN_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi

export PLUGIN_ROOT
exec "${PLUGIN_ROOT}/scripts/main" "$@"
```

## Package Descriptor

Each plugin is described by a `Package` struct in the DDx registry. The
descriptor declares what to install and where:

```yaml
name: my-plugin
version: 1.0.0
description: A workflow plugin for DDx
type: workflow
source: https://github.com/you/my-plugin
keywords:
  - workflow
  - methodology
install:
  root:
    source: "."                           # entire repo
    target: ".ddx/plugins/my-plugin"      # project-local install
  skills:
    - source: ".agents/skills/"           # skills dir in repo
      target: ".agents/skills/"           # copied here
    - source: ".agents/skills/"
      target: ".claude/skills/"           # and here
  executable:                              # files that must be executable
    - "bin/my-plugin"
    - "scripts/main"
```

### Package types

| Type | Description |
|------|-------------|
| `workflow` | Full workflow methodology (skills + CLI + resources) |
| `plugin` | Tool plugin (e.g., quality checks) |
| `persona-pack` | Collection of AI personas |
| `template-pack` | Collection of project templates |
| `resource` | Individual resource file |

### Install mappings

| Field | Purpose | Behavior |
|-------|---------|----------|
| `root` | Copy entire plugin to `.ddx/plugins/<name>/` | Full directory copy from tarball |
| `skills` | Copy skill directories into agent paths | Each entry in source dir is copied into target dir |
| `executable` | Paths (relative to root) that must be +x | Execute bit set after root copy |

All install outputs are real files under the project root. DDx never
creates symlinks during install and never writes outside `<projectRoot>/`.

## Registering Your Plugin

Currently, DDx uses a built-in Go registry. To add your plugin:

1. Fork [ddx](https://github.com/DocumentDrivenDX/ddx)
2. Add your package to `cli/internal/registry/registry.go` in the
   `BuiltinRegistry()` function
3. Open a PR with your `Package` definition

### What to include in your PR

- Package name, description, type, and source URL
- Install mappings for root, skills, and scripts
- Keywords for search discoverability
- A tagged release on your plugin repo (DDx fetches release tarballs)

### Example: HELIX registration

```go
Package{
    Name:        "helix",
    Version:     "1.0.0",
    Description: "Supervisory autopilot for AI-assisted software delivery",
    Type:        PackageTypeWorkflow,
    Source:      "https://github.com/DocumentDrivenDX/helix",
    Install: PackageInstall{
        Root: &InstallMapping{
            Source: ".",
            Target: ".ddx/plugins/helix",
        },
        Skills: []InstallMapping{
            {Source: ".agents/skills/", Target: ".agents/skills/"},
            {Source: ".agents/skills/", Target: ".claude/skills/"},
        },
        Executable: []string{"bin/helix", "scripts/helix"},
    },
    Keywords: []string{"workflow", "methodology", "ai", "development"},
}
```

Future: a `registry.yaml` in the
[ddx-library](https://github.com/DocumentDrivenDX/ddx-library) repo will
replace the built-in registry so plugins can be added without modifying DDx
itself.

## CLI Commands

```bash
ddx search <query>           # Search available packages
ddx install <name>           # Install a plugin or workflow
ddx install <name> --force   # Reinstall even if up to date
ddx installed                # List installed packages
ddx outdated                 # Check for available updates
ddx uninstall <name>         # Remove an installed package
```

### Individual resources

You can also install individual resources from the DDx library without creating
a full plugin:

```bash
ddx install persona/strict-code-reviewer   # Install one persona
ddx install template/go-service            # Install one template
```

These are fetched directly from the
[ddx-library](https://github.com/DocumentDrivenDX/ddx-library) repo and placed
in `.ddx/library/<type>/`.

## Plugin Development

### Path references

Skills and prompts reference resources using paths relative to the project
root. Since the plugin is installed at `.ddx/plugins/<name>/`, all paths must
use that prefix:

```markdown
# Good — resolves from any project that installs the plugin
Read `.ddx/plugins/my-plugin/prompts/build.md`
Read `.ddx/plugins/my-plugin/templates/design-doc.md`

# Bad — only works if plugin repo is the working directory
Read `prompts/build.md`
```

### Dev environment setup

When developing your plugin, create a symlink so the installed paths resolve
from within your repo:

```bash
mkdir -p .ddx/plugins
ln -sfn ../.. .ddx/plugins/my-plugin
```

Add `.ddx/plugins/` to your `.gitignore` — it's a dev convenience, not repo
content.

### Doctor checks

Add a health check to your CLI's `doctor` command that verifies
`.ddx/plugins/<name>/` exists in the target project. This catches the case
where someone has skills installed but the plugin root is missing.

### Pre-commit guard

Add a lefthook or pre-commit check that catches bare resource paths. The
pattern depends on your plugin's internal structure — catch any references
that skip the `.ddx/plugins/<name>/` prefix:

```yaml
# lefthook.yml
pre-commit:
  commands:
    check-plugin-paths:
      run: |
        # Adapt the grep pattern to match your plugin's resource directories
        if grep -rn '`prompts/\|`templates/\|`patterns/' skills/ --include='*.md' 2>/dev/null \
           | grep -v '.ddx/plugins/my-plugin/'; then
          echo "ERROR: Use .ddx/plugins/my-plugin/ prefix for resource paths"
          exit 1
        fi
      glob: "skills/**/*.md"
```

### Testing

```bash
# Install your plugin from the registry
ddx install my-plugin

# Verify the plugin root
ls .ddx/plugins/my-plugin/

# Verify skills are copied
ls .agents/skills/my-*
ls .claude/skills/my-*

# Run your doctor (if applicable)
my-plugin doctor
```

## Version Management

- DDx checks GitHub for the latest tagged release when installing
- `ddx install <name>` skips if already at the latest version (use `--force`
  to reinstall)
- `ddx outdated` compares installed versions against latest releases
- Plugin repos should use semver tags (`v1.0.0`, `v1.1.0`, etc.)
