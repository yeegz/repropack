# ReproPack

**One file that explains a bug.** ReproPack packages reproduction steps, screenshots, logs and diagnostics into a `.repropack` file. Review private details before sharing it with a maintainer. Capture and inspection run locally, with no account or server.

[Open the sample report](spec/examples/checkout-crash.html) · [View the local showcase](docs/showcase/index.html)

It is four pieces you can adopt separately:

| Piece | What it's for | Path |
|---|---|---|
| **Report format** | An open, versioned, ZIP + JSON format with integrity hashes and a redaction record | [`spec/SPEC.md`](spec/SPEC.md), [`format/`](format) |
| **Go SDK** | Keep a bounded, redacted buffer of diagnostics beside your existing logging; write a report on demand or on panic | [`sdk/`](sdk) |
| **CLI** | Validate, inspect, redact, convert (OpenTelemetry) and summarise reports | [`cmd/repropack`](cmd/repropack) |
| **Desktop app** | Guided capture for non-technical people, plus a viewer for the reports you receive (macOS today) | [`desktop/`](desktop) |

The core module (`format`, `redact`, `sdk`, `otlp`, `ingest`, `viewer`, CLI) uses **only the Go standard library**. The desktop app is a separate Go module so SDK users never pull in UI dependencies.

## Why

Maintainers often have to ask for the steps, logs and screenshots needed to reproduce the same problem. Those files can also contain credentials or personal information. ReproPack brings the evidence and the privacy review into the same workflow. It can sit alongside an existing crash tracker or logging setup. [Research notes](docs/research/PRIOR-ART.md)

ReproPack is that bundle:

- **Local-first.** Reports are files. Share them however you like, or don't.
- **Redaction you can check.** Every report records which rules removed how many values (never the values), whether the result was re-scanned, and which images a person reviewed.
- **Untrusted by default.** Readers enforce path rules, size and compression limits, and per-file SHA-256. Viewers never execute report content.
- **Readable without us.** `unzip` plus a text editor works. So does any language with ZIP and JSON.
- **Interoperable.** Events map to OpenTelemetry log records, in and out.

## Quick start

### CLI

From the source checkout, open the bundled checkout-failure report with Go 1.24 or later.

```bash
go build -o bin/repropack ./cmd/repropack
./bin/repropack validate spec/examples/checkout-crash.repropack
./bin/repropack inspect spec/examples/checkout-crash.repropack
./bin/repropack html spec/examples/checkout-crash.repropack -o checkout.html
```

Open `checkout.html` in a browser to read the evidence timeline offline. Use local log and screenshot files to start your own report.

```bash
./bin/repropack create -o crash.repropack --title "Checkout freezes" --log app.log --attach shot.png --step "Press Pay"
./bin/repropack scan crash.repropack
./bin/repropack redact crash.repropack -o clean.repropack
./bin/repropack validate clean.repropack
```

`scan` returns exit status 1 when it finds possible sensitive text. Review images and recordings yourself before sharing a report. [Installation and verification](docs/install.md)

### SDK (alongside your existing logger)

Start with the [local SDK installation](docs/install.md#sdk). The recorder wraps your existing `log/slog` handler.

```go
rec := sdk.New(sdk.Options{
    App:       format.AppContext{Name: "billing", Version: "1.4.2"},
    MaxEvents: 2000,                 // bounded ring buffer; oldest dropped first
    Providers: []sdk.Provider{sdk.RuntimeProvider()},
})
logger := slog.New(rec.Handler(slog.NewJSONHandler(os.Stderr, nil))) // your output is unchanged
defer rec.CapturePanic("crash.repropack")                            // writes a report, then re-panics

logger.Warn("Invoice total is wrong", "invoice", "INV-2041")
if _, err := rec.WriteFile("report.repropack", sdk.ReportOptions{Title: "Invoice total is wrong"}); err != nil {
    logger.Error("Could not save report", "error", err)
}
```

More in the [SDK guide](docs/sdk.md) and [`examples/`](examples): a minimal program, an existing-slog setup, and an HTTP service with per-request breadcrumbs and spans, panic capture, and a loopback-only report endpoint.

### Desktop app (macOS)

```bash
cd desktop && wails build    # needs Go 1.26+, Node 20+, Xcode CLT, wails v2
```

Then open `desktop/build/bin/ReproPack.app`. Full steps: [docs/install.md](docs/install.md).

The app walks through **Describe → Capture → Review → Share**:

- **Capture.** Screenshot a window or area, or record the screen, always through the macOS picker you control, with a red bar visible while recording. You can also add existing files and logs, watch an app's CPU and memory, mark the moment it happens, and take notes.
- **Review.** Every email, key, token, card number and home-directory account name found is listed with its context. Choose Hide or Keep for each. Black out parts of screenshots (the pixels are destroyed, not covered). Confirm each image and recording before export.
- **Share.** The app saves one file, reopens it to verify integrity and re-scan, and offers a web-page summary. It can also prefill a GitHub or GitLab issue that you attach the file to yourself.

It also opens reports others send you: an evidence timeline you can brush to filter, a windowed event list with an inspector, the evidence gallery, system details, and privacy and integrity status.

## Evidence

Median of three runs on an Apple M1 Pro with Go 1.27.4. [Method and raw output](docs/BENCHMARKS.md)

| Operation | Result |
|---|---|
| Record a log line with redaction on (SDK) | 0.714 µs, 4 allocations |
| Discard a log line below the level threshold | 7.1 ns, no allocations |
| Open and verify a 10,000-event fixture with a 2 MiB attachment | 19.0 ms (113.38 MB/s) |
| Write a report from 2,000 buffered events | 2.84 ms |

`make test` runs the Go race detector for the core and desktop packages, the end-to-end CLI suite and the frontend unit tests. The reader, event parser, redaction and OTLP import each passed a 90-second fuzzing run. [Fuzzing results](docs/BENCHMARKS.md#fuzzing)

## Documentation

- [Format specification](spec/SPEC.md), with [examples](spec/examples) including deliberately invalid ones
- [Architecture](docs/ARCHITECTURE.md) and [decision records](docs/adr)
- [SDK guide](docs/sdk.md) · [Extending ReproPack](docs/extending.md) (providers, policies, adapters, SDKs in other languages)
- [Desktop design and accessibility](docs/desktop.md)
- [Security and threat model](SECURITY.md)
- [Versioning and compatibility](docs/VERSIONING.md) · [Changelog](CHANGELOG.md) · [Roadmap](docs/ROADMAP.md)
- [Release preparation](docs/RELEASING.md) · [Adoption and integrations](docs/ADOPTION.md)
- [Contributing](CONTRIBUTING.md)

## Status

`0.1.0`, pre-release. The format is a 1.0 draft, stable for implementation. Nothing has been published to a registry or package manager yet. See [current limitations](docs/ROADMAP.md#current-limitations).

## License

Code: [Apache-2.0](LICENSE). The specification in `spec/` is also available under CC BY 4.0 ([NOTICE](NOTICE)).
