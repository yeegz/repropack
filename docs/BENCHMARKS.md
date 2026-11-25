# Benchmarks and fuzzing

These measurements come from three `go test -benchmem` repetitions on an Apple M1 Pro (`darwin/arm64`, Go 1.27.4, eight logical CPUs available to the benchmark). The table gives the median of each reported column. Inputs are synthetic and in memory, so the figures describe processing and allocations rather than file-system latency, peak memory or desktop responsiveness. [Raw benchmark log](bench-raw.txt)

| Operation | Time per operation | Bytes allocated per operation | Allocations per operation |
| --- | ---: | ---: | ---: |
| Record a log, redaction enabled, no attributes | 714.1 ns | 208 B | 4 |
| Record a log, redaction disabled, no attributes | 129.3 ns | 160 B | 1 |
| Record a log with four attributes, redaction enabled | 2,018 ns | 1,025 B | 15 |
| Discard a log below the configured level | 7.126 ns | 0 B | 0 |
| `slog` JSON handler without recorder | 517.7 ns | 0 B | 0 |
| `slog` JSON handler with recorder | 2,237 ns | 978 B | 13 |
| Write a report from 2,000 buffered events | 2.84 ms | 1,112,631 B | 20,082 |
| Write a report with 10,000 events | 18.20 ms | 6,872,420 B | 140,099 |
| Open and verify 10,000 events and a 2 MiB attachment | 19.02 ms | 6,022,307 B | 110,156 |
| Read 10,000 events into a slice | 18.20 ms | 7,618,000 B | 110,110 |
| Reject an archive with an unsafe path | 2.60 µs | 6,981 B | 31 |
| Scan 1 KiB of text with the default redaction policy | 108.07 µs | 10,673 B | 77 |
| Build the viewer model for 50,000 events | 129.18 ms | 89,639,725 B | 650,725 |
| Render the standalone HTML viewer for 50,000 input events | 178.47 ms | 87,461,278 B | 1,353,421 |

Run from the source checkout. Change the output path to retain both sets of results when comparing machines or revisions.

```bash
scripts/bench.sh > benchmark-results.txt
```

## Fuzzing

The recorded run started at 06:21 UTC on November 25, 2026. Each target ran for 90 seconds on the same Go version and platform. All four exited successfully. [Raw fuzz log](fuzz-raw.txt)

| Target | Executions |
| --- | ---: |
| Report reader | 11,001,888 |
| Event line parser | 1,202,437 |
| Text redaction | 1,221,396 |
| OTLP JSON import | 2,598,910 |

```bash
scripts/fuzz.sh 90s > fuzz-results.txt
```
