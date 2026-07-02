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

> **Update 2026-05-12 (single forward install model):** This feature is
> amended to remove all global/home plugin and skill installation behavior.
> The only DDx-managed file under a user's home directory is the DDx binary at
> `~/.local/bin/ddx` (plus shell PATH/completion setup when performed by the
> binary installer). DDx must not create or manage `~/.ddx`, `~/.agents`, or
> `~/.claude`.
>
> Plugin lifecycle commands are project-scoped and live under the `plugin`
> noun: `ddx plugin install`, `ddx plugin list`, `ddx plugin upgrade`, and
> `ddx plugin uninstall`. Top-level `ddx install <plugin>`, `ddx update
> <plugin>`, `ddx installed`, `ddx uninstall <plugin>`, `ddx outdated`,
> `ddx verify`, and generic `ddx update` are not part of the forward public
> interface.
>
> Registry plugin installs write real project files for clone portability.
> Local developer plugin installs use symlinks intentionally: `.ddx/plugins/<name>`
> links to the local checkout, and `.agents/skills/<skill>` plus
> `.claude/skills/<skill>` link directly to that checkout's skill directory.
> Local overlays never update the plugin version pin and never auto-commit.
>
> **Update 2026-04-17:** The skill roster referenced throughout this
> document (ddx-bead, ddx-agent, ddx-install, ddx-status, ddx-review,
> ddx-run, ddx-doctor) reflects the pre-consolidation layout. Per
> FEAT-011, DDx now ships a **single portable `ddx` skill** with
> `SKILL.md` + `reference/*.md` (progressive disclosure, agentskills.io
> standard). The installation flow described below is still accurate
> in structure — binary separate from library, `ddx init` copies skills
> as real files, `ddx plugin install <plugin>` adds plugin-scoped content —
> but any reference to the 7-skill roster should be read as historical
> context. Sections that specifically describe the bootstrap
> allowlist are now ["ddx"] instead of ["ddx-doctor", "ddx-run"], and
> the stale-skill cleanup removes all old ddx-prefixed dirs (ddx-bead,
> ddx-run, ddx-agent, ddx-review, ddx-status, ddx-doctor, ddx-install,
> ddx-release) on init and update. See FEAT-011 for the current skill
> architecture.

## Overview

Redesign the DDx installation architecture with a clean separation of concerns:

- **install.sh** — binary only: installs `ddx` to `~/.local/bin/ddx` and may
  perform shell PATH/completion setup. It creates no DDx state directories.
- **ddx upgrade** — binary only: upgrades `~/.local/bin/ddx`. It never mutates
  project files, plugins, skills, beads, or docs.
- **ddx init** — project bootstrap only: writes `<projectRoot>/.ddx/`,
  `<projectRoot>/.agents/skills/`, `<projectRoot>/.claude/skills/`,
  `AGENTS.md`, and `CLAUDE.md` as project files.
- **ddx plugin install <name>** — registry plugin install: writes real project
  files under `<projectRoot>/.ddx/plugins/<name>/`, installs plugin skill
  project files, and records plugin state in `<projectRoot>/.ddx/plugins.yaml`.
- **ddx plugin install <name> --local <path>** — developer overlay: symlinks
  project-local plugin and skill paths to the local checkout. It never updates
  the registry version pin and never auto-commits.
- **ddx plugin list / upgrade / uninstall** — project plugin lifecycle only.

DDx does not install skills, plugins, or plugin state into `$HOME`.

## Scope Invariant

**Machine scope:** `install.sh` and `ddx upgrade` may write only the DDx binary
to `~/.local/bin/ddx` and installer-owned shell integration. They must not
write `~/.ddx`, `~/.agents`, or `~/.claude`.

**Project scope:** DDx-managed project content lives under the repository:
`.ddx/` (plugins, config, versions), `.agents/skills/`, `.claude/skills/` (installer outputs),
`AGENTS.md`, and `CLAUDE.md`.

**Registry plugin installs:** install real files for clone portability and
cross-platform safety.

**Local plugin overlays:** intentionally use symlinks because they are
machine-local developer state. Local overlays are detected by
`.ddx/plugins/<name>` being a symlink.

