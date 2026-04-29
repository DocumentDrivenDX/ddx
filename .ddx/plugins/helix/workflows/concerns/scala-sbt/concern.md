# Concern: Scala + sbt

## Category
tech-stack

## Areas
all

## Components

- **Language**: Scala 2.x (pinned per project)
- **Build system**: sbt with `sbt-dynver` for git-tag-based versioning
- **Formatter**: `scalafmt`
- **Linter / refactoring**: `scalafix` with `OrganizeImports`
- **Testing**: ScalaTest (primary)
- **Effect system**: ZIO (where applicable)
- **Versioning**: `sbt-dynver` with `-SNAPSHOT` suffix for dirty/non-tagged commits

## Constraints

- All code must pass `scalafmtCheckAll` (zero diff)
- All code must pass `scalafixAll OrganizeImports`
- No uncommitted changes should reach CI with a clean version string
- Concurrent task limits: derived from CPU count (`(nproc / 2) - 1`, min 2)
- Remote build cache: `pushRemoteCacheTo` configured for incremental CI builds
- Library dependency schemes must be explicit to avoid eviction noise

## When to use

Existing Scala projects on the sbt ecosystem. New Scala services should
evaluate ZIO + sbt as the default stack. Note: projects actively migrating
from Scala to TypeScript should prefer `typescript-bun` for new code and
maintain `scala-sbt` only for the remaining Scala surface.

## ADR References
