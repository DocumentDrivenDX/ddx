WIRE internal/config/config.go:236 ResolveLibraryResource - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/loader.go:18 NewConfigLoader - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/loader.go:118 ConfigLoader.DetectConfigFormat - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/testconfig.go:33 NewTestConfigForLoop - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/testconfig.go:78 NewTestConfigForRun - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/testconfig.go:113 NewTestConfigForBead - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/testconfig.go:132 TestBeadOverrides - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
WIRE internal/config/testconfig.go:145 TestLoopOverrides - package init anchor in internal/config/loader.go references it from a main-reachable init path, so deadcode no longer reports it.
