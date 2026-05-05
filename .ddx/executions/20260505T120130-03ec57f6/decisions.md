| Symbol | Decision | Reason |
| --- | --- | --- |
| internal/metaprompt/injector.go:44 — NewMetaPromptInjector | WIRE | The current tree wires the metaprompt injector through `cli/cmd/init.go` and `cli/cmd/doctor.go`, and `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` from `cli/` reports no remaining `internal/metaprompt` dead symbols. |
