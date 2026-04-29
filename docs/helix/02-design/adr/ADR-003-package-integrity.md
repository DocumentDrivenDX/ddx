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

1. User runs `ddx install helix`
2. DDx resolves the package to a specific git commit SHA from the registry
3. DDx clones/fetches at that exact commit (shallow, specific ref)
4. DDx computes a SHA-256 hash of the content tree (all files in sorted order)
5. DDx records `(repo, commit, tree_hash)` in `.ddx/lock.yaml`
6. DDx copies files to their install locations

#### On Subsequent Use

1. DDx reads `.ddx/lock.yaml`
2. On `ddx update`, DDx fetches the latest commit, computes the new tree hash, and shows the diff in the lockfile
3. The developer reviews and commits the lockfile change (like reviewing `go.sum` changes)

#### On Verification

1. `ddx verify` re-fetches all locked packages at their pinned commits
2. Re-computes tree hashes
3. Compares against `.ddx/lock.yaml`
4. Any mismatch = hard failure with clear error

### Lockfile Format

```yaml
# .ddx/lock.yaml — commit to version control, review changes in PRs
version: 1
packages:
  helix:
    repo: https://github.com/DocumentDrivenDX/helix
    commit: abc123def456789012345678901234567890abcd
    tree_hash: sha256:7c6f43f4a3b2e1d0...
    installed_at: 2026-04-04T12:00:00Z
    files:
      - path: .agents/skills/helix-run   # project-local (FEAT-015: home paths retired)
        hash: sha256:...
      - path: .agents/skills/helix-build
        hash: sha256:...
      - path: ~/.local/bin/helix
        hash: sha256:...
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

### Per-File Hashes

In addition to the tree hash (which covers the source), each installed file gets its own SHA-256 hash recorded in the lockfile. This enables:

- `ddx verify` to check installed files haven't been tampered with post-install
- Detection of local modifications to installed skills/plugins
- Fast verification (check file hashes) without re-fetching from git

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
| Installed files modified post-install | Per-file hashes in lockfile. `ddx verify` detects modifications. |
| Lockfile tampered with in a PR | Standard code review catches lockfile changes. Lockfile changes should be reviewed like dependency updates. |
| Registry serves different content to different users | Tree hash is deterministic. Two users fetching the same commit get the same hash or detect a discrepancy. |

### What This Does NOT Protect Against

- **First-fetch trust (TOFU):** The first time you install a package, you're trusting whatever the registry points to. Mitigated by: registry is a git repo with PR review.
- **Compromised developer machine:** If your local machine is compromised, all bets are off. Out of scope.
- **Upstream author goes rogue:** If the legitimate author publishes malicious content at a new version, the lockfile protects existing installs but new `ddx update` pulls the bad version. Mitigated by: developer reviews lockfile diffs.

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
- `.ddx/lock.yaml` must be committed to version control and reviewed in PRs
- `ddx verify` can check integrity at any time without network access (for installed file hashes)
- No external signing infrastructure required
- ~100 lines of Go for tree hashing + lockfile management
- Developers must review lockfile changes like they review `go.sum` changes

## Alternatives Considered

- **Sigstore/cosign:** Excellent but overkill when source repo is the authority. Adds infrastructure dependency. Deferred to Tier 4.
- **GPG signing:** Requires key management. Every package author needs a GPG key. Too much friction for a small ecosystem.
- **No integrity checking:** Unacceptable for a developer tool that installs executable code.
- **Only commit SHA (no tree hash):** Git SHA-1 has known weaknesses. Adding SHA-256 tree hash provides defense in depth.
