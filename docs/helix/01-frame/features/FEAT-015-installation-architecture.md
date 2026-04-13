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

3a. **Bootstrap Skill Cleanup (Stale ddx-* Removal)**
   - Before copying bootstrap skills, scans each target directory (`.ddx/skills/`, `.agents/skills/`, `.claude/skills/`) for existing `ddx-*` subdirectories
   - Any `ddx-*` directory containing a `SKILL.md` that is **not** in the current bootstrap allowlist (`ddx-doctor`, `ddx-run`) is removed
   - Purpose: removes skills from older DDx versions that are no longer part of the bootstrap set
   - Only removes `ddx-*` prefixed directories; plugin skills (e.g., `helix-*`) are never touched
   - Silent: no user-visible output on cleanup; errors are ignored (non-fatal)
   - Runs on every `ddx init` invocation, including `ddx init --force`

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

5a. **Plugin Skill Stale Link Pruning**
   - Before creating new skill symlinks, scans the target skills directory for existing entries not in the plugin's current skill list
   - Removes a symlink only if **both** conditions hold: (a) it is a symlink (not a real file), and (b) its resolved target is within the plugin's installed root (`.ddx/plugins/<name>/`)
   - Purpose: removes symlinks for skills that were removed in a plugin update
   - Real files (e.g., bootstrap skills copied by `ddx init`) are never removed by this step
   - Symlinks from other plugins (pointing outside this plugin's root) are never removed
   - Runs during every `ddx install <plugin>` invocation

5b. **Stale Install File Removal**
   - The plugin registry tracks the set of files written by each install
   - On re-install (update), files from the previous install that are absent from the new install's file list are removed
   - Applies to any file tracked in the plugin registry entry (e.g., extracted tarball contents)
   - Runs during `ddx install <plugin>` when a prior install record exists

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
   - Uses GitHub `releases/latest`, so prereleases do not trigger normal update detection unless intentionally promoted as the latest release
   - Version comparison must treat any hyphenated prerelease suffix as older than the matching stable release, with channel ordering `alpha < beta < rc < stable` and numeric ordering within the same channel (`rc2 < rc10`)
   - Downloads and replaces binary
   - After upgrade, next command in project shows staleness hint (correct: new binary > old `ddx_version`)
   - Dogfood installs of prereleases remain possible via explicit version selection, e.g. `DDX_VERSION=v0.3.0-rc1 curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash`

8. **Plugin Update (`ddx update <plugin>`)**
   - Checks plugin's GitHub releases for newer version
   - Downloads new release tarball to `.ddx/plugins/<name>/`
   - Re-establishes relative symlinks
   - `ddx update <plugin> --from-source` re-clones from repo HEAD

### Non-Functional

- **No Repo Bloat:** plugins live in `.ddx/plugins/` (gitignored or committed per user preference)
- **Git-Trackable Skills:** `ddx init` copies real files, not symlinks
- **Git-Trackable Versions:** `.ddx/versions.yaml` committed to git — teammates get version gate on clone
- **Git-Trackable Execution Evidence:** `.ddx/executions/<attempt-id>/` is
  the tracked execute-bead attempt bundle defined in FEAT-006 §"Execute-Bead
  Evidence Bundle". `ddx init` and any DDx-managed `.gitignore` template
  must leave `.ddx/executions/` trackable so bundles committed by
  `execute-bead` survive clones. Only the ignored runtime scratch paths
  listed in FEAT-006 (`.ddx/exec-runs.d/`, `.ddx/agent-logs/`,
  `.ddx/.execute-bead-wt-*/`) may be excluded from tracking.
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
│   ├── executions/       (tracked execute-bead attempt bundles; see FEAT-006)
│   │   └── <attempt-id>/ (prompt.md, manifest.json, result.json, ...)
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

### AC-012: Bootstrap Skill Cleanup on `ddx init`
```bash
# Simulate a stale bootstrap skill from an older DDx version
mkdir -p .agents/skills/ddx-old-skill
echo "---" > .agents/skills/ddx-old-skill/SKILL.md
ddx init
ls .agents/skills/ddx-old-skill 2>&1  # → no such file or directory (removed)
ls .agents/skills/ddx-doctor/         # → present (in bootstrap allowlist)
ls .agents/skills/ddx-run/            # → present (in bootstrap allowlist)
# Plugin skills are untouched
ls .agents/skills/helix-align 2>&1    # → unchanged (not a ddx-* prefix)
```

### AC-013: Plugin Skill Stale Link Pruning on `ddx install`
```bash
# Install plugin, then re-install after a skill is removed upstream
ddx install helix                         # installs helix-align, helix-build
# Simulate helix-build removed in new version (re-install with fewer skills)
ddx install helix
ls .agents/skills/helix-build 2>&1        # → no such file or directory (stale link removed)
ls .agents/skills/helix-align             # → symlink present (still in plugin)
# Bootstrap skills are not removed
ls .agents/skills/ddx-doctor/             # → unchanged (real file, not symlink)
```

### AC-014: Stale Install File Removal on Plugin Update
```bash
# Install plugin, verify old files removed on update
ddx install helix@1.0.0
# Files from 1.0.0 that don't exist in 1.1.0 are removed when updating
ddx install helix@1.1.0
# Files only in 1.0.0 are gone; files in 1.1.0 are present
```
