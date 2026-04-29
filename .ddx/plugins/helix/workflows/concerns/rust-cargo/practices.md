# Practices: rust-cargo

## Requirements (Frame phase)
- Specify MSRV explicitly; it must be stable enough for CI and local dev
- Identify crates that are libraries vs binaries — error handling strategy differs
- If concurrency correctness is a requirement, plan for `loom`-based testing

## Design
- Organize as a Cargo workspace; every logical component is its own crate
- All inter-crate dependencies declared in `[workspace.dependencies]`
- Separate `crates/` (libraries) from `tools/` or `bin/` (binaries/CLIs) in workspace layout
- Design error types using `thiserror` for library crates; surface errors with `anyhow` in binaries
- Prefer newtypes and strong typing over stringly-typed parameters
- Concurrent state: prefer `Arc<Mutex<T>>` or `dashmap` for shared state; use `loom` for model-checking critical sections

## Implementation
- Run all commands through the pinned-toolchain wrapper script (e.g. `scripts/with-pinned-rust.sh cargo ...`)
- Every new crate must include `[lints] workspace = true` in its `Cargo.toml`
- Style: inline format args (`format!("{x}")`), method refs over closures (`.map(String::as_str)`), explicit match arms over wildcards, collapse nested ifs
- No `println!`/`eprintln!` for operational output — use `tracing` events
- No `.unwrap()` or `.expect()` in library code; in binary code, only at startup with a clear message
- `unsafe` blocks: add `// SAFETY:` comment explaining invariants, add local `#[allow(unsafe_code)]`, document in PR
- Adding a dependency: add to `[workspace.dependencies]` first, reference with `{ workspace = true }`, then run `cargo deny check` and `cargo machete`

## Testing
- Unit tests in `#[cfg(test)]` modules within the source file
- Integration tests in `tests/integration/`; contract/E2E tests in separate crates
- Use `proptest` for property-based testing of pure functions and data invariants
- Use `loom` for model-checking concurrent code (mutex invariants, atomic correctness)
- Use `rstest` for parameterized test cases
- Use `insta` for snapshot testing of complex outputs
- Use `testcontainers` for tests requiring real external services (databases, message queues)
- Use `tempfile` for filesystem fixtures; never hard-code paths
- Run focused tests first: `cargo test -p <crate> <test_name>`, then full suite
- Coverage: `cargo llvm-cov` for source-line coverage gating

## Quality Gates (pre-commit / CI)
- `cargo fmt --all -- --check`
- `cargo clippy --workspace --all-targets --no-deps -- -D warnings -D clippy::todo -D clippy::unimplemented -D clippy::dbg_macro -D clippy::print_stdout -D clippy::unwrap_used`
- `cargo deny check`
- `cargo machete crates tools` (or workspace equivalent)
- `cargo test --workspace` (excluding infra-dependent suites in pure-unit CI)

## Performance
- Benchmark with `criterion` (statistical) or a custom bench harness
- Profile with `[profile.profiling]` + `cargo flamegraph` or `pprof`
- Do not claim performance improvements without exact benchmark command and output
- Local storage/throughput targets take precedence over remote-tier optimizations until explicitly promoted
