---
ddx:
  id: ADR-006
  depends_on:
    - FEAT-002
    - FEAT-013
    - AR-2026-04-04
---
# ADR-006: Use Tailscale ts-net for Network Authentication

**Status:** Accepted
**Date:** 2026-04-05
**Context:** DDx server exposes HTTP and MCP endpoints for remote document
browsing, bead management, and agent dispatch. Non-localhost access requires
authentication. The initial design considered API keys, but the product owner
chose Tailscale ts-net for its zero-configuration identity model.

## Decision

Use **Tailscale's tsnet library** (`tailscale.com/tsnet`) to provide
authenticated network access to the DDx server. No custom API key management,
no token rotation, no auth middleware.

### Why ts-net

| Approach | Verdict | Reason |
|----------|---------|--------|
| **ts-net (Tailscale)** | **Chosen** | Identity from Tailscale network; zero config for users already on a tailnet; mutual TLS built in; no secrets to manage |
| API keys | Rejected | Requires key generation, storage, rotation, revocation; adds config surface DDx doesn't need |
| OAuth/OIDC | Rejected | Requires identity provider integration; too heavy for a local-first developer tool |
| mTLS (manual) | Rejected | Certificate management complexity without a PKI; ts-net provides this implicitly |
| No auth (localhost-only) | Baseline | Current default; ts-net extends this to tailnet peers without weakening local security |

### How It Works

#### Default Mode (unchanged)

DDx server binds to `127.0.0.1:<port>` by default. All localhost requests
are accepted without authentication. This is the current behavior and remains
the default.

#### ts-net Mode (opt-in)

When configured, DDx starts a ts-net listener alongside the standard listener:

```yaml
# .ddx/config.yaml
server:
  addr: "127.0.0.1:8080"    # local listener (always)
  tsnet:
    enabled: true
    hostname: "ddx"           # appears as ddx.<tailnet> on the network
    auth_key: ""              # optional; prefer TS_AUTHKEY env var over this field (secrets in config files can leak)
```

The ts-net listener:

1. Joins the user's tailnet using their Tailscale identity
2. Receives a machine certificate from the Tailscale coordination server
3. Serves HTTPS with automatic Let's Encrypt certificates via Tailscale
4. Identifies callers by their Tailscale identity (node name, user email)

#### Request Identity

Every request through the ts-net listener carries Tailscale identity headers:

```go
who, err := lc.WhoIs(r.Context(), r.RemoteAddr)
// who.UserProfile.LoginName = "erik@example.com"
// who.Node.ComputedName = "eriks-laptop"
```

DDx logs the caller identity for audit purposes. No additional authorization
layer is needed for v1 — all tailnet members who can reach the DDx node have
full access. Access control via Tailscale ACLs is a future refinement.

### Integration Pattern

```go
import "tailscale.com/tsnet"

func (s *Server) listenTsnet(hostname string) error {
    ts := &tsnet.Server{
        Hostname: hostname,
        Dir:      filepath.Join(s.WorkingDir, ".ddx", "tsnet"),
    }
    defer ts.Close()

    ln, err := ts.ListenTLS("tcp", ":443")
    if err != nil {
        return err
    }
    return http.Serve(ln, s.mux) // same mux as localhost
}
```

The same HTTP mux serves both localhost and ts-net listeners. The localhost
guard on dispatch endpoints (`isLocalhost()`) should be extended to also
accept ts-net-identified requests.

### Configuration Surface

| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `server.tsnet.enabled` | bool | false | Enable ts-net listener |
| `server.tsnet.hostname` | string | "ddx" | Tailscale hostname |
| `server.tsnet.auth_key` | string | "" | Tailscale auth key (for headless/CI); see precedence note below |
| `server.tsnet.state_dir` | string | ".ddx/tsnet" | ts-net state directory |
| `TS_AUTHKEY` | env var | unset | Tailscale auth key (preferred; avoids secrets in config files or shell history) |

**Auth key precedence:** `TS_AUTHKEY` env var > `--tsnet-auth-key` CLI flag > `server.tsnet.auth_key` config field. Prefer `TS_AUTHKEY` in CI and headless environments — secrets on the CLI are visible in `ps` output and shell history.

### What This Replaces

- The `auth` package (`cli/internal/auth/`) was designed for git platform
  authentication (GitHub, GitLab tokens). It remains for that purpose.
- Server network authentication is handled entirely by ts-net at the
  transport layer. No application-level auth middleware is needed.
- The MCP bead write tools (ddx_bead_create, ddx_bead_update, ddx_bead_claim)
  previously flagged as needing "optional API key auth" now get auth from
  ts-net instead.

## Consequences

- **Zero-config for tailnet users**: Anyone on the same tailnet can access
  the DDx server by hostname. No API keys to distribute.
- **No key management**: No generation, rotation, or revocation of API keys.
  Identity comes from Tailscale.
- **Tailscale dependency**: Users who want remote access must have Tailscale
  installed. This is acceptable for a developer tool — Tailscale is widely
  adopted in dev teams.
- **Localhost remains free**: Local-only users are unaffected. ts-net is
  opt-in.
- **~5MB binary size increase**: The tsnet library adds to the Go binary.
  Acceptable for a developer tool.
- **Automatic HTTPS**: ts-net provides TLS certificates via Tailscale's
  DERP infrastructure. No manual cert management.

## Alternatives Considered

See the comparison table above. The key insight is that DDx is a local-first
tool where remote access is a convenience, not a primary use case. ts-net
provides authentication as a side effect of network connectivity, which
matches DDx's "zero-config" philosophy better than any token-based scheme.
