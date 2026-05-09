//go:build perf && !race

package perf

// raceEnabled reports whether the test binary was built with `-race`.
// Perf budgets are only meaningful in non-race builds: the race detector
// adds 5-10x synchronisation overhead that invalidates microbenchmark
// percentile gates. Build-tag pair lives in race_off_test.go (false) and
// race_on_test.go (true) so callers see one constant either way.
const raceEnabled = false
