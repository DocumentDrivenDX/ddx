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
Status: Draft

## Summary

The DDx skill tree should be content owned by the default `ddx` plugin package,
not a parallel bootstrap tree with checked-in mirrors. `ddx init`, `ddx update`,
and `ddx plugin install ddx` should all install the same package layout through
the same package installer.

Development installs may use symlink overlays for live editing, but those
overlays are project-local. DDx must not restore home-directory skill installs;
the only expected home binary is `~/.local/bin/ddx`.

## Current Problems

- `skills/ddx/` is the editable source, but `cli/internal/skills/ddx/`,
  `.agents/skills/ddx/`, `.claude/skills/ddx/`, and `.ddx/skills/ddx/` are
  full copied mirrors.
- `cli/Makefile copy-skills` syncs the top-level skill tree into embedded and
  project-local copies, so editing behavior depends on remembering which mirror
  is canonical.
- `ddx init` installs shipped skills through a direct bootstrap path using
  `skills.SkillFiles`, then separately installs the default `ddx` plugin
  package.
- `ddx update` refreshes shipped skills through the same direct embedded-skill
  path rather than the package installer.
- `ddx plugin install ddx --local .` is unsafe in the current repo layout
  because local install prefers `<pluginRoot>/.agents/skills` before
  `<pluginRoot>/skills`; pointed at the repo root, that can target the
  destination skill tree instead of the intended source.

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
.agents/skills/ddx  -> installed by package installer
.claude/skills/ddx  -> installed by package installer
```

For registry installs, these outputs are real files. For local development
overlays, they may be symlinks to the source checkout.

## Manifest Contract

`library/package.yaml` declares package-local skill sources:

```yaml
install:
  root:
    source: .
    target: .ddx/plugins/ddx
  skills:
    - source: skills/
      target: .agents/skills/
    - source: skills/
      target: .claude/skills/
```

The installer must honor `install.skills[*].source` relative to the package
root. Skill discovery must not hard-code `.agents/skills` as the primary source
when the manifest explicitly says `skills/`.

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
2. apply `install.root.source -> install.root.target`;
3. apply every `install.skills[*].source -> install.skills[*].target`;
4. apply scripts/executables with the existing project-scope rules;
5. return one `InstalledEntry` shape.

Remote and embedded installs write real files. Local installs create developer
overlays: `.ddx/plugins/<name>` and plugin-owned skill outputs are symlinks to
the local checkout. Local overlays do not mutate recorded plugin pins and do not
auto-commit.

## Init And Update Contract

`ddx init` installs the default DDx package through the embedded package
installer. It no longer calls a separate embedded skill installer and no longer
maintains `.ddx/skills/ddx` as a bootstrap-only mirror.

`ddx update` / the forward replacement for shipped-content refresh uses the
same package installer path as `ddx init`.

Remove these direct bootstrap mechanisms after replacement tests exist:

- `skills.Install(skills.SkillFiles, ...)` calls from `ddx init` and update;
- `registerBootstrapDDxSkills`;
- `cli/internal/skills/ddx` as a distinct embedded source;
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

1. Add embedded default-plugin package support for `library/`.
2. Teach the installer to honor `install.skills[*].source` for remote,
   embedded, and local installs.
3. Change `ddx init` to install `ddx` through the embedded package installer.
4. Change shipped-content refresh/update to use the same package installer.
5. Move `skills/ddx/` to `library/skills/ddx/`.
6. Replace `copy-skills` with a default-package embed sync:
   `library/ -> cli/internal/registry/defaultplugin/library/`.
7. Remove the checked-in skill mirrors after tests prove install outputs are
   produced by init/local install.
8. Update FEAT-011, FEAT-015, SD-011, and any command help or AGENTS blocks that
   describe copied bootstrap skill mirrors.

## Required Tests

- `TestInitInstallsDDxPluginPackage`: `ddx init` creates `.ddx/plugins/ddx`,
  `.agents/skills/ddx`, and `.claude/skills/ddx` through the package installer.
- `TestPluginInstallLocalDDxLibrarySymlinksSkills`: `ddx plugin install ddx
  --local library --force` creates project-local symlinks to
  `library/skills/ddx`.
- `TestPluginInstallHonorsSkillSource`: a fixture with `install.skills.source:
  skills/` installs skills from `skills/` and does not require
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
