WIRE internal/config/config.go:236 ResolveLibraryResource — reachable from production through `config.LoadAndResolve` (`cli/internal/config/resolved.go:147`) and the config command / agent command loaders that call `LoadWithWorkingDir`.
WIRE internal/config/loader.go:18 NewConfigLoader — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/loader.go:118 ConfigLoader.DetectConfigFormat — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/testconfig.go:33 NewTestConfigForLoop — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/testconfig.go:78 NewTestConfigForRun — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/testconfig.go:113 NewTestConfigForBead — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/testconfig.go:132 TestBeadOverrides — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
WIRE internal/config/testconfig.go:145 TestLoopOverrides — reachable from the production reachability anchor in `LoadAndResolve` (`cli/internal/config/resolved.go:147`).
