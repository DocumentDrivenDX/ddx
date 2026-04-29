# Concern: Rust + Cargo

## Category
tech-stack

## Areas
all

## Components

- **Language**: Rust (latest stable; MSRV pinned in `rust-toolchain.toml`)
- **Build system**: Cargo workspace (resolver = "2")
- **Edition**: 2024
- **Toolchain pinning**: `rust-toolchain.toml` and `workspace.package.rust-version` must stay in lockstep

## Constraints

- All code must pass `cargo clippy --workspace --all-targets --no-deps -- -D warnings`
- All code must pass `cargo fmt --all -- --check`
- Workspace lints (`[workspace.lints]`) are authoritative; every crate opts in via `[lints] workspace = true`
- `unsafe_code = "deny"` at the workspace level; any `unsafe` block requires a `// SAFETY:` comment and a local `#[allow(unsafe_code)]`
- No `.unwrap()` in library crates — use `?` or explicit error handling
- Use `thiserror` for library error types; use `anyhow` for application/binary error handling
- Public API items must have `///` doc comments
- All dependencies declared in `[workspace.dependencies]`; crates reference with `{ workspace = true }`
- `cargo deny check` must pass (licenses, advisories, registry sources)
- `cargo machete` must pass (no unused dependencies)
- Repo-owned Rust commands run through a pinned-toolchain wrapper; do not rely on ambient `rustc`/`cargo` from PATH

## Clippy Lint Policy

Workspace-level `[workspace.lints.clippy]`:
- `all`, `pedantic`, `nursery` at `warn` (lint group baseline)
- `unwrap_used`, `todo`, `unimplemented`, `dbg_macro`, `print_stdout` at `deny`
- Selected pedantic/nursery lints may be `allow`-listed project-wide when they produce excessive noise; each allow must be documented in the workspace `Cargo.toml`

Workspace-level `[workspace.lints.rust]`:
- `unsafe_code = "deny"`
- `unused_must_use = "deny"`
- `missing_docs = "warn"`
- `dead_code = "warn"`

## Profile Conventions

- `[profile.dev]`: fast builds, `debug = 1`, deps at `opt-level = 1`
- `[profile.release]`: `lto = "thin"`, `codegen-units = 8`, `strip = true`
- `[profile.release-dist]` (CI only): `lto = "fat"`, `codegen-units = 1`, `panic = "abort"`
- `[profile.profiling]`: inherits release, `debug = 1`, `strip = false`

## When to use

Performance-critical systems, CLI tools, infrastructure software, and projects
where memory safety and zero-cost abstractions matter. The workspace lint policy
above is the minimum bar; projects may tighten further via `[profile]` or
additional deny-level lints.

## ADR References
