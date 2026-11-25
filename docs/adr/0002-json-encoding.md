# ADR 2: JSON and JSON Lines, not Protobuf or CBOR

**Status:** accepted, 25 Nov 2026

**Context.** Reports are read by people (often in a text editor), by tools in many languages, and occasionally at scale. Typical reports hold thousands to tens of thousands of events.

**Options.**
- **Protobuf.** Compact and typed, but needs schema tooling and can't be read without it.
- **CBOR.** Compact, schemaless, but not human-readable.
- **JSON with JSON Lines for events.** Universally readable and streamable. Its verbosity is mostly removed by ZIP deflate.

**Decision.** `manifest.json` is one JSON object. `events.jsonl` holds one event per line with a strictly increasing `seq`. Formal JSON Schemas (draft 2020-12) ship with the reference implementation.

**Evidence.** 10,000 events write in about 18 ms and open and verify at about 114 MB/s (see [BENCHMARKS.md](../BENCHMARKS.md)). That's well inside interactive budgets, so the cost of a binary encoding isn't justified.

**Consequences.**
- Readers must cap line length and event count, which they do.
- Numbers follow JSON semantics. The spec says how to map them to OTLP (int64 as decimal strings in OTLP).
