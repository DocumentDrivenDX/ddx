# Releasing DDx

This checklist is the durable operator contract for a DDx binary release. A
release is complete only when its tag, workflow, nine assets, checksums, binary
metadata, and installer behavior have been verified and the evidence record is
filled in.

The release workflow ties every checkout and build to the commit peeled from
the requested tag. That provides source traceability and artifact integrity,
not bit-for-bit reproducibility: runner images, actions, Bun, and build
timestamps are not all pinned. Do not describe two runs for the same commit as
producing byte-identical archives.

## 1. Prepare the candidate

Set the proposed tag and update the local view of `main` and all tags:

```bash
TAG=vX.Y.Z-alphaN        # or vX.Y.Z for a stable release
git fetch origin main --tags
git switch main
git pull --ff-only origin main
COMMIT=$(git rev-parse HEAD)
test -z "$(git status --porcelain)"
```

Confirm that the tag does not already exist locally or remotely. Published
release tags are immutable and must never be replaced.

```bash
! git show-ref --verify --quiet "refs/tags/$TAG"
test -z "$(git ls-remote --tags origin "refs/tags/$TAG")"
```

Run the repository gates sequentially from the candidate commit:

```bash
cd cli
go test ./... -count=1 -timeout 600s
cd ..
lefthook run pre-commit
```

Review the commits since the prior release, the proposed version, and any
operator-facing migration notes. Record a pre-tag Go/No-Go decision before
creating the tag.

## 2. Create and push one annotated tag

Create the annotated tag at the already-verified commit, inspect its peeled
commit, and push only that tag:

```bash
git tag -a "$TAG" "$COMMIT" -m "Release $TAG"
TAG_COMMIT=$(git rev-parse "${TAG}^{commit}")
test "$TAG_COMMIT" = "$COMMIT"
git show --no-patch --decorate "$TAG"
git push origin "$TAG"
```

A `v*` tag push starts `.github/workflows/release.yml`. The workflow resolves
the pushed tag, verifies the checkout, and carries the resulting commit through
every job.

Manual dispatch is a recovery entry point for an existing, already-pushed tag.
It is not a way to release a branch or an unpushed tag:

```bash
gh workflow run release.yml --ref main -f tag="$TAG"
```

Both entry points must resolve the same `TAG_COMMIT`. Never move, delete, or
force-push a release tag to change the source of an existing release.

## 3. Verify the workflow and release identity

Find the run for the tag, wait for it, and require a successful conclusion:

```bash
gh run list --workflow release.yml --limit 20
RUN_ID=<run-id-for-$TAG>
gh run watch "$RUN_ID" --exit-status
gh run view "$RUN_ID" --json conclusion,url,jobs
```

Record `Release workflow conclusion: success`. Compare the remote tag with the
local peeled commit and inspect the release metadata:

```bash
git fetch origin --tags
test "$(git rev-parse "${TAG}^{commit}")" = "$TAG_COMMIT"
gh release view "$TAG" --json tagName,isPrerelease,url
```

For a hyphenated tag, `isPrerelease` must be true. For `vX.Y.Z`, it must be
false. The release page tag and name must both identify `$TAG`.

## 4. Verify all nine assets

The GitHub release must contain exactly these four archives, their four
sidecar checksums, and the aggregate checksum file:

| # | Required asset |
|---:|---|
| 1 | `ddx-linux-amd64.tar.gz` |
| 2 | `ddx-linux-amd64.tar.gz.sha256` |
| 3 | `ddx-linux-arm64.tar.gz` |
| 4 | `ddx-linux-arm64.tar.gz.sha256` |
| 5 | `ddx-darwin-amd64.tar.gz` |
| 6 | `ddx-darwin-amd64.tar.gz.sha256` |
| 7 | `ddx-darwin-arm64.tar.gz` |
| 8 | `ddx-darwin-arm64.tar.gz.sha256` |
| 9 | `checksums.sha256` |

Download the release into an empty directory and compare the observed names
with the table before running checksum validation:

