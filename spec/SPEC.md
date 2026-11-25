# ReproPack Report Format — Specification 1.0

Status: **Draft 1.0** (stable for implementation; changes follow the compatibility rules in §9).
Media type: `application/vnd.repropack+zip` · Extension: `.repropack`
Machine-readable schemas: [`format/schema/manifest.schema.json`](../format/schema/manifest.schema.json), [`format/schema/event.schema.json`](../format/schema/event.schema.json) (also printed by `repropack schema`).

The key words MUST, MUST NOT, SHOULD and MAY are used as described in RFC 2119.

## 1. Goals

A `.repropack` file is one portable bug report: what a person did and saw, what the software was doing, and the evidence (screenshots, recordings, logs), with integrity hashes and a record of what was redacted. It MUST be readable with standard tools (any ZIP utility and a text editor) and MUST NOT depend on any hosted service.

Non-goals: streaming telemetry, session replay, memory dumps, executable content.

## 2. Container

A report is a ZIP archive (APPNOTE 6.3, Zip64 permitted). Entries MUST use the *stored* (0) or *deflate* (8) method and MUST NOT be encrypted.

The archive MUST contain exactly these entries and no others:

| Path | Required | Content |
|---|---|---|
| `manifest.json` | yes | §3, UTF-8 JSON object |
| `events.jsonl` | yes (may be empty) | §4, UTF-8 JSON Lines |
| `attachments/<name>` | no | §5, opaque bytes |

### 2.1 Entry names