**Manifest validation:** Plugin manifests whose install targets begin with
`~` are invalid. Targets must be project-relative paths under `<projectRoot>/`.

## No Legacy Home Compatibility

DDx does not migrate or read legacy home plugin state. Old `~/.ddx`,
`~/.agents`, and `~/.claude` layouts are outside the forward contract and may
be removed manually by users.

## Problem Statement

Legacy behavior this feature replaces:
- `install.sh` does too much (creates `~/.ddx/`, sets up symlinks)
- Top-level `ddx install helix` clones to user home (`~/.ddx/plugins/`), not project-scoped
- Symlinks aren't tracked by git, so project-local `.agents/skills → .ddx/skills` breaks on clone
- No separation between global installation and project-scoped plugin management

Desired behavior:
- `install.sh` does one thing: get the binary into PATH
- `ddx upgrade` upgrades only the DDx binary
- `ddx init` manages project bootstrap files
- `ddx plugin install/list/upgrade/uninstall` owns plugin lifecycle
- `ddx plugin install --local` creates a project-local symlink overlay for
  developer iteration

## Requirements

### Functional

#### install.sh (curl script)

1. **Binary-Only Installation**
   - Downloads `ddx` binary to `~/.local/bin/ddx`
   - Installs a prebuilt local development binary with
     `./install.sh --from-build [path]`, defaulting to `cli/build/ddx`
   - Sets up PATH in shell rc file
   - Sets up shell completions
   - Does NOT create `~/.ddx/`, `~/.agents/`, or `~/.claude/`

#### Repository Initialization (`ddx init`)

3. **Project Structure Creation**
   - Creates `.ddx/` directory with config.yaml, library structure, and versions.yaml
   - Installs the default `ddx` plugin through the embedded package installer
   - Produces `.agents/skills/ddx/` and `.claude/skills/ddx/` as **real files** (no symlinks)
   - All files are git-trackable for project portability

3a. **Bootstrap Skill Cleanup (Stale ddx-* Removal)**
   - Before copying bootstrap skills, scans each target directory (`.ddx/skills/`, `.agents/skills/`, `.claude/skills/`) for existing `ddx-*` subdirectories
   - Any `ddx-*` directory containing a `SKILL.md` that is **not** in the current bootstrap allowlist (`ddx`) is removed
   - Purpose: removes skills from older DDx versions that are no longer part of the bootstrap set
   - Only removes `ddx-*` prefixed directories; plugin skills (e.g., `helix-*`) are never touched
   - Silent: no user-visible output on cleanup; errors are ignored (non-fatal)
   - Runs on every `ddx init` invocation, including `ddx init --force`

4. **Pre-flight Check**
   - Verify `ddx` binary exists in PATH
   - If missing: warn user, suggest running install.sh

#### Plugin Installation (`ddx plugin install <plugin>`)

5. **Project-Scoped Plugin Install**
   - Default: downloads released tarball from plugin's GitHub releases
   - Extracts real files to `$PROJECT/.ddx/plugins/<name>/`
   - Copies plugin skills as project files into `.agents/skills/<skill>` and
     `.claude/skills/<skill>`
   - Records project-local plugin state in `.ddx/plugins.yaml`
   - `ddx install <plugin>` is not the forward public interface and is not
     retained as a compatibility alias

5a. **Local Plugin Overlay (`ddx plugin install <plugin> --local <path>`)**
   - Validates the local plugin checkout before writing project paths
   - Replaces `.ddx/plugins/<name>` with a symlink to the absolute local
     checkout path
   - Replaces `.agents/skills/<skill>` and `.claude/skills/<skill>` with direct
     symlinks to the local checkout skill directory
   - Does not update `.ddx/plugins.yaml` version pin/state
   - Never auto-commits
   - Local overlay detection is `os.Lstat(".ddx/plugins/<name>")` returning a
     symlink

5b. **Plugin Skill Stale Entry Pruning**
   - Before installing release skills, scans plugin-owned target skill entries
     and removes entries absent from the new plugin release
   - Removes only entries owned by the plugin being installed
   - Real bootstrap skills and other plugins' skills are never removed

