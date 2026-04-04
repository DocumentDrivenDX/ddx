# DDx CLI Command Reference

## Command Structure Philosophy

DDx follows a **noun-verb** command structure for better organization, discoverability, and consistency. This means resources (nouns) come first, followed by actions (verbs).

**Pattern:** `ddx <resource> <action> [options]`

This design provides:
- Clear mental model: "I want to work with prompts" → `ddx prompts ...`
- Better tab completion: Type `ddx prompts` and see all prompt-related actions
- Consistent experience across all resource types
- Natural command grouping in help text

## Core Commands

These commands operate at the project level:

### `ddx init`
Initialize DDx in your project.

```bash
ddx init                    # Interactive initialization
ddx init --template nextjs  # Initialize with specific template
```

### `ddx doctor`
Analyze your project health and suggest improvements.

```bash
ddx doctor         # Analyze and report
ddx doctor --fix   # Analyze and apply fixes
```

### `ddx upgrade`
Upgrade DDx binary to the latest release version.

```bash
ddx upgrade          # Upgrade to latest version
ddx upgrade --check  # Only check for updates
ddx upgrade --force  # Force upgrade even if already latest
```

### `ddx update`
Update DDx toolkit resources from the master repository.

```bash
ddx update           # Update all resources
ddx update templates # Update only templates
```

### `ddx contribute`
Share your improvements back to the community.

```bash
ddx contribute patterns/my-pattern  # Contribute a specific pattern
```

## Resource Commands

All resource commands follow the noun-verb pattern:

### Prompts

AI prompts and instructions for development tasks.

```bash
ddx prompts list                     # List all available prompts
ddx prompts list --verbose           # List with file details
ddx prompts list --search review     # Search for specific prompts
ddx prompts show claude/code-review  # Display a specific prompt
```

### Templates

Project templates and boilerplate code.

```bash
ddx templates list                   # List all templates
ddx templates show nextjs            # Show template details
ddx templates apply nextjs           # Apply template to current project
```

### Patterns

Reusable code patterns and implementations.

```bash
ddx patterns list                    # List all patterns
ddx patterns show error-handling     # Show pattern details
ddx patterns apply error-handling    # Apply pattern to project
```

### Personas

AI personality definitions for consistent interactions.

```bash
ddx persona list                           # List available personas
ddx persona show strict-code-reviewer     # Show persona details
ddx persona bind code-reviewer strict-code-reviewer  # Bind persona to role
ddx persona load                          # Load personas into CLAUDE.md
ddx persona status                        # Show loaded personas
```

### MCP Servers

Model Context Protocol server configurations.

```bash
ddx mcp list                         # List available MCP servers
ddx mcp show filesystem              # Show server details
ddx mcp install filesystem           # Install MCP server
```

## Common Options

Most commands support these common options:

- `--help` - Show help for any command
- `--verbose` / `-v` - Show detailed output
- `--search <term>` - Filter results (for list commands)
- `--library-base-path <path>` - Override library location

## Examples

### Finding and Using Prompts

```bash
# See what prompts are available
ddx prompts list

# Look for code review prompts
ddx prompts list --search review

# View a specific prompt
ddx prompts show claude/code-review

# See all prompt files (not just directories)
ddx prompts list --verbose
```

### Working with Templates

```bash
# Browse available templates
ddx templates list

# Get details about a template
ddx templates show nextjs

# Apply template to current project
ddx templates apply nextjs
```

### Managing AI Personas

```bash
# See available personas
ddx persona list

# Bind a persona to a role
ddx persona bind code-reviewer strict-code-reviewer

# Load all bound personas
ddx persona load
```

## Library Path Resolution

DDx uses a smart library path resolution system with the following priority:

1. Command-line flag: `--library-base-path`
2. Environment variable: `DDX_LIBRARY_BASE_PATH`
3. Config file: `library_path` in `.ddx.yml`
4. Development mode: `./library` in DDx repository
5. Project library: `.ddx/library/`
6. Global fallback: `~/.ddx/library/`

This ensures DDx works correctly in development, project-specific, and global contexts.

## Migration from Old Commands

If you're used to the old command structure, here's the mapping:

| Old Command | New Command |
|-------------|-------------|
| `ddx list` | `ddx templates list`, `ddx prompts list`, etc. |
| `ddx list --type prompts` | `ddx prompts list` |
| `ddx apply nextjs` | `ddx templates apply nextjs` |
| `ddx apply error-handling` | `ddx patterns apply error-handling` |

## Tab Completion

DDx supports tab completion for all shells:

```bash
# Bash
source <(ddx completion bash)

# Zsh
source <(ddx completion zsh)

# Fish
ddx completion fish | source
```

With tab completion, you can easily discover commands:
- Type `ddx ` and press Tab to see all resources
- Type `ddx prompts ` and press Tab to see all prompt actions