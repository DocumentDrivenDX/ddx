# Verification

No code changes were required for this bead. The current `cli/cmd` implementation already routes project installs and updates through the resolved project root, and the bead-specific tests are present in the tree.

## Evidence

- Install routing: `cli/cmd/install.go:181-231`
- Update routing: `cli/cmd/update.go:201-310`
- Install coverage: `cli/cmd/install_test.go:107-318`
- Update coverage: `cli/cmd/update_test.go:340-495`

## Commands

- `cd cli && go test ./cmd -run 'TestInstall_ConventionMode|TestUpdate_RespectsConventionVsInTreeMode|TestInstallProject_ConventionLinks|TestUpdate'`
- `cd cli && PATH=/home/linuxbrew/.linuxbrew/bin:/usr/bin:/bin HOME=/tmp/ddx-clean-home go test ./cmd/...`
- `cd cli && lefthook run pre-commit`

## Notes

- The full `go test ./cmd/...` run passes with a clean `HOME` and `PATH` that exclude the host's stale `ddx` install.
- The host-environment `PATH` run surfaced unrelated diagnostics noise, but the bead-relevant install/update paths and tests are green.
