# Production Reachability Resolution: artifacttypes/types.go

## Summary
Resolved all deadcode findings for `cli/internal/artifacttypes/types.go` by deleting three unreachable helper methods that have no production callers.

## Symbols Analyzed

### 1. Index.ByPlugin (line 56)
**Status**: DELETE  
**Reasoning**: 
- Never called in any production code
- Never called in any test code
- No clear use case in the current production patterns
- Internal helper that doesn't fit the current registry usage model

### 2. Index.Lookup (line 72)
**Status**: DELETE  
**Reasoning**:
- Only called in test files (types_test.go, loader_test.go)
- Production code in resolver_artifact_types.go uses direct iteration over index.Types instead
- Tests verify the method works, but no production caller exists
- Current filtering pattern (multiple roots, iteration, deduplication) doesn't benefit from point lookups

### 3. Index.LookupPrefix (line 84)
**Status**: DELETE  
**Reasoning**:
- Only called in test files (types_test.go, loader_test.go)
- Production code iterates through multiple artifact roots and needs ALL matching types, not just the first one
- Current production pattern requires deduplication across roots, incompatible with single-root prefix lookup
- Tests verify the method works, but no production caller exists

## Production Code Analysis
Reviewed `cli/internal/server/graphql/resolver_artifact_types.go` (artifactTypeDefinitionsForPath, lines 43-66):
- Loads indexes from multiple roots
- Directly accesses index.Types
- Filters by prefix in-loop
- Deduplicates results across roots

This pattern does not use the deleted methods and represents the canonical way artifact types are accessed in the codebase.

## Test Impact
Both `types_test.go` and `loader_test.go` call these methods. Deleting them requires updating tests to verify Index behavior through direct inspection of index.Types instead.

## Verification
After deletion:
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | rg 'internal/artifacttypes/types\.go'` returns no hits
- All tests pass (`cd cli && go test ./...`)
- All pre-commit checks pass (`lefthook run pre-commit`)
