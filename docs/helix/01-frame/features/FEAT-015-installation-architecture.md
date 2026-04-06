---
ddx:
  id: FEAT-015
  depends_on:
    - FEAT-001
    - FEAT-009
    - FEAT-011
---
# Feature: DDx Installation Architecture

**ID:** FEAT-015
**Status:** In Progress
**Priority:** P0
**Owner:** DDx Team

## Overview

Redesign the DDx installation architecture with a clean separation of concerns:
- **install.sh** — binary only (minimal attack surface, fast)
- **ddx install --global** — extract embedded skills to `~/.ddx/`, symlink to `~/.agents/` and `~/.claude/`
- **ddx init** — project-local skills copied (not symlinked) into `.ddx/skills/`, `.agents/skills/`, `.claude/skills/`
- **ddx install \<plugin\>** — project-scoped: plugin resources to `.ddx/plugins/`, skills to `.agents/` and `.claude/` via relative symlinks

## Problem Statement

Current behavior:
- `install.sh` does too much (creates `~/.ddx/`, sets up symlinks)
- `ddx install helix` clones to user home (`~/.ddx/plugins/`), not project-scoped
- Symlinks aren't tracked by git, so project-local `.agents/skills → .ddx/skills` breaks on clone
- No separation between global installation and project-scoped plugin management

Desired behavior:
- `install.sh` does one thing: get the binary into PATH
- `ddx install --global` owns global skill setup (home directory)
- `ddx init` copies bootstrap skills as real files (git-trackable)
- `ddx install <plugin>` is project-scoped, uses relative symlinks for `.agents/` and `.claude/`

## Requirements

### Functional

#### install.sh (curl script)

1. **Binary-Only Installation**
   - Downloads `ddx` binary to `~/.local/bin/ddx`
   - Sets up PATH in shell rc file
   - Sets up shell completions
   - Does NOT create `~/.ddx/`, `~/.agents/`, or `~/.claude/`

#### Global Installation (`ddx install --global`)

2. **DDx Skills Extraction**
   - Extracts embedded skills (ddx-bead, ddx-agent, ddx-install, ddx-status, ddx-review, ddx-run) to `~/.ddx/skills/`
   - Creates `~/.agents/skills/ddx-*` symlinks → `~/.ddx/skills/ddx-*`
   - Creates `~/.claude/skills/ddx-*` symlinks → `~/.agents/skills/ddx-*`

#### Repository Initialization (`ddx init`)

3. **Project Structure Creation**
   - Creates `.ddx/` directory with config.yaml and library structure
   - Copies bootstrap skills (ddx-doctor, ddx-run) as **real files** to `.ddx/skills/`
   - Copies bootstrap skills to `.agents/skills/` and `.claude/skills/` as **real files**
   - All files are git-trackable (no symlinks for project-local skills)

4. **Pre-flight Check**
   - Verify `ddx` binary exists in PATH
   - If missing: warn user, suggest running install.sh

#### Plugin Installation (`ddx install <plugin>`)

5. **Project-Scoped Plugin Install**
   - Default: downloads released tarball from plugin's GitHub releases
   - Extracts to `$PROJECT/.ddx/plugins/<name>/`
   - Creates relative symlinks from `.agents/skills/<skill>` → `.ddx/plugins/<name>/.agents/skills/<skill>`
   - Creates relative symlinks from `.claude/skills/<skill>` → `.agents/skills/<skill>`
   - Relative symlinks work across clone/checkout (no absolute paths)
   - Fallback: `ddx install <plugin> --from-source` clones repo (for developers working on the plugin)

6. **Plugin Manifest**
   - Records installed plugins in `.ddx/plugins.yaml` or similar
   - Tracks name, version, install source (release vs source), install date
   - Enables `ddx installed` to show project-scoped plugins
   - Enables `ddx outdated` to check for newer released versions

#### Version Tracking & Staleness Detection

9. **Project Version Stamp (`.ddx/versions.yaml`)**
   - System-managed file, separate from user config. Users never edit.
   - Written by `ddx init`, committed to git alongside config.yaml
   - Contains single field: `ddx_version` — the binary version that initialized/last updated the project
   - Schema:
     ```yaml
     ddx_version: "0.3.0"
     ```

