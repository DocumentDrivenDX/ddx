DELETE internal/testutils/fixture_repo.go:23 NewFixtureRepo - test-only fixture helper removed; consumers now inline their setup in tests.
DELETE internal/testutils/fixture_repo.go:50 repoRoot - helper was only used by the deleted test-only fixture builder.
DELETE internal/testutils/fixture_repo.go:79 ResolveDDxBinary - helper was only used by the deleted test-only fixture builder.
DELETE internal/testutils/fixture_repo.go:88 resolveDDxBinary - helper was only used by the deleted test-only fixture builder.
DELETE internal/testutils/testutils.go:21 NewTestEnvironment - test-only helper package deleted; cmd/internal tests already have local environment helpers.
DELETE internal/testutils/testutils.go:45 TestEnvironment.Cleanup - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:54 TestEnvironment.HomeDir - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:59 TestEnvironment.WorkDir - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:64 TestEnvironment.CreateFile - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:72 TestEnvironment.CreateHomeFile - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:80 TestEnvironment.CreateTemplate - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:92 TestEnvironment.CreateConfig - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:97 TestEnvironment.CreateGlobalConfig - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:102 TestEnvironment.AssertFileExists - deleted with the test-only environment helper.
DELETE internal/testutils/testutils.go:108 TestEnvironment.AssertFileContent - deleted with the test-only environment helper.
