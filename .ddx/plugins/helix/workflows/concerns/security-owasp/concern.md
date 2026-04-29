# Concern: Security (OWASP)

## Category
security

## Areas
all

## Components

- **Standard**: OWASP Top 10 (current edition)
- **Dependency auditing**: language-specific tooling (see per-stack practices)
- **Secret management**: environment variables or secret manager — never in code or config files
- **TLS**: HTTPS/TLS required for all network-facing services
- **Input validation**: at all system boundaries (API endpoints, file parsers, CLI args)

## Constraints

- No secrets, credentials, API keys, or tokens committed to source control
- All external inputs validated before use; reject or sanitize at the boundary
- Authentication and authorization checked on every protected endpoint
- Dependencies must be audited for known vulnerabilities before release
- HTTPS/TLS enforced for all production network traffic
- SQL queries must use parameterized queries / prepared statements — no string interpolation
- Error responses must not leak implementation details (stack traces, internal paths, SQL errors)

## Per-Stack Dependency Audit Commands

| Stack | Audit command |
|-------|--------------|
| Rust | `cargo deny check advisories` |
| Go | `govulncheck ./...` |
| TypeScript/Bun | `bun audit` |
| Python | `pip-audit` or `uv run pip-audit` |
| Scala | `sbt dependencyCheck` (OWASP plugin) |

## When to use

All projects with network-facing services, user authentication, or data storage.
Security is a cross-cutting concern — it is not a phase or a separate checklist,
it applies throughout every phase of development.

## ADR References
