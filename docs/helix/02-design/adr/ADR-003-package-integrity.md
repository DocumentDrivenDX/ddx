---
ddx:
  id: ADR-003
  depends_on:
    - FEAT-009
---
# ADR-003: Package Integrity and Supply Chain Security

**Status:** Accepted
**Date:** 2026-04-04
**Context:** DDx installs packages (workflows, personas, plugins) from git-based registries. Downloaded content must be verified to prevent supply chain attacks.

## Decision

Use **commit SHA pinning + content tree hashing** in a version-controlled lockfile. No signing infrastructure. The lockfile is the trust anchor — it's "signed" by whoever committed it via git's own commit authorship.

### How It Works

#### On Install

1. User runs `ddx plugin install helix`
2. DDx resolves the package to a specific git commit SHA from the registry
3. DDx fetches the package payload for that exact version into the shared XDG
   plugin cache
4. DDx computes a SHA-256 hash of the content tree (all files in sorted order)
5. DDx records `(repo, commit, tree_hash)` and generated adapter paths in
   `.ddx/plugins.lock.yaml`
6. DDx materializes generated adapters under `.agents/skills/` and
   `.claude/skills/`; full marketplace payloads remain in the cache

#### On Subsequent Use

1. DDx reads `.ddx/plugins.lock.yaml`
2. On `ddx plugin upgrade`, DDx fetches the latest commit, computes the new tree hash, and shows the diff in the lockfile
3. The developer reviews and commits the lockfile change (like reviewing `go.sum` changes)

#### On Verification

1. `ddx doctor --plugins` audits installed plugin integrity
2. Re-computes tree hashes
3. Compares against `.ddx/plugins.lock.yaml`
4. Any mismatch = hard failure with clear error

### Lockfile Format

```yaml
# .ddx/plugins.lock.yaml — commit to version control, review changes in PRs
version: 1
packages:
  helix:
    repo: https://github.com/DocumentDrivenDX/helix
    commit: abc123def456789012345678901234567890abcd
    tree_hash: sha256:7c6f43f4a3b2e1d0...
    installed_at: 2026-04-04T12:00:00Z
    cache_path: ${XDG_DATA_HOME}/ddx/cache/plugins/helix/1.0.0
    generated_files:
      - .agents/skills/helix
      - .claude/skills/helix
  persona/strict-code-reviewer:
    repo: https://github.com/DocumentDrivenDX/ddx-library
    commit: def456789012345678901234567890abcdef0123
    tree_hash: sha256:a3f2dd...
    installed_at: 2026-04-04T12:00:00Z
    files:
      - path: .ddx/library/personas/strict-code-reviewer.md
        hash: sha256:...
```

### Tree Hash Computation

Deterministic hash of a directory tree:

```
For each file in sorted path order:
  hash += "file:" + relative_path + "\n"
  hash += file_contents
```

Using SHA-256. Same content always produces the same hash. File ordering is lexicographic by path. Symlinks are resolved. Binary files are included. `.git/` is excluded.

### Cached Payload and Adapter Checks

In addition to the tree hash (which covers the source), the lock records the
cache path and generated adapter paths. This enables:

- `ddx doctor --plugins` to distinguish missing cache payloads from missing
  generated adapters
- Detection of stale or missing generated skill shims
- Fast verification without re-fetching from git when the cache is already
  populated

### Registry-Level Checksums

Each registry's `registry.yaml` includes checksums for the latest release of each package:

```yaml
# registry.yaml
packages:
  helix:
    version: 1.0.0
    repo: https://github.com/DocumentDrivenDX/helix
    commit: abc123...
    tree_hash: sha256:7c6f43...
```

When DDx fetches the registry, it verifies that the commit and tree_hash for a package match what the registry claims. If DDx already has a lockfile entry for a package, the lockfile takes precedence (you trust your own pinned version over the registry's latest).

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| Registry repo compromised (modified registry.yaml) | Lockfile pins override registry. Developer reviews lockfile changes in PRs. |
| Source repo compromised (force-push, tag mutation) | Commit SHA is immutable. Tree hash catches any content change at that commit. |
| Man-in-the-middle during download | Tree hash computed after download must match lockfile. Any tampering detected. |
| Cached payload or generated adapters missing | `.ddx/plugins.lock.yaml` records the expected cache path and generated adapter paths. `ddx doctor --plugins` reports cache-missing or shims-missing. |
| Lockfile tampered with in a PR | Standard code review catches lockfile changes. Lockfile changes should be reviewed like dependency updates. |
| Registry serves different content to different users | Tree hash is deterministic. Two users fetching the same commit get the same hash or detect a discrepancy. |

### What This Does NOT Protect Against

- **First-fetch trust (TOFU):** The first time you install a package, you're trusting whatever the registry points to. Mitigated by: registry is a git repo with PR review.
- **Compromised developer machine:** If your local machine is compromised, all bets are off. Out of scope.
- **Upstream author goes rogue:** If the legitimate author publishes malicious content at a new version, the lockfile protects existing installs but new `ddx plugin upgrade` pulls the bad version. Mitigated by: developer reviews lockfile diffs.

## Future Tiers (Deferred)

### Tier 2: GitHub API Verification

Before trusting a commit, verify via GitHub API that it exists in the claimed repository:

```
GET /repos/{owner}/{repo}/commits/{sha}
```

This catches scenarios where someone provides a commit SHA from a fork or unrelated repo.

### Tier 3: GitHub Artifact Attestations

For packages that produce release artifacts, add attestations:

```yaml
# In the plugin repo's release workflow
- uses: actions/attest-build-provenance@v1
  with:
    subject-path: dist/plugin-v1.0.0.tar.gz
```

Consumers verify: `gh attestation verify plugin.tar.gz --repo org/repo`

This proves the artifact was built by the repo's CI from a specific commit.

### Tier 4: Sigstore Keyless Signing

For the strongest guarantee that content was published by a specific identity. Deferred because Tier 1 (lockfile pinning) is sufficient when the source repo is the authority.

## Consequences

- Every installed package is pinned by commit SHA and tree hash
- `.ddx/plugins.lock.yaml` must be committed to version control and reviewed in PRs
- `ddx doctor --plugins` can check lock, cache, and generated adapter state at any time without network access
- No external signing infrastructure required
- ~100 lines of Go for tree hashing + lockfile management
- Developers must review lockfile changes like they review `go.sum` changes

## Alternatives Considered

- **Sigstore/cosign:** Excellent but overkill when source repo is the authority. Adds infrastructure dependency. Deferred to Tier 4.
- **GPG signing:** Requires key management. Every package author needs a GPG key. Too much friction for a small ecosystem.
- **No integrity checking:** Unacceptable for a developer tool that installs executable code.
- **Only commit SHA (no tree hash):** Git SHA-1 has known weaknesses. Adding SHA-256 tree hash provides defense in depth.
