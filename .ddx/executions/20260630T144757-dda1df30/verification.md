# Verification

Bead: `ddx-835b13ca`

## Checks

- `cd cli && go test ./cmd/... ./internal/registry/... -run 'TestInitGlobal_CreatesAgentTierLinks|TestInitGlobal_WritesGlobalConfig|TestInitProject_DoesNotInstallPlugins|TestInitProject_LeavesPluginsForLazyResolution|TestPluginLookup_PrefersProjectOverGlobal|TestPluginLookup_FallsBackToGlobal|TestPluginLookup_BakedInDefaultOnly' -count=1`
- `cd cli && go test ./cmd/... ./internal/registry/...`

## Result

Both commands passed. The current implementation already matches the bead's
requested init and plugin-resolution behavior, so no source changes were
required for this execution.