10. **Version Gate (pre-run, every command)**
    - If `.ddx/versions.yaml` does not exist → skip (not a DDx project, or pre-versioning)
    - If binary version is `"dev"` → skip (development builds bypass gate)
    - If binary version < project's `ddx_version` → **hard error, block execution:**
      ```
      Error: This project requires DDx v0.4.0 or newer (you have v0.3.0).
      Run 'ddx upgrade' to update your DDx binary.
      ```
    - Exempt commands: `upgrade`, `version`, `doctor`, `init` (must work even when binary is too old)
    - Runs in `PersistentPreRunE`, after config init, before update check
    - Pure string compare — no network, no disk beyond the config read

11. **Staleness Hints (post-run, every command)**
    - If binary version > project's `ddx_version` → soft hint:
      ```
      💡 Project skills from DDx v0.3.0 (you have v0.4.0). Run 'ddx init --force' to update.
      ```
    - Plugin staleness: compare `~/.ddx/installed.yaml` entries vs `BuiltinRegistry()` → soft hint:
      ```
      💡 helix 1.0.0 installed, 2.0.0 available. Run 'ddx install helix' to update.
      ```
    - Runs in `PersistentPostRunE`, after existing update-available banner
    - Pure local comparisons — no network

12. **Force Refresh (`ddx init --force`)**
    - Overwrites `.ddx/versions.yaml` with current binary version
    - Overwrites existing skill files (currently `registerProjectSkills` skips existing files — must add `force` parameter)
    - Preserves user config in `.ddx/config.yaml`

#### Updates

7. **Binary Update (`ddx upgrade`)**
   - Checks GitHub for new ddx release (async, 24h cache via `~/.cache/ddx/last-update-check.json`)
   - Downloads and replaces binary
   - After upgrade, next command in project shows staleness hint (correct: new binary > old `ddx_version`)

8. **Plugin Update (`ddx update <plugin>`)**
   - Checks plugin's GitHub releases for newer version
   - Downloads new release tarball to `.ddx/plugins/<name>/`
   - Re-establishes relative symlinks
   - `ddx update <plugin> --from-source` re-clones from repo HEAD

### Non-Functional

- **No Repo Bloat:** plugins live in `.ddx/plugins/` (gitignored or committed per user preference)
- **Git-Trackable Skills:** `ddx init` copies real files, not symlinks
- **Git-Trackable Versions:** `.ddx/versions.yaml` committed to git — teammates get version gate on clone
- **Relative Symlinks for Plugins:** work across machines, no absolute paths
- **No Windows Targets:** relative symlinks are acceptable
- **Offline-First:** bootstrap skills work without network; version gate is local-only
- **Idempotent:** multiple runs of same command produce same result
- **Separation of Concerns:** `.ddx/config.yaml` for user preferences, `.ddx/versions.yaml` for system-managed state, `~/.ddx/installed.yaml` for global plugin state

## Architecture

### Directory Structure

```
# Global (via ddx install --global)
~/.ddx/
├── skills/
│   ├── ddx-bead/
│   ├── ddx-agent/
│   ├── ddx-install/
│   ├── ddx-status/
│   ├── ddx-review/
│   └── ddx-run/
└── config.yaml

~/.agents/skills/
├── ddx-bead/ → ~/.ddx/skills/ddx-bead/
├── ddx-agent/ → ~/.ddx/skills/ddx-agent/
└── ...

~/.claude/skills/
├── ddx-bead/ → ~/.agents/skills/ddx-bead/
└── ...

# Project (via ddx init + ddx install helix)
project/
├── .ddx/
│   ├── config.yaml       (user preferences)
│   ├── versions.yaml     (system-managed: ddx_version)
│   ├── library/
│   ├── skills/
│   │   ├── ddx-doctor/   (real files, git-tracked)
│   │   └── ddx-run/      (real files, git-tracked)
│   └── plugins/
│       └── helix/        (cloned plugin)
│           └── .agents/skills/
│               ├── helix-align/
│               ├── helix-build/
│               └── ...
├── .agents/skills/
│   ├── ddx-doctor/       (real files, copied by ddx init)
│   ├── ddx-run/          (real files, copied by ddx init)
│   ├── helix-align/ → ../.ddx/plugins/helix/.agents/skills/helix-align
│   ├── helix-build/ → ../.ddx/plugins/helix/.agents/skills/helix-build
│   └── ...
├── .claude/skills/
│   ├── ddx-doctor/ → ../.agents/skills/ddx-doctor
│   ├── helix-align/ → ../.agents/skills/helix-align
│   └── ...
└── ...
```

### Command Matrix

