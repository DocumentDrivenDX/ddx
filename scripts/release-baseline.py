#!/usr/bin/env python3
"""Select the previous published GitHub Release tag for a release commit.

GitHub Release metadata is supplied by the caller so this helper remains
network-free and hermetic. Tag reachability is proved against the local git
repository; tag existence alone is never treated as publication evidence.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any


class BaselineError(RuntimeError):
    """Raised when a safe changelog baseline cannot be established."""


RELEASE_TAG_RE = re.compile(r"^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+)?$")
COMMIT_SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")


def git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
        check=check,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


def peel_commit(repo: Path, revision: str) -> str:
    if not COMMIT_SHA_RE.fullmatch(revision):
        raise BaselineError(f"release commit must be a full 40-character SHA, got {revision!r}")
    result = git(repo, "rev-parse", "--verify", f"{revision}^{{commit}}", check=False)
    if result.returncode != 0:
        detail = result.stderr.strip() or "revision was not found"
        raise BaselineError(f"cannot resolve release commit {revision!r}: {detail}")
    return result.stdout.strip()


def load_release_rows(path: str) -> list[dict[str, Any]]:
    try:
        if path == "-":
            payload = json.load(sys.stdin)
        else:
            with Path(path).open(encoding="utf-8") as handle:
                payload = json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        raise BaselineError(f"cannot read GitHub Release metadata from {path!r}: {exc}") from exc

    if isinstance(payload, dict) and "releases" in payload:
        payload = payload["releases"]
    if not isinstance(payload, list):
        raise BaselineError("GitHub Release metadata must be a JSON array")

    rows: list[dict[str, Any]] = []
    for item in payload:
        if isinstance(item, list):
            rows.extend(row for row in item if isinstance(row, dict))
        elif isinstance(item, dict):
            rows.append(item)
    return rows


def published_candidates(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    candidates = []
    for row in rows:
        tag = str(row.get("tag_name") or "").strip()
        published_at = str(row.get("published_at") or "").strip()
        if not tag or row.get("draft") is not False or not published_at:
            continue
        candidates.append(row)
    return sorted(
        candidates,
        key=lambda row: (
            str(row.get("published_at") or ""),
            str(row.get("created_at") or ""),
            str(row.get("tag_name") or ""),
        ),
        reverse=True,
    )


def select_baseline(
    repo: Path, release_tag: str, release_commit: str, rows: list[dict[str, Any]]
) -> dict[str, str]:
    if not RELEASE_TAG_RE.fullmatch(release_tag):
        raise BaselineError(f"release tag does not match the DDx release format: {release_tag!r}")
    release_sha = peel_commit(repo, release_commit)
    for row in published_candidates(rows):
        tag = str(row["tag_name"]).strip()
        if tag == release_tag:
            continue
        if not RELEASE_TAG_RE.fullmatch(tag):
            sys.stderr.write(
                f"warning: published GitHub Release tag {tag!r} does not match the DDx release format; skipping\n"
            )
            continue
        tag_result = git(repo, "rev-parse", "--verify", f"refs/tags/{tag}^{{commit}}", check=False)
        if tag_result.returncode != 0:
            sys.stderr.write(
                f"warning: published GitHub Release tag {tag!r} is unavailable locally; skipping\n"
            )
            continue
        tag_sha = tag_result.stdout.strip()
        reachable = git(repo, "merge-base", "--is-ancestor", tag_sha, release_sha, check=False)
        if reachable.returncode == 0:
            return {"tag": tag, "commit": tag_sha}
        if reachable.returncode not in (0, 1):
            detail = reachable.stderr.strip() or "git merge-base failed"
            raise BaselineError(f"cannot prove reachability for published release {tag!r}: {detail}")

    raise BaselineError(
        "no previous published GitHub Release tag is reachable from release commit "
        f"{release_sha}; refusing nearest-tag fallback"
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="select the newest reachable published, non-draft GitHub Release tag"
    )
    parser.add_argument("--release-tag", required=True)
    parser.add_argument("--release-commit", required=True)
    parser.add_argument("--releases-json", required=True, help="GitHub Releases JSON file, or -")
    parser.add_argument("--repo", default=".", help="git repository (default: current directory)")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        baseline = select_baseline(
            Path(args.repo).resolve(),
            args.release_tag,
            args.release_commit,
            load_release_rows(args.releases_json),
        )
    except BaselineError as exc:
        sys.stderr.write(f"release baseline error: {exc}\n")
        return 1
    sys.stdout.write(json.dumps(baseline, sort_keys=True) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
