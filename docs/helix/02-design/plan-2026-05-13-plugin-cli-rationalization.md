---
ddx:
  id: plan-2026-05-13-plugin-cli-rationalization
  depends_on:
    - FEAT-001
    - FEAT-009
    - FEAT-015
    - FEAT-018
---
# Plugin CLI Rationalization

Date: 2026-05-13
Status: Draft

## Summary

DDx has one forward installation model:

- `install.sh` installs only the DDx binary under `~/.local/bin/ddx`.
- `ddx upgrade` upgrades only the DDx binary.
- `ddx init` bootstraps project DDx files and built-in skills.
- `ddx plugin install/list/upgrade/uninstall` owns project plugin lifecycle.
- `ddx plugin install <name> --local <path>` creates project-local symlink
  overlays for developer iteration.

The implementation must remove the old mixed interface rather than preserve it.
There is no legacy home plugin state, no home skill install, and no compatibility
layer for top-level plugin commands.

## Current Implementation Facts

- `cli/cmd/command_factory.go` registers both `ddx plugin` and the old top-level
  `ddx install`, `ddx installed`, `ddx uninstall`, `ddx outdated`, and `ddx verify`
  commands.
- `cli/cmd/install.go` implements plugin install, resource install, local install,
  list, uninstall, verify, and outdated behavior in one file.
- `newPluginCommand` currently reuses `newInstallCommand`, so `ddx plugin install`
  inherits old help text and behavior.
- `cli/internal/registry/state.go` persists install state to
  `~/.ddx/installed.yaml`; this violates FEAT-015.
- `runInstall` auto-commits registry plugin installs through `commitPluginChanges`;
  FEAT-015 only forbids local overlay auto-commit, but the forward interface should
  keep commit policy explicit and consistent with tracker/docs guidance.
- `installLocal` already contains partial symlink-overlay behavior, but it still
  touches installed-state concepts and accepts old top-level invocation paths.
- `cli/internal/registry/installer.go` still supports manifest-declared symlinks
  and script targets that can escape project scope unless every mapping is
  validated.
- `cli/cmd/update.go` performs plugin update semantics under `ddx update`;
  forward plugin upgrade belongs under `ddx plugin upgrade`.
- `cli/cmd/install.go` still exposes root `ddx outdated` and `ddx verify`
  surfaces that read installed package state. These overlap the new plugin
  lifecycle and must be retired or moved under the plugin noun.
- Tests in `cli/cmd/install_manifest_test.go`, `cli/cmd/doctor_plugins_test.go`,
  and related registry tests still assume old state and command names in places.

## Target CLI Contract

| Command | Forward behavior |
|---|---|
| `ddx upgrade [--check] [--force]` | Binary update only. Runs installer path; no project/plugin mutation. |
| `ddx init [--force]` | Project bootstrap only. Writes `.ddx/`, `.agents/skills/ddx`, `.claude/skills/ddx`, `AGENTS.md`, `CLAUDE.md`. |
| `ddx plugin install <name> [--force]` | Registry plugin install as real project files. Records project state in `.ddx/plugins.yaml`. |
| `ddx plugin install <name> --local <path> [--force]` | Local symlink overlay. Replaces `.ddx/plugins/<name>`, `.agents/skills/<skill>`, `.claude/skills/<skill>` with direct symlinks to the checkout. Does not mutate `.ddx/plugins.yaml`. Does not auto-commit. |
| `ddx plugin list [--json]` | Lists registry plugins from `.ddx/plugins.yaml` plus symlink overlays discovered under `.ddx/plugins/`. |
| `ddx plugin upgrade [name] [--force]` | Upgrades one or all registry plugins. Skips local overlays with explicit output. |
| `ddx plugin uninstall <name>` | Removes plugin root and plugin-owned skills. For local overlays, removes only project symlinks and leaves checkout target untouched. |
| `ddx search <query>` | May remain top-level for library discovery, but must not imply top-level plugin install/list/uninstall. |
| `ddx doctor --plugins` | Read-only plugin health and integrity audit. Replaces `ddx verify` for plugin/package integrity checks. |

These commands are not forward public interface and should be removed from root
registration: `ddx install <plugin>`, `ddx installed`, `ddx uninstall <plugin>`,
`ddx outdated`, `ddx verify`,
`ddx update <plugin>`, and package-update semantics under `ddx upgrade <name>`.

