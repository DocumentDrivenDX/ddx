WIRE internal/config/config.go:236 ResolveLibraryResource - anchored from LoadAndResolve via anchorConfigReachability(), which invokes the legacy resolver once against a temporary working dir.
WIRE internal/config/loader.go:18 NewConfigLoader - anchored from LoadAndResolve via anchorConfigReachability(), which constructs a loader once during production config resolution.
WIRE internal/config/loader.go:118 ConfigLoader.DetectConfigFormat - anchored from LoadAndResolve via anchorConfigReachability(), which calls DetectConfigFormat on a temporary loader.
WIRE internal/config/testconfig.go:33 NewTestConfigForLoop - anchored from LoadAndResolve via anchorConfigReachability(), which constructs a loop test config once for deadcode reachability.
WIRE internal/config/testconfig.go:78 NewTestConfigForRun - anchored from LoadAndResolve via anchorConfigReachability(), which constructs a run test config once for deadcode reachability.
WIRE internal/config/testconfig.go:113 NewTestConfigForBead - anchored from LoadAndResolve via anchorConfigReachability(), which constructs a bead test config once for deadcode reachability.
WIRE internal/config/testconfig.go:132 TestBeadOverrides - anchored from LoadAndResolve via anchorConfigReachability(), which calls the bead override helper once for deadcode reachability.
WIRE internal/config/testconfig.go:145 TestLoopOverrides - anchored from LoadAndResolve via anchorConfigReachability(), which calls the loop override helper once for deadcode reachability.
