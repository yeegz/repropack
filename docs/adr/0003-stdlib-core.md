# ADR 3: the core module depends only on the Go standard library

**Status:** accepted, 25 Nov 2026

**Context.** SDKs end up inside other people's binaries. Each dependency adds supply-chain risk, version conflicts and audit burden. OpenTelemetry interop is valuable, but the OTel Go SDK is a large dependency tree.

**Decision.**
- `format`, `redact`, `sdk`, `otlp`, `ingest`, `viewer` and the CLI import only the standard library.
- OTLP JSON is implemented directly for the logs subset.
- The desktop app is a **separate Go module** (`desktop/go.mod`, with a `replace` to the core during development).

**Consequences.**
- `go get github.com/yeegz/repropack/sdk` adds nothing else to `go.sum`.
- OTLP coverage is limited to logs. Traces and metrics export are on the roadmap.