| Command | Scope | What It Does |
|---------|-------|--------------|
| `curl install.sh \| bash` | Global | Binary to `~/.local/bin/ddx` + PATH |
| `ddx install --global` | Global | Extract skills to `~/.ddx/`, symlink `~/.agents/`, `~/.claude/` |
| `ddx init` | Project | `.ddx/` structure + copy bootstrap skills to `.agents/`, `.claude/` |
| `ddx install helix` | Project | Clone to `.ddx/plugins/helix/`, relative symlinks in `.agents/`, `.claude/` |
| `ddx upgrade` | Global | Update binary |
| `ddx update <plugin>` | Project | Re-clone plugin, re-establish symlinks |

### Key Design Decisions

1. **Copy over symlink for ddx init**: Git doesn't track symlinks well. Bootstrap skills must survive `git clone` on a fresh machine.

2. **Relative symlinks for plugins**: Plugin skills are installed via relative symlinks (e.g., `../.ddx/plugins/helix/.agents/skills/helix-align`). This works across machines without absolute paths. Acceptable since we're not targeting Windows.

3. **Project-scoped plugins**: Plugins install to the project, not globally. This lets different projects use different plugin versions and makes the project self-contained.

4. **Minimal install.sh**: The curl script does one thing (install binary). Everything else is handled by `ddx` commands that have proper error handling, embedded assets, and testability.

## Out of Scope

- Windows-specific installation (future)
- System package manager integration (apt, brew, etc.) (future)
- Plugin publishing to registry (future)
- Global plugin installation (future — currently project-scoped only)

## Acceptance Criteria

### AC-001: Clean Machine Installation
```bash
# In Docker container with nothing installed
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
which ddx            # → ~/.local/bin/ddx
ddx version          # → shows version
ls ~/.ddx/ 2>&1      # → no such directory (install.sh doesn't create it)
```

### AC-002: Global Skill Installation
```bash
# After AC-001
ddx install --global
ls ~/.ddx/skills/ddx-bead/   # → skill files exist
ls ~/.agents/skills/ddx-bead # → symlink to ~/.ddx/skills/ddx-bead
ls ~/.claude/skills/ddx-bead # → symlink to ~/.agents/skills/ddx-bead
```

### AC-003: Repository Initialization
```bash
# In empty project directory
ddx init
ls .ddx/skills/ddx-doctor/   # → real files (not symlinks)
ls .agents/skills/ddx-doctor/ # → real files (copied, not symlinked)
ls .claude/skills/ddx-doctor/ # → real files or relative symlink to .agents
git add .agents/ .claude/     # → works (real files tracked by git)
```

### AC-004: Plugin Installation (Project-Scoped)
```bash
# In initialized project
ddx install helix
ls .ddx/plugins/helix/                        # → plugin cloned
readlink .agents/skills/helix-align           # → ../.ddx/plugins/helix/.agents/skills/helix-align
readlink .claude/skills/helix-align           # → ../.agents/skills/helix-align
```

### AC-005: Missing DDx Detection
```bash
# Clone a project with .ddx/skills/ddx-doctor/ but no ddx binary
# ddx-doctor skill detects missing binary and prompts install
```

### AC-006: Version Tracking
```bash
# ddx init writes versions.yaml
ddx init
cat .ddx/versions.yaml  # → ddx_version: "0.3.0" (current binary version)
git log --oneline -1     # → commit includes .ddx/versions.yaml
```

### AC-007: Version Gate (binary too old)
```bash
# Simulate: edit versions.yaml to require newer version
echo 'ddx_version: "99.0.0"' > .ddx/versions.yaml
ddx bead list            # → Error: This project requires DDx v99.0.0 or newer...
ddx version              # → works (exempt command)
ddx upgrade              # → works (exempt command)
```

### AC-008: Staleness Hint (binary newer)
```bash
# Simulate: edit versions.yaml to older version
echo 'ddx_version: "0.0.1"' > .ddx/versions.yaml
ddx bead list            # → normal output + hint: "💡 Project skills from DDx v0.0.1..."
```

### AC-009: Force Refresh
```bash
# After staleness hint
ddx init --force
cat .ddx/versions.yaml   # → ddx_version updated to current
cat .agents/skills/ddx-doctor/SKILL.md  # → overwritten with latest
```

### AC-010: Dev Build Bypass
```bash
# Dev build (version="dev") should not trigger gate
# Even if versions.yaml says v99.0.0
ddx bead list            # → works normally, no error
```

### AC-011: Docker Test Coverage
All above scenarios run in Docker containers:
- `tests/docker/Dockerfile.clean` — minimal image for AC-001
- `tests/docker/Dockerfile.with-ddx` — pre-installed for AC-002, AC-003, AC-004
- `tests/docker/Dockerfile.no-binary` — ddx removed for AC-005
