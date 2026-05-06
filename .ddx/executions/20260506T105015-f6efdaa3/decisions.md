`internal/metaprompt/NewMetaPromptInjectorWithPaths` -> WIRE
- `cmd/command_factory.go:177-178` roots the package with `metaprompt.KeepReachabilityForDeadcode()`.
- `cmd/init.go:706-720` and `cmd/doctor.go:917-936` are the production call sites that instantiate the injector for `init` and `doctor`.
- Verified by `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...`, which reports no remaining `internal/metaprompt` dead symbols.
