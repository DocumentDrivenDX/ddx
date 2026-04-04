---
title: CLI Reference
weight: 3
---

Complete command reference for the `ddx` CLI.

## Foundation Commands

### `ddx init`

Initialize a DDx document library in your project.

```bash
ddx init                    # Interactive initialization
ddx init --no-git           # Skip git subtree setup
ddx init --force            # Reinitialize existing project
```

Creates `.ddx/config.yaml` and `.ddx/library/` structure.

### `ddx list`

Browse available documents in your library.

```bash
ddx list                    # All document categories
ddx list prompts            # Only prompts
ddx list templates          # Only templates
ddx list --json             # JSON output
ddx list --filter react     # Search by name
```

### `ddx doctor`

Validate your DDx installation and library health.

```bash
ddx doctor                  # Run all checks
ddx doctor --verbose        # Detailed output
```

Checks: binary, PATH, git, library structure, config validity.

### `ddx update`

Pull latest documents from the upstream repository.

```bash
ddx update                  # Pull latest changes
ddx update --check          # Check without applying
ddx update --dry-run        # Preview changes
```

### `ddx contribute`

Share your document improvements back to the upstream repository.

```bash
ddx contribute -m "Improved error handling pattern"
ddx contribute --dry-run    # Preview what would be shared
```

### `ddx upgrade`

Upgrade the DDx binary to the latest release.

```bash
ddx upgrade                 # Upgrade to latest
ddx upgrade --check         # Check for updates only
```

### `ddx status`

Show version and sync status.

```bash
ddx status                  # Basic status
ddx status --changes        # List modified files
ddx status --diff           # Show differences
```

### `ddx log`

Show DDx asset history.

```bash
ddx log                     # Recent history
ddx log -n 10               # Last 10 commits
ddx log --oneline           # Compact format
```

## Document Commands

### Prompts

```bash
ddx prompts list            # List available prompts
ddx prompts show <name>     # Display prompt content
```

### Templates

```bash
ddx templates list          # List available templates
ddx templates apply <name>  # Apply template to project
```

### Personas

```bash
ddx persona list            # List available personas
ddx persona show <name>     # View persona definition
ddx persona bind <role> <name>  # Bind persona to role
```

### MCP Servers

```bash
ddx mcp list                # List available MCP servers
ddx mcp install <name>      # Install MCP server
ddx mcp --status            # Show installed servers
```

## Configuration

```bash
ddx config                  # Show help
ddx config set <key> <val>  # Set a value
ddx config get <key>        # Get a value
ddx config --validate       # Validate config
```

## Global Options

| Flag | Description |
|------|------------|
| `--verbose`, `-v` | Verbose output |
| `--config` | Config file path |
| `--library-base-path` | Override library location |
| `--help` | Show help |
