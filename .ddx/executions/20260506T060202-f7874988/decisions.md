# ddx-ae4b7393 decisions

- `internal/evidence/read.go:24` `OversizeError.Error` - WIRE: used by the CLI reachability anchor in `cmd/root.go:45` and by production oversize handling in `internal/agent/prompt_file_read.go:29-33`.
- `internal/evidence/read.go:29` `OversizeError.Unwrap` - WIRE: used by the CLI reachability anchor in `cmd/root.go:46` and by production `errors.Is` handling in `internal/agent/prompt_file_read.go:31-33`.
- `internal/evidence/read.go:70` `ReadFileHardFail` - WIRE: used by production prompt ingress in `internal/agent/prompt_file_read.go:29-33` and by the CLI reachability anchor in `cmd/root.go:47`.
- `internal/evidence/sections.go:51` `FitSections` - WIRE: used by production prompt assembly in `internal/server/review_session_prompt.go:56-92` and by the CLI reachability anchor in `cmd/root.go:48`.
- `internal/evidence/sections.go:139` `capContent` - WIRE: reachable through `FitSections`, which is used by production prompt assembly in `internal/server/review_session_prompt.go:56-92`.
- `internal/evidence/sections.go:153` `trimToLineBudget` - WIRE: reachable through `FitSections`, which is used by production prompt assembly in `internal/server/review_session_prompt.go:56-92`.
- `internal/evidence/strategy.go:19` `AssembleRefOnly` - DELETE: the current tree does not define this symbol; the originating reachability issue appears to have been renamed away before this bead was executed, and the production prompt assembly now uses `AssembleInline` instead.
- `internal/evidence/strategy.go:50` `AssembleInline` - WIRE: used by production prompt assembly in `internal/server/review_session_prompt.go:56-92` and by the CLI reachability anchor in `cmd/root.go:49`.
