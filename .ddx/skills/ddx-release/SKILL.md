---
skill:
  name: ddx-release
  description: Prepare and publish a DDx release with full validation pipeline.
---

# DDx Release

Prepare and publish a DDx release. This is a sequential pipeline — each step
must pass before proceeding. If a step fails, fix the issue and restart from
that step.

## When to Use

- Cutting a new DDx release (tagged version)
- Verifying everything is green before tagging
- After major changes that touch CLI, skills, demos, or website

## Release Pipeline

### Phase 1: Clean Working Tree

Everything starts from a clean state. Commit all pending work, ensure
`.gitignore` covers generated artifacts, and clean up any stray files.

```bash
# Check current state
git status

# If there are untracked files that should be ignored, update .gitignore
# Common things to ignore:
#   cli/build/         - build artifacts
#   bin/               - local dev binaries
#   website/public/    - Hugo output
#   *.cast, *.gif in website/static/demos/ are TRACKED (they ship with the site)

# Stage and commit everything that should be tracked
git add -A
git status  # Review what's staged — no secrets, no binaries, no junk

# If untracked files remain that shouldn't be committed:
#   1. Add them to .gitignore and commit .gitignore
#   2. Or remove them: git clean -fd --dry-run  (preview first!)

# Commit all pending changes
git commit -m "chore: clean working tree for release"

# Verify clean state
git status
# Must show: "nothing to commit, working tree clean"
```

Do NOT proceed until `git status` shows a completely clean working tree.

### Phase 2: Tests

Run the full test suite. All tests must pass — no exceptions.

```bash
cd cli
go test -race ./...
```

If tests fail, fix them before proceeding. Do not skip failing tests.

### Phase 3: Lint & Format

```bash
cd cli
gofmt -l .        # Must produce no output
go vet ./...      # Must pass
```

If `golangci-lint` is available:
```bash
golangci-lint run --timeout=5m
```

### Phase 4: Build Cross-Platform

Verify the binary builds for all release platforms.

```bash
cd cli
make build-all
```

This builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.

### Phase 5: Demo Recordings + Website Build

Rebuild all demo screencasts and the Hugo microsite in Docker. The Docker
pipeline pins tool versions (asciinema, agg, Hugo extended) for reproducible,
environment-independent output.

```bash
bash scripts/demos/run-docker-demos.sh
```

This single command:
1. Records all demo screencasts
2. Renders .gif files from each cast
3. Builds the website with `hugo --gc --minify`

Then validate the cast files:

```bash
bash scripts/demos/validate-casts.sh
```

Both must succeed. If demos fail, the CLI behavior has changed — update the
demo scripts. Check `website/public/` for website output.

### Phase 6: Commit Demo & Website Updates

If Phase 5 produced changes (updated .cast/.gif files, website content):

```bash
git add website/static/demos/*.cast website/static/demos/*.gif
git commit -m "chore: regenerate demo recordings for release"
```

### Phase 7: Push & Verify CI

Push to main and verify GitHub Actions pass.

```bash
git push origin main
```

Then check:
- **CI workflow** (`ci.yml`): lint, build, tests, integration
- **Demos workflow** (`demos.yml`): demo recording validation
- Wait for both to go green before tagging.

To check CI status:
```bash
gh run list --limit 5
gh run view <run-id>
```

### Phase 8: Tag the Release

Only after CI is green:

```bash
VERSION="v0.X.0"  # Set the version
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"
```

This triggers the **Release workflow** (`release.yml`) which:
1. Validates the tag format
2. Runs the full test suite again
3. Builds binaries for all platforms with version info baked in
4. Creates GitHub Release with changelog and artifacts
5. Triggers **Pages workflow** (`pages.yml`) to rebuild the website with version info

### Phase 9: Verify Release

After the Release workflow completes:

```bash
# Check the release exists
gh release view "$VERSION"

# Verify the install script works with the new version
curl -fsSL https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh | DDX_VERSION="$VERSION" bash

# Verify the installed binary
ddx version
```

Also verify:
- GitHub Pages deployed with updated version: https://documentdrivendx.github.io/ddx/
- Release page has all platform archives and checksums

## Quick Reference: Step Dependencies

```
clean tree
  └─→ tests pass
       └─→ lint clean
            └─→ cross-platform build
                 └─→ demos + website built in Docker (validated)
                      └─→ commit updates
                           └─→ push + CI green
                                └─→ tag + release workflow
                                     └─→ verify release artifacts
```

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Tests fail | Fix the code, don't skip tests |
| Demo recording fails | Demo script needs updating for CLI changes |
| Hugo build fails | Check website/content/ for broken frontmatter or links |
| CI fails after push | Check `gh run view` for details, fix and push again |
| Release workflow fails | Usually a build issue — check the matrix build logs |
| Install script fails | Check install.sh for issues, or GitHub API rate limits |

## Environment Requirements

- Go (version matching `cli/go.mod`)
- Docker (for demo recordings + website build — pins asciinema, agg, Hugo versions)
- `gh` CLI (for checking CI status and releases)
- `golangci-lint` (optional, for comprehensive linting)