```bash
ASSET_DIR=$(mktemp -d)
gh release download "$TAG" --dir "$ASSET_DIR"
find "$ASSET_DIR" -maxdepth 1 -type f -printf '%f\n' | sort
(
  cd "$ASSET_DIR"
  for sidecar in ddx-*.tar.gz.sha256; do
    sha256sum -c "$sidecar"
  done
  sha256sum -c checksums.sha256
)
```

All eight archive checks must report `OK`: four via the sidecars and the same
four via `checksums.sha256`. A missing asset, extra unexpected release archive,
empty aggregate file, or checksum mismatch is a No-Go.

## 5. Run binary metadata smoke checks

At minimum, execute the Linux amd64 binary and inspect the archive metadata.
The reported version and commit must match the release identity:

```bash
SMOKE_DIR=$(mktemp -d)
tar -xzf "$ASSET_DIR/ddx-linux-amd64.tar.gz" -C "$SMOKE_DIR"
"$SMOKE_DIR/ddx" version | tee "$SMOKE_DIR/version.txt"
grep -F "DDx $TAG" "$SMOKE_DIR/version.txt"
grep -F "Commit: $TAG_COMMIT" "$SMOKE_DIR/version.txt"
grep -F "DDx $TAG" "$SMOKE_DIR/VERSION"
grep -F "Commit: $TAG_COMMIT" "$SMOKE_DIR/VERSION"
```

Repeat the archive inspection for the other three platforms. Run a native
`./ddx version` smoke check on each platform available to the release operator.

## 6. Verify installer selection

The default installer follows GitHub's latest stable release. A prerelease must
be selected explicitly with `DDX_VERSION`; it must not replace the default
stable selection.

```bash
INSTALL_SCRIPT=https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh
DEFAULT_PREFIX=$(mktemp -d)
curl -fsSL "$INSTALL_SCRIPT" | INSTALL_PREFIX="$DEFAULT_PREFIX" bash
"$DEFAULT_PREFIX/bin/ddx" version

EXPLICIT_PREFIX=$(mktemp -d)
curl -fsSL "$INSTALL_SCRIPT" \
  | DDX_VERSION="$TAG" INSTALL_PREFIX="$EXPLICIT_PREFIX" bash
"$EXPLICIT_PREFIX/bin/ddx" version | tee "$EXPLICIT_PREFIX/version.txt"
grep -F "DDx $TAG" "$EXPLICIT_PREFIX/version.txt"
grep -F "Commit: $TAG_COMMIT" "$EXPLICIT_PREFIX/version.txt"
```

For a prerelease, confirm the default installation did not report `$TAG` and
the explicit installation did. For a stable release, confirm both resolve to
the intended stable tag once GitHub marks it latest.

## 7. Record Go/No-Go evidence

Attach or paste this completed record into the release audit:

```text
Release tag:
Tag commit SHA:
Previous tag:
Pre-tag Go/No-Go:
Pre-commit result:
Full Go test result:
Workflow URL:
Release workflow conclusion: success
Release URL:
Nine assets present:
Sidecar checksum result:
Aggregate checksum result:
Binary version and commit smoke result:
Default installer selected tag:
Explicit prerelease install selected tag:
Final Go/No-Go:
Operator:
Verified at (UTC):
```

The final decision is Go only when every field has affirmative, attributable
evidence. Preserve the workflow and release URLs so the decision can be
rechecked later.

## Failure and recovery

- Before pushing a tag, fix the candidate and rerun all gates.
- If the workflow fails after the immutable tag is pushed but before a usable
  release exists, fix the workflow on `main` and manually dispatch the same tag.
  Record both run URLs; do not claim that rerun archives are byte-identical.
- If a release is published with bad binaries, mark it as affected and cut a
  higher patch or prerelease tag from the corrected commit. Do not replace the
  tag, rewrite its commit, or rewrite branch history.
- Never bypass the Go, pre-commit, release workflow, checksum, binary, or
  installer checks to turn a No-Go into a Go.