`ddx update` is also removed. Its old responsibilities have explicit homes:
`ddx upgrade` for the binary, `ddx init --force` for shipped skills and
project bootstrap refresh, and `ddx plugin upgrade` for plugins. Keeping
`update` as a generic verb reintroduces the ambiguity this plan removes.

## Superfluous Command Disposition

| Current command | Disposition | Replacement |
|---|---|---|
| `ddx install <plugin>` | Remove | `ddx plugin install <plugin>` |
| `ddx installed` | Remove | `ddx plugin list` |
| `ddx uninstall <plugin>` | Remove | `ddx plugin uninstall <plugin>` |
| `ddx outdated` | Remove | Staleness hints plus `ddx plugin upgrade [name]` |
| `ddx verify` | Remove | `ddx doctor --plugins` |
| `ddx update` | Remove | `ddx upgrade`, `ddx init --force`, `ddx plugin upgrade` |

Adjacent legacy `ddx agent *` workflow commands are not part of this plugin CLI
cleanup. They are already governed by
`plan-2026-04-29-artifact-and-run-architecture`, which moves execution to
`ddx run`, `ddx try`, `ddx work`, `ddx runs`, `ddx tries`, and skills such as
`compare-prompts`, `benchmark-suite`, and `replay-bead`.

## Data Model

### Project State

Create a project-local plugin state file:

```yaml
# .ddx/plugins.yaml
plugins:
  - name: helix
    version: 1.2.3
    type: plugin
    source: https://github.com/DocumentDrivenDX/helix
    installed_at: 2026-05-13T12:00:00Z
    files:
      - .ddx/plugins/helix
      - .agents/skills/helix
      - .claude/skills/helix
```

Rules:

- Replace installed-package state naming with plugin-state naming:
  `registry.LoadPluginState(projectRoot)`, `SavePluginState(projectRoot, state)`,
  `PluginState`, and `PluginEntry`. Avoid preserving `InstalledState` /
  `InstalledEntry` names for new plugin lifecycle code.
- The plugin-state APIs must be project-root aware and read/write
  `.ddx/plugins.yaml`.
- No production path reads or writes `~/.ddx/installed.yaml`.
- State entries describe registry-installed plugins only.
- Local overlays are derived from `os.Lstat(".ddx/plugins/<name>")` returning a
  symlink, not from state pins.
- Stored file paths should be project-relative where possible.

### Ownership

Plugin-owned skill entries should be attributable without following symlinks into
developer checkouts:

- Registry install ownership comes from `.ddx/plugins.yaml.files`.
- Local overlay ownership is discovered at uninstall/list time from the current
  symlinked plugin checkout. Remove only `.agents/skills/<skill>` and
  `.claude/skills/<skill>` entries that are symlinks pointing directly into that
  checkout's discovered skill directories.
- Cleanup must use `os.Lstat` for roots and skill entries.
- Cleanup must not `RemoveAll` a symlink target.

## Implementation Phases

### Phase 1: Command Surface Lockdown

Files:

- `cli/cmd/command_factory.go`
- `cli/cmd/install.go`
- `cli/cmd/update.go`
- `cli/cmd/command_factory_commands.go`
- CLI help/contract tests under `cli/cmd/*test.go`

Work:

1. Stop registering top-level `newInstallCommand`, `newInstalledCommand`,
   `newUninstallCommand`, `newOutdatedCommand`, `newVerifyCommand`, and
   `newUpdateCommand` on the root command.
2. Make `newPluginCommand` own dedicated subcommands instead of reusing the
   top-level install command:
   - `newPluginInstallCommand`
   - `newPluginListCommand`
   - `newPluginUpgradeCommand`
   - `newPluginUninstallCommand`
3. Move any shipped-skill refresh behavior currently in `ddx update` to
   `ddx init --force`, then delete the `ddx update` command.
4. Keep `ddx upgrade` binary-only. Do not add plugin arguments.
5. Update command help text so examples teach only `ddx plugin *` for plugins.

Acceptance:

- `ddx install helix` exits as an unknown command.
- `ddx installed` exits as an unknown command.
- `ddx uninstall helix` exits as an unknown command.
- `ddx outdated` exits as an unknown command.
- `ddx verify` exits as an unknown command.
- `ddx update` exits as an unknown command.
- Plugin integrity checks are covered by `ddx doctor --plugins`.
- `ddx plugin --help` shows install/list/upgrade/uninstall.
- `ddx upgrade --help` describes binary upgrade only.

