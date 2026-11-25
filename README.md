# ReproPack

**One file that explains a bug.** ReproPack captures what someone did, what they saw, and what the software was doing at the time, hides what's private, and saves it all as a single `.repropack` file anyone can open. No account, no server, no upload.

It is four pieces you can adopt separately:

| Piece | What it's for | Path |
|---|---|---|
| **Report format** | An open, versioned, ZIP + JSON format with integrity hashes and a redaction record | [`spec/SPEC.md`](spec/SPEC.md), [`format/`](format) |
| **Go SDK** | Keep a bounded, redacted buffer of diagnostics beside your existing logging; write a report on demand or on panic | [`sdk/`](sdk) |
| **CLI** | Validate, inspect, redact, convert (OpenTelemetry) and summarise reports | [`cmd/repropack`](cmd/repropack) |
| **Desktop app** | Guided capture for non-technical people, plus a viewer for the reports you receive (macOS today) | [`desktop/`](desktop) |

The core module (`format`, `redact`, `sdk`, `otlp`, `ingest`, `viewer`, CLI) uses **only the Go standard library**. The desktop app is a separate Go module so SDK users never pull in UI dependencies.

## Why

Bug reports usually arrive as a sentence, a cropped screenshot, or a link into someone's SaaS account. Crash SDKs need a server; self-hosting Sentry needs 16 GB of RAM and about 16 services. Shared diagnostic files leak secrets: Okta's 2023 breach went through customer-uploaded HAR files containing session tokens. And there's no standard, offline bundle a person can *inspect before sending*. ([Research notes](docs/research/PRIOR-ART.md).)

ReproPack is that bundle:

- **Local-first.** Reports are files. Share them however you like, or don't.
- **Redaction you can check.** Every report records which rules removed how many values (never the values), whether the result was re-scanned, and which images a person reviewed.
- **Untrusted by default.** Readers enforce path rules, size and compression limits, and per-file SHA-256. Viewers never execute report content.
- **Readable without us.** `unzip` plus a text editor works. So does any language with ZIP and JSON.
- **Interoperable.** Events map to OpenTelemetry log records, in and out.

## Quick start

### CLI

```bash
go install github.com/yeegz/repropack/cmd/repropack@latest   # once published; for now: go build -o bin/repropack ./cmd/repropack
```

```bash
repropack create -o crash.repropack --title "Checkout freezes" --log app.log --attach shot.png --step "Press Pay"
```

```bash
repropack inspect crash.repropack
```

```bash
repropack html crash.repropack          # self-contained, offline web page
```

```bash
repropack scan crash.repropack && repropack redact crash.repropack -o clean.repropack
```

```bash
repropack validate clean.repropack      # exit 0 valid, 1 invalid, 2 usage
```

### SDK (alongside your existing logger)

```go
rec := sdk.New(sdk.Options{
    App:       format.AppContext{Name: "billing", Version: "1.4.2"},
    MaxEvents: 2000,                 // bounded ring buffer; oldest dropped first
    Providers: []sdk.Provider{sdk.RuntimeProvider()},
})
logger := slog.New(rec.Handler(slog.NewJSONHandler(os.Stderr, nil))) // your output is unchanged
defer rec.CapturePanic("crash.repropack")                            // writes a report, then re-panics

// When a user clicks "Report a problem":
rec.WriteFile("report.repropack", sdk.ReportOptions{Title: "Invoice total is wrong"})
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

All numbers come from `scripts/bench.sh` on an Apple M-series laptop (Go 1.27.4). Full output: [docs/BENCHMARKS.md](docs/BENCHMARKS.md).

| Operation | Result |
|---|---|
| Record a log line with redaction on (SDK) | ~0.7 µs, 4 allocs |
| Record a log line below the level threshold | ~10 ns, 0 allocs |
| Open and verify a 2 MB report with 10k events | ~19 ms (~114 MB/s) |
| Write a report from a full 2000-event buffer | ~6 ms |
| Reject a zip-slip archive | ~2 µs |

Tests cover all six Go packages plus the desktop backend under `-race`, an end-to-end CLI suite, 16 malicious-archive cases, and fuzzers for the reader, the event parser, redaction and OTLP import ([results](docs/BENCHMARKS.md#fuzzing)).

## Documentation

- [Format specification](spec/SPEC.md), with [examples](spec/examples) including deliberately invalid ones
- [Architecture](docs/ARCHITECTURE.md) and [decision records](docs/adr)
- [SDK guide](docs/sdk.md) · [Extending ReproPack](docs/extending.md) (providers, policies, adapters, SDKs in other languages)
- [Desktop design and accessibility](docs/desktop.md)
- [Security and threat model](SECURITY.md)
- [Versioning and compatibility](docs/VERSIONING.md) · [Changelog](CHANGELOG.md) · [Roadmap](docs/ROADMAP.md)
- [Contributing](CONTRIBUTING.md)

## Status

`0.1.0`, pre-release. The format is a 1.0 draft, stable for implementation. Nothing has been published to a registry or package manager yet. See [current limitations](docs/ROADMAP.md#current-limitations).

## License

Code: [Apache-2.0](LICENSE). The specification in `spec/` is also available under CC BY 4.0 ([NOTICE](NOTICE)).