5c. **Stale Install File Removal**
   - The plugin registry tracks the set of files written by each install
   - On registry reinstall or plugin upgrade, files from the previous install
     that are absent from the new install's file list are removed
   - Cleanup must use `os.Lstat` and must not follow symlinked plugin roots into
     a developer checkout

6. **Plugin Manifest**
   - Records registry-installed plugins in `.ddx/plugins.yaml`
   - Tracks name, version, install source (release vs source), install date
   - Enables `ddx plugin list` to show project-scoped plugins
   - Enables `ddx plugin upgrade` to check for newer released versions
   - Local overlays are discovered from symlinked `.ddx/plugins/<name>` paths
     and shown as `local` even when no state entry exists

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
    - Plugin staleness: compare `.ddx/plugins.yaml` entries vs `BuiltinRegistry()` → soft hint:
      ```
      💡 helix 1.0.0 installed, 2.0.0 available. Run 'ddx plugin upgrade helix' to update.
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

8. **Plugin Upgrade (`ddx plugin upgrade [plugin]`)**
   - Checks plugin's GitHub releases for newer version
   - With a plugin name, upgrades that one registry-installed plugin
   - Without a plugin name, upgrades all registry-installed plugins
   - Downloads new release tarball to `.ddx/plugins/<name>/` as real files
   - Reinstalls plugin skills as project files
   - Skips symlinked local overlays with explicit output:
     `helix is local-linked; skipped`

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
- **Local Symlinks Only:** symlinks are used only for `ddx plugin install --local`
- **Offline-First:** bootstrap skills work without network; version gate is local-only
- **Idempotent:** multiple runs of same command produce same result
- **Separation of Concerns:** `.ddx/config.yaml` for user preferences,
  `.ddx/versions.yaml` for DDx binary compatibility, `.ddx/plugins.yaml` for
  project-local plugin state

## Architecture

### Directory Structure

```
# Machine (via install.sh / ddx upgrade)
~/.local/bin/ddx

