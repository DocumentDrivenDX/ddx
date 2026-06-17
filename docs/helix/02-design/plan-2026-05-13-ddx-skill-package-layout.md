---
ddx:
  id: plan-2026-05-13-ddx-skill-package-layout
  depends_on:
    - FEAT-011
    - FEAT-015
    - FEAT-018
---
# DDx Skill Package Layout

Date: 2026-05-13
Status: Accepted; implemented for cache-backed adapters; remaining cleanup tracked

## Summary

The DDx skill tree is content owned by the default `ddx` bootstrap package, not
a parallel tree of checked-in project payloads. `ddx init`, `ddx update`, and
`ddx plugin install ddx` install through the same package/cache topology.

As of 2026-06-17, the forward behavior is cache-backed and npx-like:
`ddx init` and `ddx plugin sync` materialize the built-in `ddx` package into
the shared XDG plugin cache and expose generated adapters in
`.agents/skills/ddx` and `.claude/skills/ddx`. Marketplace plugins such as
HELIX use the same generated-adapter shape from `.ddx/plugins.lock.yaml` plus
`${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`. Checked-in plugin
payloads and generated agent adapters are not part of the forward repository
state.

Because HELIX is a marketplace plugin, the built-in `ddx` plugin must stay
minimal. It exists to bootstrap DDx worker discovery and operator commands
offline; it must not become the home for HELIX prompts, personas, workflow
templates, checks, MCP server definitions, or other workflow assets. Those
assets belong in separately versioned marketplace/cache packages and are
resolved by lockfile intent plus the shared cache, similar to how `npx` resolves
package code without vendoring it into each project.

Development installs may use symlink overlays for live editing, but those
overlays are project-local. DDx must not restore home-directory skill installs;
the only expected home binary is `~/.local/bin/ddx`.

## Remaining Gaps

- `cli/Makefile copy-skills` and `sync-embedded-default-plugin` must remain
  deterministic: they may sync source into embedded release fixtures, but they
  must not recreate project-local payload copies.
- The root and embedded `package.yaml` manifests must stay minimal:
  `materialize.skills` only. No `install.root` payload should be advertised for
  normal installs.
- `ddx plugin install ddx --local .` must continue to resolve safely to the
  package root or fail before it can self-link `.agents/skills/ddx`.
- Docs and command help must describe `.ddx/plugins/<name>` only as legacy or
  local-overlay state, never as the expected registry install layout.

## Target Layout

The default package owns the DDx skill:

```text
library/
  package.yaml
  skills/
    ddx/
      SKILL.md
      reference/
      bead-lifecycle/
      ...
```

The checked-in editable source is `library/skills/ddx/`.

The release binary embeds the default package content, not a special skill-only
tree:

```text
library/ -> cli/internal/registry/defaultplugin/library/
```

The project-local discovered skill paths are install outputs:

```text
.agents/skills/ddx  -> generated adapter to package cache or local overlay
.claude/skills/ddx  -> generated adapter to package cache or local overlay
```

For registry installs, these outputs are generated adapters that resolve into
`${XDG_DATA_HOME}/ddx/cache/plugins/<name>/<version>/`. For the built-in
`ddx` package, `ddx init` may populate that cache from the binary's embedded
default package before writing adapters. For local development overlays, the
adapters may link directly to the source checkout.

## Manifest Contract

`library/package.yaml` declares package-local skill sources for the built-in
DDx bootstrap package:

```yaml
materialize:
  skills:
    - source: skills/
      target: .agents/skills/
    - source: skills/
      target: .claude/skills/
```

The built-in `ddx` manifest must not declare `install.root`. It is a bootstrap
skill package, not a full workflow payload. Normal installs write project intent
to the plugin lock, store payloads in the shared XDG cache, and materialize only
generated adapters into the project worktree. The full payload tree is not
copied to `.ddx/plugins/<name>/` for registry installs.

The installer must honor `materialize.skills[*].source` relative to the package
root, with `install.skills` retained only as a compatibility fallback. Skill
discovery must not hard-code `.agents/skills` as the primary source when the
manifest explicitly says `skills/`.

Directory-to-directory skill mappings install the child skill directories under
the target. The example above installs `skills/ddx` to `.agents/skills/ddx` and
`.claude/skills/ddx`; it does not replace `.agents/skills` or `.claude/skills`
as whole directories. This keeps unrelated project skills visible.

## Installer Contract

Split the package installer into source-agnostic operations:

```go
InstallPackageFromRemote(pkg, projectRoot)
InstallPackageFromDir(pkg, sourceDir, projectRoot)
InstallPackageFromFS(pkg, sourceFS, projectRoot)
```

All three paths must share the same core install implementation:

