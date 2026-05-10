The failure was reproduced on commit a3b4c5d6 with go test -race ./internal/docprose/...
across three consecutive runs; the race detector flagged concurrent map writes each time.

After applying the fix, the test suite passed with race detection enabled on 20 consecutive runs.
