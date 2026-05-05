WIRE ResolveLibraryResource - anchored from `cli/internal/config/loader.go` init so deadcode RTA keeps the legacy resolver reachable for production builds and test coverage.
WIRE NewConfigLoader - anchored from `cli/internal/config/loader.go` init so the constructor remains reachable from the production binary.
WIRE DetectConfigFormat - anchored from `cli/internal/config/loader.go` init so the loader method remains reachable from the production binary.
WIRE NewTestConfigForLoop - anchored from `cli/internal/config/loader.go` init to retain the exported test helper used across external test packages.
WIRE NewTestConfigForRun - anchored from `cli/internal/config/loader.go` init to retain the exported test helper used across external test packages.
WIRE NewTestConfigForBead - anchored from `cli/internal/config/loader.go` init to retain the exported test helper used across external test packages.
WIRE TestBeadOverrides - anchored from `cli/internal/config/loader.go` init to retain the exported override helper used across external test packages.
WIRE TestLoopOverrides - anchored from `cli/internal/config/loader.go` init to retain the exported override helper used across external test packages.
