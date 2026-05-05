ddx-a78f836f decisions

- internal/testutils/fixture_repo.go:23 — DELETE: `cli/internal/testutils` is absent from this checkout; the corresponding test-only helper surface now lives in `cli/cmd/testutils_test.go` and is not part of the production build graph.
- internal/testutils/fixture_repo.go:50 — DELETE: same as above; no reachable production call path remains for `repoRoot`.
- internal/testutils/fixture_repo.go:79 — DELETE: same as above; `ResolveDDxBinary` is no longer present under `cli/`.
- internal/testutils/fixture_repo.go:88 — DELETE: same as above; `resolveDDxBinary` is no longer present under `cli/`.
- internal/testutils/testutils.go:21 — DELETE: same as above; `NewTestEnvironment` is now defined in `cli/cmd/testutils_test.go` for tests only.
- internal/testutils/testutils.go:45 — DELETE: same as above; `TestEnvironment.Cleanup` is not a production symbol in this tree.
- internal/testutils/testutils.go:54 — DELETE: same as above; `TestEnvironment.HomeDir` is not a production symbol in this tree.
- internal/testutils/testutils.go:59 — DELETE: same as above; `TestEnvironment.WorkDir` is not a production symbol in this tree.
- internal/testutils/testutils.go:64 — DELETE: same as above; `TestEnvironment.CreateFile` is not a production symbol in this tree.
- internal/testutils/testutils.go:72 — DELETE: same as above; `TestEnvironment.CreateHomeFile` is not a production symbol in this tree.
- internal/testutils/testutils.go:80 — DELETE: same as above; `TestEnvironment.CreateTemplate` is not a production symbol in this tree.
- internal/testutils/testutils.go:92 — DELETE: same as above; `TestEnvironment.CreateConfig` is not a production symbol in this tree.
- internal/testutils/testutils.go:97 — DELETE: same as above; `TestEnvironment.CreateGlobalConfig` is not a production symbol in this tree.
- internal/testutils/testutils.go:102 — DELETE: same as above; `TestEnvironment.AssertFileExists` is not a production symbol in this tree.
- internal/testutils/testutils.go:108 — DELETE: same as above; `TestEnvironment.AssertFileContent` is not a production symbol in this tree.
