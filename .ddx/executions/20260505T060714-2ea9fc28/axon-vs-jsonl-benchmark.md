# Axon vs JSONL Benchmark

Run command:

```sh
cd cli && go test -run '^$' -bench=. ./internal/bead/...
```

Corpus shape:

- 1100 total beads
- Open beads with ready and blocked dependency shapes
- Recent closed beads that remain active
- Old closed beads that archive into the axon/archive partner
- Inline event history on the synthetic rows so the axon split-store path is exercised

Results from the final run:

| Benchmark | JSONL ns/op | JSONL B/op | JSONL allocs/op | Axon ns/op | Axon B/op | Axon allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Ready | 29,411,986 | 17,925,533 | 68,010 | 20,261,003 | 10,126,264 | 67,656 |
| Blocked | 29,323,935 | 17,843,549 | 68,009 | 19,872,045 | 10,044,263 | 67,654 |
| Show | 29,302,569 | 17,684,733 | 67,745 | 19,582,076 | 9,885,623 | 67,391 |

Summary:

- Axon is faster on wall-time for Ready, Blocked, and Show.
- Axon uses fewer bytes/op and slightly fewer allocs/op on all three benchmarks.
- The benchmark is reproducible with `go test -run '^$' -bench=. ./internal/bead/...` from `cli/`.
