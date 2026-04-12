# MCP Server Registry

This directory contains the registry of MCP (Model Context Protocol) servers available for installation through DDx.

## Structure

```
mcp-servers/
├── registry.yml          # Main registry index
├── servers/             # Individual server definitions
│   ├── github.yml
│   ├── postgres.yml
│   └── ...
└── categories.yml       # Category definitions
```

## Adding a New Server

To add a new MCP server to the registry:

1. Create a new YAML file in `servers/` directory
2. Follow the schema defined below
3. Add an entry to `registry.yml`
4. Submit a pull request

## Server Definition Schema

```yaml
name: string                    # Unique server identifier
description: string             # Brief description
category: string               # One of: development, database, filesystem, utility, productivity
author: string                 # Server author/maintainer
version: string                # Server version (semver)
tags: [string]                 # Searchable tags

command:
  executable: string           # Command to run (usually "npx")
  args: [string]              # Command arguments

environment:                   # Required environment variables
  - name: string              # Variable name
    description: string       # What it's for
    required: boolean         # Is it mandatory?
    sensitive: boolean        # Should it be masked?
    validation: string        # Regex pattern for validation
    default: string           # Default value (optional)
    example: string           # Example value (optional)

documentation:
  setup: string               # Setup instructions
  permissions: [string]       # Required permissions
  examples: [string]          # Usage examples
  security_notes: string      # Security considerations

compatibility:
  platforms: [string]         # Supported OS: darwin, linux, windows
  claude_versions: [string]  # Supported Claude: desktop, code
  min_ddx_version: string     # Minimum DDx version

security:
  sandbox: string             # Sandboxing level: required, recommended, optional
  network_access: string      # Network needs: required, optional, none
  file_access: string         # File access: required, optional, none
  data_handling: string       # How sensitive data is handled
  warnings: [string]          # Security warnings

verification:
  test_command: string        # Command to verify installation
  expected_response: string   # What to expect

links:
  homepage: string            # Project homepage
  documentation: string       # Documentation URL
  issues: string             # Issue tracker
```

## Categories

- **development**: Version control, code management, CI/CD
- **database**: Database connections and management
- **filesystem**: File and directory operations
- **utility**: General purpose tools
- **productivity**: Collaboration and productivity tools

## Security Guidelines

1. **Sensitive Data**: Always mark credentials as `sensitive: true`
2. **Validation**: Provide regex patterns for environment variables
3. **Sandboxing**: Specify appropriate sandbox level
4. **Warnings**: Document any security risks clearly
5. **Permissions**: List minimum required permissions

## Testing a Server Definition

```bash
# Validate YAML syntax
yamllint servers/myserver.yml

# Test with DDx
ddx mcp list --search myserver
ddx mcp install myserver --dry-run
```

## Contributing

We welcome contributions! Please:

1. Follow the schema exactly
2. Test your server definition
3. Include clear documentation
4. Add security warnings where appropriate
5. Submit via pull request

## License

Server definitions in this registry are provided as-is for use with DDx.
Individual MCP servers may have their own licenses.