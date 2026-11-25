# Extending ReproPack

Each piece is designed to be replaced or extended on its own.

## Providers (Go SDK)

A provider contributes context, events and attachments when a report is written:

```go
type Provider interface {
    Name() string
    Collect(ctx context.Context) (sdk.ProviderData, error)
}
```

Rules:
- Respect `ctx`. Collection shares one timeout.
- Return only what the person would expect in a bug report. `EnvProvider` takes an explicit allowlist, never the whole environment.
- Context keys are namespaced automatically (`<provider>.<key>`), and values are redacted.

Example: a database pool provider.

```go
sdk.ProviderFunc{ID: "db", Fn: func(ctx context.Context) (sdk.ProviderData, error) {
    st := pool.Stats()
    return sdk.ProviderData{Context: map[string]any{"open": st.OpenConnections, "wait_count": st.WaitCount}}, nil
}}
```

## Redaction policies

Rules are plain JSON (see the [SDK guide](sdk.md#4-redaction)). A rule needs `id` and either `pattern` (Go RE2; a named group `v` limits replacement to that part) or `keys` (attribute-name substrings). Add `hints`: literals one of which must appear before the regex runs. Hints are what keep record-time redaction cheap.

Every new rule should come with a positive test, a negative test, and a check that the replacement never re-matches. `FuzzRedactString` enforces that last point.

## Format extensions

Put producer-specific data in `manifest.extensions` under `x-<vendor>.<name>`. Readers ignore what they don't know and must preserve it when rewriting. Proposals to promote an extension into the spec are welcome as issues with a draft spec diff.

## Adapters

Adapters convert between ReproPack and other systems. Keep them outside the core package if they need dependencies. Existing adapters:

| Adapter | Package | Direction |
|---|---|---|
| OpenTelemetry OTLP JSON logs | `otlp` | both |
| Plain logs (JSON lines, logfmt, timestamped text) | `ingest` | in |
| GitHub and GitLab issue links | `desktop` (`IssueLink`) | out, via the browser, no upload |

Planned or welcome: Sentry envelope export, HAR import with redaction, Jira, Linear.

An adapter that sends data to a service must:
- run only on an explicit user action;
- show what will be sent;
- never run from a report's content.

## SDKs in other languages

The format is the contract. An SDK in TypeScript, Python, Rust, .NET or Dart is conformant when:

1. **It writes valid reports.** Output passes `repropack validate`. Event lines have a strictly increasing `seq`. Hashes and `content_digest` follow spec §7.
2. **It is bounded.** It keeps a ring buffer bounded by count and bytes, counts dropped events, and reports them in `events.dropped`.
3. **It redacts before storage.** It implements the default rule IDs with equivalent semantics (`redact/rules.go` is the reference; test vectors are in `redact/redact_test.go`). It records counts in `redaction.rules` and sets `verified: false` unless it re-scans.
4. **It never transmits.** Writing a report produces bytes or a file. Sending it is the host application's decision.
5. **It contains provider failures.** A slow or failing provider becomes an event, not an error.

Suggested API shape, mirroring Go:

```
Recorder(options) -> recorder
recorder.record(event) / log / breadcrumb / metric / error / span
recorder.set_context(key, value)
recorder.write_report(writer, report_options) -> manifest
logger_bridge(recorder, next_handler)   # the language's standard logging hook
```

Check your SDK by generating reports in its test suite and running `repropack validate --json`. Then run the `spec/examples` conformance files through its reader, if it has one.
