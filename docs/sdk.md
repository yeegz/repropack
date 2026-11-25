# Go SDK guide

The module is available from a source checkout. Follow the [installation steps](install.md#sdk) to add it to an existing Go module or run the standalone example.

The SDK keeps a **bounded, redacted buffer** of recent diagnostics in memory and writes a `.repropack` report when you ask: from a "Report a problem" button, from an error path, or from a panic. It never sends anything anywhere, and it doesn't replace your logger, tracer or crash reporter.

## 1. Create a recorder

```go
rec := sdk.New(sdk.Options{
    App:       format.AppContext{Name: "billing", Version: version, Environment: "production"},
    MaxEvents: 2000,          // default 2000
    MaxBytes:  4 << 20,       // default 4 MiB; the oldest events go first
    MinLevel:  format.LevelDebug,
    Providers: []sdk.Provider{sdk.RuntimeProvider()},
    Filters:   []sdk.Filter{func(e *format.Event) bool { return e.Attrs["route"] != "/healthz" }},
})
```

A recorder is safe for concurrent use. Create one per process.

## 2. Feed it without changing your logging

**slog.** Wrap your existing handler. Its output is untouched, and the recorder can keep a lower level than your sink:

```go
logger := slog.New(rec.Handler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
```

Groups and `With` attributes are flattened (`req.user.plan`). `error` values become structured error chains. `trace_id` and `span_id` attributes become correlation fields.

**Direct calls:**

```go
rec.Breadcrumb("opened invoice", map[string]any{"invoice": id})
rec.Log(format.LevelWarn, "retrying upload", map[string]any{"attempt": 2})
rec.Metric("queue.depth", 41, "{job}")
rec.Error(err, "save failed", nil)          // wrap chain + caller stack
rec.Span("POST /pay", start, time.Now(), "502")
rec.SetContext("feature.new_checkout", true)
```

**Other loggers.** zap, zerolog and logrus can write JSON lines to an `io.Writer`. Parse those with `ingest.Reader` and pass the result to `rec.Record`, or attach the log file itself (see below).

**OpenTelemetry.** Keep your OTel pipeline. To put spans and logs into a report, call `rec.Span` or `rec.Record` from a small span processor or log exporter, or convert an OTLP JSON file with `repropack import --otlp`.

## 3. Write reports

```go
m, err := rec.WriteFile("report.repropack", sdk.ReportOptions{
    Title:        "Invoice total is wrong",
    Reproduction: &format.Reproduction{Steps: []string{"Open INV-2041", "Apply discount"}},
    Attachments:  []sdk.Attachment{sdk.FileAttachment("/var/log/app.log", "text/plain", format.RoleLog)},
})
```

- `WriteFile` is atomic and creates the file with mode 0600. `WriteReport(ctx, w, ...)` streams to any `io.Writer`.
- Providers run concurrently under `Timeout` (default 5 s). Failures and panics become warning events, never failed reports.
- Text attachments are redacted with the same policy.

**Crashes:**

```go
func main() {
    defer rec.CapturePanic("crash.repropack") // writes the report, then re-panics
    ...
}
```

For goroutines and HTTP handlers, recover yourself and call `rec.WriteCrash(path, recovered, nil)`. See `examples/httpservice`.

## 4. Redaction

Redaction is on by default and runs **when an event is recorded**. Use a custom policy with:

```go
p, _ := redact.LoadPolicy("policy.json")
rec := sdk.New(sdk.Options{Policy: p})
```

```json
{
  "name": "billing",
  "extends": "default",
  "disable": ["ipv4"],
  "rules": [
    { "id": "customer-id", "description": "customer IDs", "pattern": "\\bcus_[A-Za-z0-9]{14}\\b", "hints": ["cus_"] },
    { "id": "iban-key", "description": "IBAN fields", "keys": ["iban"] }
  ]
}
```

`sdk.NoRedaction` turns it off explicitly, for example when your logs are already scrubbed. The manifest records the policy and per-rule counts. `verified` stays `false` until a verifier (`repropack redact`, the desktop app) re-scans the file.

## 5. Overhead

The median of three runs on an Apple M1 Pro with Go 1.27.4. [Benchmark method and raw results](BENCHMARKS.md)

| | ns/op | allocs |
|---|---|---|
| Log line, redaction on | 714.1 | 4 |
| Log line, redaction off | 129.3 | 1 |
| Log line below `MinLevel` | 7.126 | 0 |
| 4-attribute log line, redaction on | 2,018 | 15 |
| `slog` JSON handler without recorder | 517.7 | 0 |
| `slog` JSON handler with recorder | 2,237 | 13 |
| Write report, 2,000 buffered events | 2,836,982 | 20,082 |

Memory is bounded by `MaxBytes`, an approximate accounting of the buffered events, plus Go's per-object overhead.

**Guidance:**
- Record breadcrumbs and warnings and above in hot paths.
- Keep debug recording for paths that matter.
- Use filters to drop noisy sources before they occupy the buffer.

## 6. Reading reports from Go

```go
r, err := format.Open("report.repropack", format.Limits{}) // untrusted-safe defaults
defer r.Close()
if err := r.Verify(); err != nil { /* altered or damaged */ }
r.Events(func(e format.Event) error { fmt.Println(e.Seq, e.Message); return nil })
s, _ := viewer.Summarize(r, viewer.Options{})
```