# Project (via ddx init + ddx plugin install helix)
project/
├── .ddx/
│   ├── config.yaml       (user preferences)
│   ├── versions.yaml     (system-managed: ddx_version)
│   ├── plugins.yaml      (project plugin state)
│   ├── library/
│   ├── executions/       (tracked execute-bead attempt bundles; see FEAT-006)
│   │   └── <attempt-id>/ (prompt.md, manifest.json, result.json, ...)
│   └── plugins/
│       └── helix/        (registry install: real files; local install: symlink)
│           └── skills/helix/
├── .agents/skills/
│   ├── ddx/              (real files, installed by package installer)
│   └── helix/            (registry: real files; local: symlink to checkout)
├── .claude/skills/
│   ├── ddx/              (real files, installed by package installer)
│   └── helix/            (registry: real files; local: symlink to checkout)
└── ...
```

### Command Matrix

| Command | Scope | What It Does |
|---------|-------|--------------|
| `curl install.sh \| bash` | Machine | Binary to `~/.local/bin/ddx` + PATH/completions |
| `ddx upgrade` | Machine | Upgrade only the DDx binary |
| `ddx init [--force]` | Project | `.ddx/` structure + built-in `ddx` skill + AGENTS/CLAUDE guidance |
| `ddx plugin install <name>` | Project | Install registry plugin as real project files |
| `ddx plugin install <name> --local <path>` | Project/dev | Symlink project plugin and skill paths to a local checkout |
| `ddx plugin list` | Project | List project plugins and local overlays |
| `ddx plugin upgrade [name]` | Project | Upgrade one or all registry-installed plugins; skip local overlays |
| `ddx plugin uninstall <name>` | Project | Remove plugin root, plugin skills, and project plugin state |

### Key Design Decisions

1. **Binary-only machine install**: The only DDx-owned home path is
   `~/.local/bin/ddx`. Plugins, skills, and state are project-local.

2. **Copy for project bootstrap and registry plugins**: `ddx init` and
   registry plugin installs write real files so clones and CI are portable.

3. **Symlink only for local plugin overlays**: `ddx plugin install --local`
   symlinks to a developer checkout for live iteration. This is machine-local
   state and is never a registry install artifact.

4. **Project-scoped plugins**: Plugins install to the project, not globally.
   This lets different projects use different plugin versions and keeps plugin
   state in `.ddx/plugins.yaml`.

5. **Noun-owned plugin lifecycle**: `ddx plugin *` owns plugin install, list,
   upgrade, and uninstall. `ddx upgrade` owns the DDx binary.

## Out of Scope

- Windows-specific installation (future)
- System package manager integration (apt, brew, etc.) (future)
- Plugin publishing to registry (future)
- Global plugin or skill installation
- Home-scoped DDx plugin state

## Acceptance Criteria

### AC-001: Clean Machine Installation
```bash
# In Docker container with nothing installed
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | bash
which ddx            # → ~/.local/bin/ddx
ddx version          # → shows version
ls ~/.ddx/ 2>&1      # → no such directory (install.sh doesn't create it)
```

### AC-002: No Home DDx State
```bash
# After AC-001
test ! -e ~/.ddx
test ! -e ~/.agents
test ! -e ~/.claude
```

### AC-003: Repository Initialization
```bash
# In empty project directory
ddx init
ls .agents/skills/ddx/      # → real files (installed by package installer)
ls .claude/skills/ddx/      # → real files (installed by package installer)
git add .agents/ .claude/     # → works (real files tracked by git)
```

### AC-004: Plugin Installation (Project-Scoped)
```bash
# In initialized project
ddx plugin install helix
test -d .ddx/plugins/helix
test ! -L .ddx/plugins/helix
test -f .agents/skills/helix/SKILL.md
test ! -L .agents/skills/helix
test -f .claude/skills/helix/SKILL.md
test ! -L .claude/skills/helix
test -f .ddx/plugins.yaml
```

### AC-004a: Local Plugin Overlay
```bash
# In initialized project with ../helix checked out locally
ddx plugin install helix --local ../helix --force
readlink .ddx/plugins/helix                    # → absolute path to ../helix
readlink .agents/skills/helix                  # → absolute path to ../helix/skills/helix
readlink .claude/skills/helix                  # → absolute path to ../helix/skills/helix
git diff --cached --quiet                      # → local install does not auto-commit
```

### AC-005: Missing DDx Detection
```bash
# Clone a project with .agents/skills/ddx/ and .claude/skills/ddx/ but no ddx binary
# ddx skill guidance detects missing binary and prompts install
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
cat .agents/skills/ddx/SKILL.md  # → overwritten with latest
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
ls .agents/skills/ddx/                # → present (in bootstrap allowlist)
# Plugin skills are untouched
ls .agents/skills/helix 2>&1          # → unchanged (not a ddx-* prefix)
```

### AC-013: Plugin Skill Stale Entry Pruning on `ddx plugin install`
```bash
# Install plugin, then re-install after a skill is removed upstream
ddx plugin install helix
# Simulate a skill removed in a new version (re-install with fewer skills)
ddx plugin install helix --force
ls .agents/skills/removed-skill 2>&1      # → no such file or directory
ls .agents/skills/helix/SKILL.md          # → present
# Bootstrap skills are not removed
ls .agents/skills/ddx/                    # → unchanged
```

### AC-014: Plugin Upgrade
```bash
ddx plugin install helix
ddx plugin upgrade helix
# Files from the prior version that do not exist in the new version are gone
# Files in the new version are present
```

### AC-015: Plugin List
```bash
ddx plugin list
# Output includes name, version or "local", source/path, and status
```

### AC-016: Plugin Upgrade Skips Local Overlays
```bash
ddx plugin install helix --local ../helix --force
ddx plugin upgrade helix
# Output: helix is local-linked; skipped
readlink .ddx/plugins/helix  # → still points at ../helix
```

### AC-017: Plugin Uninstall Removes Overlay Only
```bash
ddx plugin install helix --local ../helix --force
ddx plugin uninstall helix
test ! -e .ddx/plugins/helix
test ! -e .agents/skills/helix
test ! -e .claude/skills/helix
test -d ../helix             # local checkout target is untouched
```
