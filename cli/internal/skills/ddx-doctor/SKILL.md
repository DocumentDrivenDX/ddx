---
skill:
  name: ddx-doctor
  description: Verify DDx is installed and working, or guide the user to install it.
---

# DDx Doctor: Installation Verification

Check if DDx is properly installed and provide guidance if it's missing.

## When to Use

- You need to verify DDx is available before running DDx commands
- DDx commands fail because the binary is not found
- First time setup in a new project

## Steps

### 1. Check if ddx binary exists

```bash
which ddx
```

If this returns a path, skip to step 3.

### 2. If ddx not found

Output a clear message to the user:

```
DDx is not installed or not in PATH.

To install DDx, run:

    curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash

Or install manually:
    1. Download the latest release from https://github.com/DocumentDrivenDX/ddx/releases
    2. Extract and place the ddx binary in your PATH (e.g., ~/.local/bin/)
    3. Restart your terminal or Claude Code session

After installation, verify with: ddx version
```

### 3. If ddx found, verify it works

```bash
ddx version
ddx doctor
```

If these fail, the installation may be corrupted. Recommend reinstalling.

### 4. Verify project initialization

If ddx works, check if the current project is initialized:

```bash
ls -la .ddx/
```

If `.ddx/` doesn't exist, suggest running:

```bash
ddx init
```

## Common Issues

| Issue | Solution |
|-------|----------|
| `ddx: command not found` | Add `~/.local/bin` to PATH, or reinstall |
| `ddx version` fails | Binary may be corrupted; reinstall |
| `.ddx/` not found in project | Run `ddx init` in the project directory |

## References

- Installation: https://github.com/DocumentDrivenDX/ddx
- Releases: https://github.com/DocumentDrivenDX/ddx/releases
- DDx CLI docs: `ddx --help`
