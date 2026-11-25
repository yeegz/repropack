# Prior art and differentiation

Researched 25 Nov 2026. Tags: **[F]** page fetched and read, **[S]** search snippet or secondary coverage, **[K]** background knowledge (canonical URL, not fetched; verify before citing).

## Summary

There is plenty of prior art on capture tools, crash envelopes and replay, and a long record of privacy failures with shared diagnostic files (HAR, session replay). **No standard exists for a portable, offline diagnostic bundle that a person can inspect before sending.** That is the gap ReproPack fills. It deliberately does not build a hosted platform.

## 1. End-user bug-capture tools

| Tool | Captures | Where data goes |
|---|---|---|
| Jam.dev | Console/network logs, device metadata, user actions, instant replay [F] | Hosted SaaS; Free 30 Jams/mo; Team $14/creator/mo; no self-hosting listed [F] https://jam.dev/pricing |
| Marker.io | Browser/OS/URL on all plans; console, network and replay only on Team ($149–199/mo) and up [F] | Pushes to Jira/GitHub/Linear https://marker.io/pricing |
| BugHerd | Visual feedback pinned to page elements [K] | Hosted https://bugherd.com |
| Bird Eats Bug → BrowserStack Bug Capture | Screen recording + console/network [K] | Acquired 27 Aug 2024 [S] |
| Loom | Video/audio only [K] | Hosted |
| Windows Steps Recorder (psr.exe) | Click-by-click screenshots in a ZIP/MHT [K] | Local; **deprecated Nov 2023** [F] https://learn.microsoft.com/en-us/windows/whats-new/deprecated-features |
| macOS Feedback Assistant / sysdiagnose | System, crash, hang logs [S] | Goes to Apple; Apple warns sysdiagnose may contain private information [S] https://developer.apple.com/forums/thread/739560 |
| GitHub issue forms | Structured YAML forms, `render: shell` log fields [F] | Free, but text only, no capture https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-issue-forms |

Every hosted tool stores the artifact on the vendor's server and shares a link. None defines a file the reporter owns. The one local precedent, psr.exe, is deprecated.

Closest "generate a bundle" analogues: GitHub Enterprise Server support bundles (strip tokens from logs) [S], Tailscale bug-report IDs with `--record` [S], cloud-init `collect-logs` tarballs [S]. Each project solves this ad hoc.

## 2. Crash and error SDKs

**Sentry envelope** [F] https://develop.sentry.dev/sdk/foundations/transport/envelopes/ — newline-delimited: one JSON header line, then items of `header line + payload` (payload length declared via `length`). Unknown item types are skipped and retained. Streams well and is easy to append to, but it is not random-access, not a self-describing bundle, and assumes a DSN and a server.

**Sentry self-hosting** [F] https://develop.sentry.dev/self-hosted/ — minimum 4 cores, 16 GB RAM + 16 GB swap, 20 GB disk, ~16 services (Kafka, ClickHouse, Postgres, Redis, Snuba, Relay, Symbolicator…). Complaints: getsentry/self-hosted#787 (realistic limits, users reporting 3.7–9.5 GB) and #2700 (RAM exhaustion, Kafka errors) [F].

**Crashpad/Breakpad** [F] https://chromium.googlesource.com/crashpad/crashpad/+/HEAD/doc/overview_design.md — out-of-process handler writes minidumps (binary, stacks + registers + memory) to a local database, uploads via multipart HTTP. The one widely used crash-file standard, but binary, native-only and memory-heavy, which is a privacy exposure.

**Crashlytics** [S] — Google-managed, BigQuery export, no self-host. **Bugsnag** [K] — hosted SaaS.

## 3. Session replay and privacy

- rrweb (MIT, ~20.5k stars) records DOM snapshots plus mutations; underpins Sentry Replay and others [F] https://github.com/rrweb-io/rrweb
- PostHog masks inputs by default, not general text [F] https://posthog.com/docs/session-replay/privacy
- Sentry Replay masks all text and blocks media by default; server-side scrubbing is best-effort [S]
- Princeton "No boundaries" (2017–18): replay scripts exfiltrated full page content and form inputs including passwords [F] https://webtransparency.cs.princeton.edu/no_boundaries/
- Masking regressions: getsentry/sentry-javascript#10258 (password visible in a replay) [S], rrweb-io/rrweb PR #1745 (hidden inputs not masked) [S]

Lesson: allow/deny masking keeps failing at the edges. ReproPack does not do replay. Evidence is explicit, user-chosen and reviewed.

## 4. OpenTelemetry

- OTLP JSON: hex trace/span IDs, integer enums, int64 as decimal strings, lowerCamelCase keys, unknown fields ignored [F] https://opentelemetry.io/docs/specs/otlp/
- File exporter: **Development** status; JSON Lines, one signal per file, no container, no attachments, no redaction record [F] https://opentelemetry.io/docs/specs/otel/protocol/file-exporter/
- No standard portable offline diagnostic bundle found (negative finding from limited searching).
- Go `log/slog` handlers (`Enabled`, `Handle`, `WithAttrs`, `WithGroup`) make a tee into a bounded buffer straightforward [F] https://pkg.go.dev/log/slog

ReproPack therefore **imports and exports OTLP JSON logs** rather than competing with OTel.

## 5. Portable formats

- **HAR** [K]: the standard "send me a trace" artifact. **Okta 2023**: customer-uploaded HAR files contained session tokens; an attacker with support-system access hijacked sessions; 134 customers affected [S] https://www.bleepingcomputer.com/news/security/okta-breach-134-customers-exposed-in-october-support-system-hack/ — the reason ReproPack makes redaction a recorded, verified step.
- **OCI image layout** [F]: manifest + content-addressed blobs; may be carried in tar or zip. ReproPack follows the manifest + digested-parts pattern.
- **ZIP vs tar**: ZIP's central directory allows random access (read the manifest without reading recordings); tar+gzip is sequential [F/K]. ZIP also opens natively on every desktop OS.
- **JSON vs Protobuf vs CBOR** [K]: JSON is inspectable in any text editor and every language; Protobuf needs schema tooling; CBOR is less hand-inspectable. ReproPack's events are small and the format must be readable without our tools → JSON + JSON Lines.
- **Zip-slip** [F] https://security.snyk.io/research/zip-slip-vulnerability and **zip bombs** [K]: Go's `archive/zip` does not enforce size limits; names must be validated. ReproPack's reader enforces both.

## 6. Gap

What a vendor-neutral, offline, inspectable, redaction-first bundle uniquely solves:

1. One file the reporter can inspect before sending — no account, DSN or upload.
2. A manifest-driven container holding logs, events, screenshots, recordings, context and notes, with per-file SHA-256.
3. Redaction as a first-class, recorded and re-verified step (what rule removed how much; never the value).
4. A Go SDK that tees `slog` into a bounded buffer and snapshots on demand, alongside existing logging.
5. Safe reading by default: strict paths, size and ratio caps, no execution.
6. Adapters outward (OTLP JSON now; Sentry envelope later) to avoid a new silo.

**Not built:** a hosted platform or dashboard; session/DOM replay; minidump-style memory capture; a new telemetry schema; a large standards effort up front.