1. load and validate `package.yaml` when present;
2. for local overlays only, apply `install.root.source -> install.root.target`;
3. apply every `materialize.skills[*].source -> materialize.skills[*].target`
   mapping, falling back to `install.skills` for legacy packages;
4. apply scripts/executables with the existing project-scope rules;
5. return one `InstalledEntry` shape.

Remote installs write the package payload to the shared XDG cache and generated
adapters to the project. Embedded installs do the same using the baked-in
default package as the source and do not create `.ddx/plugins/ddx` or a normal
project plugin-lock entry for `ddx`. Local installs may create developer
overlays: `.ddx/plugins/<name>` and plugin-owned skill outputs are links to the
local checkout. Local overlays do not mutate recorded plugin pins and do not
auto-commit.

## Init And Update Contract

`ddx init` exposes the default DDx package through the same cache-backed
adapter topology as registry plugins. It materializes the embedded package into
the XDG cache when needed, creates `.agents/skills/ddx` and
`.claude/skills/ddx` adapters, and does not create `.ddx/plugins/ddx` for the
built-in package. It no longer calls a separate embedded skill installer and no
longer maintains `.ddx/skills/ddx` as a bootstrap-only mirror.

`ddx update` / the forward replacement for shipped-content refresh uses the
same package installer path as `ddx init`.

Do not restore these direct bootstrap mechanisms:

- `skills.Install(skills.SkillFiles, ...)` calls from `ddx init` and update;
- `registerBootstrapDDxSkills`;
- `cli/internal/skills/ddx` as an embedded skill mirror;
- `skills/ddx` as the top-level canonical source;
- checked-in `.agents/skills/ddx`, `.claude/skills/ddx`, and `.ddx/skills/ddx`
  mirrors.

## Development Workflow

The repo development workflow becomes:

```bash
ddx plugin install ddx --local library --force
```

After that command, edits to `library/skills/ddx/` are live through the
project-local harness discovery paths.

The command must be safe when run from the DDx repo. If someone passes the repo
root instead of `library`, the installer must either resolve to the manifest's
declared package root or fail with a clear error; it must not self-link
`.agents/skills/ddx`.

## Migration Sequence

1. Done: add embedded default-plugin package support for `library/`.
2. Done: teach the installer to honor `install.skills[*].source` for remote,
   embedded, and local installs.
3. Done: change `ddx init` to install `ddx` through the embedded package
   installer.
4. Done: materialize project skill paths as cache-backed adapters, not payload
   copies.
5. Done: move the canonical DDx skill source to `library/skills/ddx/`.
6. Keep: sync the minimal `library/package.yaml` plus `library/skills/ddx/`
   into `cli/internal/registry/defaultplugin/library/` as the generated
   embedded release fixture.
7. Done: remove the legacy `cli/internal/skills/ddx` embedded skill mirror.
8. Continue updating FEAT-011, FEAT-015, SD-011, command help, and AGENTS blocks
   when they describe copied bootstrap skill mirrors.

## Required Tests

- `TestInitInstallsDDxPluginPackage`: `ddx init` creates cache-backed
  `.agents/skills/ddx` and `.claude/skills/ddx` adapters through the package
  installer and does not create `.ddx/plugins/ddx`.
- `TestDDxDefaultManifestIsBootstrapOnly`: root and embedded
  `library/package.yaml` expose only `materialize.skills` and do not advertise
  HELIX-style prompts, personas, templates, checks, tools, MCP servers, or
  `install.root`.
- `TestPluginInstallLocalDDxLibrarySymlinksSkills`: `ddx plugin install ddx
  --local library --force` creates project-local symlinks to
  `library/skills/ddx`.
- `TestPluginInstallHonorsSkillSource`: a fixture with
  `materialize.skills.source: skills/` installs skills from `skills/` and does
  not require
  `.agents/skills` inside the source package.
- `TestPluginInstallPreservesUnrelatedProjectSkills`: installing the `ddx`
  package does not replace `.agents/skills` or `.claude/skills` when other
  skills already exist there.
- `TestInitWorksOfflineWithEmbeddedDefaultPlugin`: no network is needed for
  `ddx init` to install the default package and skill.
- `TestRepoRootLocalInstallDoesNotSelfLink`: passing the DDx repo root either
  resolves to the package root safely or fails before modifying skill outputs.
- `ddx skills check library/skills/ddx .agents/skills/ddx .claude/skills/ddx`
  passes after local install.
- `make skill-schema` validates the package-owned skill source and embedded
  default-package copy.

## Non-Scope

- Do not restore home-directory skill installs.
- Do not change the public plugin CLI names beyond the separate plugin CLI
  rationalization plan.
- Do not remove the embedded package copy until offline `ddx init` is green.
