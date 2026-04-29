# Practices: security-owasp

## Requirements (Frame phase)
- Identify trust boundaries: where does data enter the system from untrusted sources?
- Classify data sensitivity: what data requires encryption at rest or in transit?
- Identify authentication model: who authenticates, how, and what are the session semantics?
- Include security acceptance criteria in user stories for any auth, data access, or input-handling feature

## Design
- Apply least-privilege: services, users, and credentials have minimum necessary permissions
- Prefer deny-by-default for authorization (explicitly allow rather than explicitly deny)
- Separate authentication (who are you?) from authorization (what can you do?)
- Never store plaintext passwords — use `argon2id` or `bcrypt`
- Encryption at rest for sensitive data stores; TLS for all transport
- Audit log for security-relevant actions (login, auth failure, privilege escalation)

## Implementation
- Input validation at every system boundary — validate type, length, format, and range
- SQL: use parameterized queries or ORM with parameter binding; no string interpolation
- Secrets: load from environment variables or secret manager at startup; never embed in source
- Error messages: return generic error to clients; log full details server-side with correlation ID
- File operations: validate paths (prevent path traversal); confirm file type before processing
- Dependencies: run audit tool before merging; pin to known-good versions
- TLS: use `rustls` (Rust), Go's stdlib `crypto/tls`, or established TLS library; no SSLv3/TLS 1.0/1.1
- CORS: whitelist allowed origins; do not use wildcard `*` for authenticated endpoints

## Testing
- Include at least one negative test per auth boundary (unauthenticated access must be rejected)
- Fuzz parser inputs where practical (Rust: `cargo fuzz`, Go: `go test -fuzz`, Python: hypothesis)
- Secrets scanning: run `trufflesecurity/trufflehog` or `gitleaks` in CI
- Dependency audit in CI gate (see per-stack commands in concern.md)

## Quality Gates (per-stack, add to CI)
- Rust: `cargo deny check advisories`
- Go: `govulncheck ./...` + `gosec ./...`
- TypeScript: `bun audit`
- Python: `uv run pip-audit`
- All: `gitleaks detect` or equivalent secrets scan on PR

## Incident Response
- Rotate compromised credentials immediately; do not wait to assess
- File a security bead with `security` label; treat as P0 if customer data at risk
- Document the incident in `docs/helix/06-iterate/` post-resolution
