# Changelog

All notable changes are documented here. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning: [docs/VERSIONING.md](docs/VERSIONING.md).

## [Unreleased] — 0.1.0

### Added
- **Format 1.0 draft:** a ZIP container with `manifest.json`, `events.jsonl` and `attachments/`; per-file SHA-256 and `content_digest`; a redaction record; namespaced extensions; JSON Schemas; valid and invalid examples.
- **`format`:** a streaming writer and a hardened reader (path grammar, entry allowlist, size, ratio and line limits, lazy hash-checked streams), plus fuzzers.
- **`redact`:** a 17-rule default policy with literal prefilters, JSON policies that extend the defaults, report rewriting with post-write verification, and a fuzzer guaranteeing replacements never re-match.
- **`sdk`:** a bounded recorder (count and bytes), record-time redaction, filters, a `slog` tee, error chains with stacks, panic capture, concurrent providers under a timeout, and atomic 0600 report files.
- **`otlp`:** OTLP JSON logs import and export.
- **`ingest`:** parsing for JSON-lines, logfmt and timestamped text logs, with stack continuation and tail reads.
- **`viewer`:** a summary model and a self-contained HTML page (strict CSP with a script hash, accessible, offline).
- **CLI:** `validate`, `inspect`, `html`, `scan`, `redact`, `create`, `extract`, `export --otlp`, `import --otlp`, `schema`, `version`.
- **Desktop app (macOS):** guided capture, the review and redaction flow, verified export, issue links, the report viewer and retention settings.