### Phase 2: Project-Local Plugin State

Files:

- `cli/internal/registry/state.go`
- `cli/internal/registry/registry.go`
- `cli/cmd/install.go`
- `cli/cmd/doctor_plugins_test.go`
- tests that currently create `~/.ddx/installed.yaml`

Work:

1. Replace `installedStatePath()` with `PluginStatePath(projectRoot)` returning
   `<projectRoot>/.ddx/plugins.yaml`.
2. Replace `LoadState()` / `SaveState()` call sites with
   `LoadPluginState(projectRoot)` / `SavePluginState(projectRoot, state)`.
   Avoid package-level cwd assumptions.
3. Rename user-facing concepts from "installed packages" to "project plugins".
4. Rename new lifecycle structs and helper methods from installed/package terms
   to plugin terms.
5. Remove any production code that creates `~/.ddx`.
6. Update doctor/plugin audit code to inspect `.ddx/plugins.yaml` and discovered
   local overlays.

Acceptance:

- Plugin install/list/upgrade/uninstall creates or updates `.ddx/plugins.yaml`.
- Running plugin commands in a temp project leaves `~/.ddx`, `~/.agents`, and
  `~/.claude` absent.
- Tests do not use `~/.ddx/installed.yaml` except, if retained at all, as a
  negative assertion that it is ignored.

### Phase 3: Registry Plugin Install as Real Files

Files:

- `cli/cmd/install.go`
- `cli/internal/registry/installer.go`
- `cli/internal/registry/manifest.go`
- `cli/internal/skills/install.go`
- `cli/cmd/install_manifest_test.go`
- `cli/internal/registry/manifest_test.go`
- `cli/internal/registry/installer_test.go`

Work:

1. Keep registry plugin root installation as real files under
   `.ddx/plugins/<name>/`.
2. Keep registry skill installation as real files under `.agents/skills/<skill>`
   and `.claude/skills/<skill>`.
3. Remove manifest-declared install symlink behavior from registry installs or
   reject it during validation. Symlinks are reserved for `--local`.
4. Reject every manifest install target beginning with `~` or escaping the project
   root, including root, skills, scripts, symlinks, hooks, and executables.
5. Remove registry install auto-commit unless an explicit command-level policy is
   approved separately. The implementation should leave normal git diff for the
   operator.
6. Preserve stale file cleanup using project-relative file lists and `os.Lstat`.

Acceptance:

- `ddx plugin install helix` writes real directories, not symlinks.
- `.ddx/plugins/helix`, `.agents/skills/helix`, and `.claude/skills/helix` are
  project-local.
- Home-rooted or project-escaping manifest targets fail before writes.
- Registry reinstall removes stale plugin-owned files without touching unrelated
  bootstrap skills or other plugins.

### Phase 4: Local Symlink Overlay

Files:

- `cli/cmd/install.go`
- `cli/internal/registry/manifest.go`
- `cli/internal/skills/validate.go`
- `cli/cmd/install_manifest_test.go`

Work:

1. Keep `ddx plugin install <name> --local <path>` as the only public local
   overlay command.
2. Validate the local checkout before writing:
   - Accept `skills/<skill>` as the preferred skill layout.
   - Accept `.agents/skills/<skill>` where existing plugin repos still use it.
   - Require each skill to pass `ddx skills check` equivalent validation.
3. Resolve `<path>` to an absolute path.
4. Replace `.ddx/plugins/<name>` with a symlink to the absolute checkout path.
5. Replace `.agents/skills/<skill>` and `.claude/skills/<skill>` with direct
   symlinks to the checkout skill directory. Do not create chained symlinks.
6. Require `--force` before replacing an existing non-symlink plugin root or skill
   directory.
7. Do not update `.ddx/plugins.yaml`.
8. Do not auto-commit.

Acceptance:

- `readlink .ddx/plugins/helix` returns the absolute checkout path.
- `readlink .agents/skills/helix` and `.claude/skills/helix` point directly to
  the checkout skill directory.
- `ddx plugin list` shows the overlay as `local` even when `.ddx/plugins.yaml`
  has no helix entry.
- `ddx plugin upgrade helix` skips the overlay with `helix is local-linked; skipped`.
- `ddx plugin uninstall helix` removes only project symlinks and leaves the
  checkout untouched.

### Phase 5: Plugin List, Upgrade, Uninstall

Files:

