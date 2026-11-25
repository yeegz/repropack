# Architecture

```
                    ┌──────────────────────── core module: github.com/yeegz/repropack (stdlib only) ────────────────────────┐
                    │                                                                                                    │
 your Go service ──▶│  sdk ──────────┐            redact ◀──── policies (JSON)                                           │
  (slog, errors,    │  Recorder      │  events    ▲                                                                      │
   panics, HTTP)    │  ring buffer   ├──────────▶ format.Writer ──▶ .repropack  ──▶ format.Reader (hardened) ─┬──▶ viewer │
                    │  providers     │            (streaming ZIP)                 (limits, paths, hashes)     │   (HTML) │
                    │                │                                                                         ├──▶ otlp   │
 plain log files ──▶│  ingest ───────┘                                                                         └──▶ CLI    │
                    └────────────────────────────────────────────────────────────────────────────────────────────────────┘
                    ┌──────────────── desktop module: github.com/yeegz/repropack/desktop (Wails v2 + Svelte 5) ───────────┐
 a person ─────────▶│  Svelte UI ◀── bindings ──▶ App (Go) ── SQLite drafts ── screencapture / ps ── core module          │
                    │             ◀── /media/ ───  mediaHandler (allowlisted types, verified reports only)              │
                    └────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Packages

| Package | Responsibility | Depends on |
|---|---|---|
| `format` | Types, streaming `Writer`, hardened `Reader`, validation, JSON Schemas, IDs | stdlib |
| `redact` | Rule engine (regex + key rules + prefilters), report rewrite with verification, review helpers | `format` |
| `sdk` | `Recorder` (bounded ring buffer), `slog` tee, error/crash capture, providers, report writing | `format`, `redact` |
| `ingest` | Parse existing logs (JSON lines, logfmt, timestamped text) into events | `format` |
| `otlp` | OpenTelemetry OTLP-JSON logs import/export (no OTel dependency) | `format` |
| `viewer` | `Summarize` (display model), self-contained HTML with strict CSP | `format`, `redact` |
| `cmd/repropack` | CLI | all of the above |
| `desktop` | App: capture, drafts, review, export, viewer | core + Wails + SQLite |

Each package is importable on its own. A Rust or Python implementation needs only `spec/SPEC.md` and the schemas.

## Data flow

**SDK path.** Each `Record` call does four things in order: (1) a level check, which is a 10 ns, allocation-free exit; (2) **redaction at record time**, so secrets never sit in memory; (3) filters, which only ever see redacted values; (4) an insert into a ring buffer bounded by count *and* approximate bytes, evicting the oldest first and counting what it dropped. `WriteReport` collects providers concurrently under a timeout (a provider that fails or panics becomes a warning event, never a failed report), snapshots the buffer, and streams the archive. `WriteFile` is atomic: it writes a temp file, sets 0600, then renames.

**Desktop path.** Each draft lives in SQLite (WAL) under `~/Library/Application Support/ReproPack`, so a crash never loses a capture in progress. Its evidence files sit in a 0700 directory. Review scans every text field with the same function export uses, so the finding keys match exactly. Export writes to a temp file, reopens it with the hardened reader, verifies the hashes, re-scans, and renames only if the remaining matches equal the ones the person chose to keep.

**Reader path.** The reader works in three passes:
1. *Structure*, before reading any content: names, duplicates, entry types, methods, ratios, total size, and the integrity table cross-check.
2. *Manifest*: size-limited, then shape-validated.
3. *Content*, lazily: every stream is wrapped to enforce the exact size and check SHA-256 at EOF.

## Concurrency

- `sdk.Recorder` uses a single mutex around the ring. Redaction runs *outside* the lock, so contention is limited to a short copy. `BenchmarkLogParallel` shows throughput holding up across 8 cores. `-race` tests cover 16 writers taking reports while recording.
- The desktop backend serialises SQLite access (`MaxOpenConns(1)`). The process sampler and the recording waiter are goroutines with explicit cancellation, and shutdown stops both.

## Why these choices

See the decision records:

- [ADR 1: ZIP container](adr/0001-zip-container.md)
- [ADR 2: JSON and JSON Lines, not Protobuf](adr/0002-json-encoding.md)
- [ADR 3: stdlib-only core](adr/0003-stdlib-core.md)
- [ADR 4: redaction model](adr/0004-redaction-model.md)
- [ADR 5: desktop stack and capture](adr/0005-desktop-stack.md)