Every entry name MUST:
- be valid UTF-8, at most 255 bytes, use `/` as separator;
- not be absolute, not end in `/`, and not contain empty, `.` or `..` segments;
- not contain `\`, `:`, NUL or any control character (U+0000–U+001F, U+007F);
- be `manifest.json`, `events.jsonl`, or `attachments/` followed by exactly one segment.

Every entry MUST be a regular file (no directories, symlinks or devices) and MUST appear at most once. Every entry other than `manifest.json` MUST be listed in the manifest's integrity table (`events`, `attachments`). Readers MUST reject archives that violate this section.

Writers SHOULD store already-compressed media (PNG, JPEG, video) with method 0 and SHOULD set entry modification times to a constant so identical content produces identical archives.

## 3. Manifest (`manifest.json`)

A single JSON object. Field summary (see schema for full detail):

| Field | Type | Notes |
|---|---|---|
| `format` | string | MUST be `"repropack"` |
| `spec_version` | string | `"MAJOR.MINOR"`; this document is `1.0` |
| `report_id` | string | 1–64 chars; writers SHOULD use a UUIDv7 |
| `created_at` | RFC 3339 time | UTC recommended |
| `title` | string | 1–300 bytes, required |
| `description` | string | ≤ 64 KiB |
| `producer` | object | `name` (required), `version`, `sdk` |
| `app` | object | the software the report is about: `name`, `version`, `build`, `environment`, `runtime`, `pid`, `started_at` |
| `os` | object | `name` (GOOS-style: `darwin`, `linux`, `windows`), `version`, `build`, `arch`, `kernel`, `locale`, `timezone` |
| `device` | object | `model`, `cpu`, `cpu_cores`, `memory_bytes`, `displays` |
| `reproduction` | object | `steps[]`, `expected`, `actual`, `frequency` |
| `context` | object | ≤ 256 custom fields; values MUST be JSON scalars |
| `annotations` | array | user notes: `id`, `text`, optional `at`, `attachment_id`, `author` |
| `events` | object | integrity entry for `events.jsonl`: `path`, `sha256`, `size`, `count`, `first`, `last`, `dropped` |
| `attachments` | array | §5 |
| `redaction` | object | §6 |
| `content_digest` | string | §7 |
| `extensions` | object | §8 |

Readers MUST ignore unknown fields. Writers MUST NOT put secrets in `producer`, `os` or `device`.

## 4. Events (`events.jsonl`)

UTF-8 JSON Lines: one JSON object per line, each terminated by `\n`. Blank lines are not allowed. Lines SHOULD be ≤ 1 MiB; readers MAY reject longer lines.

| Field | Type | Notes |
|---|---|---|
| `seq` | integer ≥ 1 | MUST strictly increase line by line |
| `ts` | RFC 3339 time | when it happened |
| `kind` | string | `log`, `error`, `crash`, `metric`, `span`, `breadcrumb`, `marker`, `custom` |
| `level` | string | `trace`, `debug`, `info`, `warn`, `error`, `fatal` |
| `source` | string | logger, component or file name |
| `msg` | string | human-readable message |
| `attrs` | object | structured fields (any JSON) |
| `trace_id`, `span_id` | string | W3C/OpenTelemetry hex IDs for correlation |
| `error` | object | required for `error`/`crash`: `message`, `type`, `stack[]` (`function`, `file`, `line`, `module`), `cause` (recursive, depth ≤ 32), `raw_stack`, `handled` |
| `metric` | object | required for `metric`: `name`, `value`, `unit` (UCUM as in OpenTelemetry: `By`, `ms`, `%`, `{request}`) |
| `span` | object | required for `span`: `name`, `start`, `end`, `trace_id`, `span_id`, `parent_span_id`, `status` |

Order is by `seq`, which reflects recording order. Timestamps MAY be non-monotonic (clock changes, imported logs); viewers SHOULD sort by time for display when appropriate. Readers MUST display unknown kinds as `custom` and unknown levels as `info`.

`marker` events with `source: "you"` denote a moment the reporter flagged ("it happened here").

## 5. Attachments

Each attachment entry in the manifest:

| Field | Notes |
|---|---|
| `id` | 1–64 chars `[A-Za-z0-9_-]`, unique |
| `path` | `attachments/<name>`; writers SHOULD derive `<name>` from `id` plus an extension, never from user input |
| `media_type` | declared type, e.g. `image/png` |
| `sha256`, `size` | integrity of the uncompressed bytes |
| `role` | `screenshot`, `recording`, `log`, `dump`, `other` (unknown roles allowed) |
| `title`, `created_at`, `width`, `height`, `duration_ms` | descriptive |
| `annotated` | true when markup (including redaction boxes) is burned into the pixels |

Attachments are **data, never code**. Readers MUST NOT execute attachments or render them as active content. In particular, viewers MUST NOT render `text/html`, `image/svg+xml`, PDF or any scriptable type inline, and SHOULD verify magic bytes match the declared raster/video type before display.

## 6. Redaction record

```json
"redaction": {
  "policy": "default",
  "rules": [{ "id": "email", "description": "email addresses", "matches": 3 }],
  "manual": 1,
  "verified": true,
  "verified_at": "2026-11-25T09:41:08Z",
  "reviewed_attachments": ["shot-pay"]
}
```

- `rules` counts what each rule removed. It MUST NOT contain the removed values.
- `manual` counts areas a person blacked out on images.
- `verified: true` asserts that, after writing, the producer re-read the archive and found **no** remaining matches for `policy`. A producer MUST set `verified: false` when a person chose to keep matched values or when no re-scan happened.
- `reviewed_attachments` lists images/recordings a person confirmed contain nothing private. Pattern matching cannot inspect pixels; viewers SHOULD warn about media not listed here.

Redaction is best effort. A `verified` report has no matches *for that policy*; it is not a guarantee that nothing sensitive remains.

## 7. Integrity

Each file's `sha256` is lowercase hex of its uncompressed bytes, and `size` its uncompressed length. Readers MUST check both before trusting content and MUST treat a mismatch as an invalid report.

`content_digest` identifies the report's content independently of ZIP layout and JSON formatting:

```
rows = [(events.path, events.sha256, events.size)] + [(a.path, a.sha256, a.size) for a in attachments]
sort rows by path (byte order)
content_digest = "sha256:" + hex(SHA-256( concat(path + "\0" + sha256 + "\0" + decimal(size) + "\n" for each row) ))
```

Digital signatures are out of scope for 1.0 (see the roadmap); the digest is designed to be the payload a future signature covers.

## 8. Extensions

Producers MAY add `extensions` keys of the form `x-<vendor>.<name>` (lowercase), each holding any JSON value. Readers MUST ignore extensions they don't understand and MUST preserve them when rewriting a report (for example, when redacting).

## 9. Versioning and compatibility

- `spec_version` is `MAJOR.MINOR`.
- **Minor** versions only add optional fields, kinds, roles or levels. A 1.x reader MUST accept any 1.y report, ignoring what it doesn't know.
- **Major** versions may change meaning or structure. Readers MUST reject a major version they don't implement, with a message naming the version.
- Writers MUST write the lowest minor version whose features they use.
- Removing a field requires a major version. Deprecations are announced one minor version in advance in `CHANGELOG.md`.

## 10. Resource limits (reader requirements)

Readers MUST bound resources for untrusted input. The reference reader's defaults, which conforming writers SHOULD stay within:

| Limit | Default |
|---|---|
| archive size | 1 GiB |
| entries | 4096 |
| manifest | 4 MiB |
| `events.jsonl` (uncompressed) | 256 MiB |
| event line | 1 MiB |
| events | 2,000,000 |
| one attachment (uncompressed) | 512 MiB |
| all entries (uncompressed) | 2 GiB |
| compression ratio (entries > 1 MiB) | 200:1 |

Readers MUST enforce sizes while decompressing (never trusting declared sizes) and MUST NOT extract entries to disk except through the name rules of §2.1.

## 11. Conformance

- **Writer** conformance: output passes `repropack validate`.
- **Reader** conformance: accepts `spec/examples/*.repropack` and rejects every file in `spec/examples/invalid/` with an error.

## 12. Example

`spec/examples/checkout-crash.repropack` (generated by `go run ./internal/genexamples`):

```
$ unzip -l checkout-crash.repropack
  events.jsonl
  attachments/shot-pay.png
  attachments/applog.txt
  manifest.json
```

```json
{"seq":12,"ts":"2026-11-25T09:41:07.01Z","kind":"error","level":"error","source":"checkout","msg":"Payment failed",
 "error":{"type":"*payments.GatewayError","message":"confirm payment: gateway returned 502",
  "stack":[{"function":"(*Client).Confirm","file":"payments/client.go","line":142,"module":"example.com/shop/payments"}],
  "cause":{"type":"*url.Error","message":"Post \"https://gateway.example/confirm\": EOF"}}}
```

## 13. Interoperability

- **OpenTelemetry:** events map to OTLP log records (`repropack export --otlp`, `repropack import --otlp`): `level` ↔ `severityNumber` (trace 1, debug 5, info 9, warn 13, error 17, fatal 21), `attrs` ↔ attributes, `error` ↔ `exception.*` attributes, `trace_id`/`span_id` unchanged.
- **Plain logs:** `repropack create --log` parses JSON-lines (slog/zap/zerolog), logfmt and `<timestamp> <LEVEL> message` lines.