- `cli/cmd/install.go` or new `cli/cmd/plugin.go`
- `cli/cmd/update.go`
- `cli/internal/registry/*`
- `cli/cmd/install_manifest_test.go`

Work:

1. Implement `ddx plugin list` from two sources:
   - `.ddx/plugins.yaml` registry entries.
   - symlinked directories under `.ddx/plugins/` as local overlays.
2. Implement `ddx plugin upgrade [name]`:
   - Named: upgrade one registry plugin, skip if local-linked.
   - Unnamed: upgrade all registry plugins, skip all local overlays.
   - Use GitHub release lookup currently used by install/update.
3. Implement `ddx plugin uninstall <name>`:
   - Registry install: remove recorded files and state entry.
   - Local overlay: remove `.ddx/plugins/<name>` symlink and plugin-owned skill
     symlinks only.
   - Discover plugin-owned local-overlay skills from the symlinked checkout and
     remove only project skill entries whose symlink targets point into that
     checkout.
   - Missing plugin should return a clear not-installed error.
4. Ensure stale cleanup never follows symlinked plugin roots.

Acceptance:

- `ddx plugin list` reports name, version or `local`, source/path, status.
- `ddx plugin upgrade` upgrades registry plugins and skips local overlays.
- `ddx plugin uninstall` behaves correctly for registry installs and local overlays.
- No command touches legacy home plugin state.

### Phase 6: Docs, Skills, Website, Tests

Files:

- `cli/internal/skills/ddx/reference/agents.md`
- `.agents/skills/ddx/reference/agents.md`
- `.claude/skills/ddx/reference/agents.md`
- `docs/helix/**`
- README / website command snippets
- demo scripts under `scripts/demos/`

Work:

1. Replace active user-facing examples with `ddx plugin install/list/upgrade/uninstall`.
2. Keep old command mentions only in historical docs or explicit retired-interface notes.
3. Update DDx skill routing/reference text so agents do not recommend old commands.
4. Update onboarding and demo scripts to use the new plugin noun.

Acceptance:

- Repo search for active docs has no unqualified `ddx install helix`,
  `ddx installed`, `ddx uninstall <plugin>`, `ddx outdated`, `ddx verify`, or
  `ddx update` recommendations.
- `ddx skills check` passes for DDx skill copies touched by this work.

## Test Plan

Run focused tests as implementation proceeds:

```bash
cd cli && go test ./cmd -run 'Test.*Plugin|Test.*Install|Test.*Upgrade|Test.*Uninstall|Test.*Installed|Test.*Outdated'
cd cli && go test ./internal/registry/...
cd cli && go test ./internal/skills/...
ddx skills check cli/internal/skills/ddx .agents/skills/ddx .claude/skills/ddx
```

Final gate:

```bash
cd cli && go test ./...
lefthook run pre-commit
```

## Bead Breakdown

Recommended execution slices:

1. **CLI surface lockdown** — root registration and dedicated `plugin` command
   tree, with help/unknown-command tests.
2. **Project plugin state** — move state from `~/.ddx/installed.yaml` to
   `.ddx/plugins.yaml`, update audit/list helpers.
3. **Registry install cleanup** — real-file install, manifest target validation,
   stale cleanup, no auto-commit.
4. **Local overlay mode** — symlink behavior, validation, no pin mutation,
   uninstall safety.
5. **Plugin upgrade/list/uninstall** — lifecycle commands and skip semantics for
   overlays.
6. **Docs and skill references** — active command examples, demos, website, and
   skill text.
7. **End-to-end acceptance** — temp-project tests covering FEAT-015 AC-001
   through AC-017.

## Risks And Mitigations

- **Risk: tests rely on home-state helpers.** Mitigation: migrate helpers first
  and make home-state absence a fixture invariant.
- **Risk: stale cleanup follows a local overlay symlink.** Mitigation: centralize
  deletion through `os.Lstat` and test with symlinked plugin roots.
- **Risk: resource-library installs are conflated with plugin lifecycle.**
  Mitigation: keep `ddx plugin *` plugin-only and leave resource install as a
  separate future design if needed.
- **Risk: top-level command removal breaks hidden scripts.** Mitigation: update
  demos, README, website, skills, and active docs in the same implementation pass.
- **Risk: registry manifests still declare script targets under `~`.** Mitigation:
  fail fast with actionable validation and update shipped/builtin registry entries
  to project-relative targets.
