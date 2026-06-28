Bead verification for ddx-b85090ae

Summary:
- The requested ancestry rejection is already present in the current branch tip.
- No source files required changes for this bead.

Evidence:
- `cd cli && go test ./internal/bead/... -run TestBeadDepAddRejectsParentAncestor`
- `cd cli && go test ./cmd/... -run TestBeadCreateRejectsDependsOnParentAncestor`
- `cd cli && go test ./internal/bead/... ./cmd/...`
- `cd cli && lefthook run pre-commit`

Relevant code paths already enforce the contract:
- `cli/internal/bead/store.go`
- `cli/cmd/bead.go`
- `cli/internal/bead/store_test.go`
- `cli/cmd/bead_create_dep_test.go`
