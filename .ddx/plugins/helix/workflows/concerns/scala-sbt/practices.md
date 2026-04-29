# Practices: scala-sbt

## Requirements (Frame phase)
- Identify whether the project is greenfield Scala or has a migration plan to another runtime
- If mid-migration, scope new work to the target stack and minimize new Scala surface

## Design
- Organize as an sbt multi-project build; each logical module is a subproject
- Depend on ZIO for effect management, ZIO JSON for serialization where applicable
- Define portable contracts at service seams to enable incremental migration

## Implementation
- Format before commit: run `scalafmtAll` + `scalafixAll OrganizeImports` (or the combined alias)
- Use `sbt-dynver` for versioning; do not hardcode version strings
- `dynverSeparator := "-"` for Docker compatibility
- `packageTimestamp := Package.gitCommitDateTimestamp` for reproducible artifacts
- Exclude `.bloop`, `.cache`, `.targets`, `.hydra`, `.metals` from IDE indexing

## Testing
- Framework: ScalaTest
- Property-based: ScalaCheck (if used)
- Run: `sbt test`
- CI: separate unit and integration suites; integration tests may require Docker services

## Quality Gates (pre-commit / CI)
- `sbt scalafmtCheckAll` — format check
- `sbt scalafixAll OrganizeImports` — import organization
- `sbt test` — unit test suite
- `sbt compile` — compile all subprojects

## Dependency Management
- Declare in `project/Dependencies.scala` or `build.sbt` with explicit `libraryDependencySchemes` for version conflicts
- Use `VersionScheme.Always` sparingly (only for known-safe upgrades)
- Remote cache: `pushRemoteCacheTo` reduces incremental CI time
