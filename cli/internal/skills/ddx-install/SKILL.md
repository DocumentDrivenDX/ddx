---
skill:
  name: ddx-install
  description: Search, install, verify, and uninstall DDx packages.
  args:
    - name: package
      description: Package name or search query
      required: false
---

# DDx Install: Manage Packages

DDx packages extend the platform with additional templates, prompts, personas,
MCP servers, and tool configurations. This skill guides you through finding and
installing packages safely.

## When to Use

- Adding a new template, persona, or MCP server to a project
- Finding available packages that match a need
- Verifying what packages are currently installed
- Cleaning up packages that are no longer needed

## Steps

### 1. Search for Packages

```bash
ddx search <query>
```

Returns matching packages with names, types, and descriptions. Narrow your
query if results are too broad.

### 2. Review Package Details

Before installing, inspect what the package provides:

```bash
ddx search <name> --detail
```

Confirm the package matches your need and note any prerequisites or
configuration it requires.

### 3. Install the Package

```bash
ddx install <name>
```

DDx will fetch the package from the library and place its resources in the
appropriate location under `.ddx/`.

### 4. Verify Installation

```bash
ddx installed
```

Lists all currently installed packages. Confirm the new package appears and
check that dependent resources are accessible.

For resource-specific verification:
```bash
ddx prompts list      # Verify prompt packages
ddx persona list      # Verify persona packages
ddx templates list    # Verify template packages
ddx mcp list          # Verify MCP server packages
```

### 5. Uninstall if Needed

```bash
ddx uninstall <name>
```

Removes the package and its resources. Run `ddx installed` afterward to
confirm removal.

## References

- Full flag list: `ddx install --help`, `ddx search --help`
- CLI feature spec: `docs/helix/01-frame/features/FEAT-001-cli.md`
