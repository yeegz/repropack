# ADR 1: ZIP as the container

**Status:** accepted, 25 Nov 2026

**Context.** A report bundles small structured files with large binaries (videos). It must open with standard tools on every OS, and viewers should be able to show the summary without reading a 200 MB recording.

**Options.**
- **tar.gz.** Ubiquitous, but sequential, and gzip prevents seeking.
- **A directory.** Not one shareable file.
- **SQLite.** Random access and one file, but not inspectable without tools, and a mutable format is risky as an interchange medium.
- **ZIP.** A central directory allows random access. Every desktop OS can open it natively. Per-entry compression lets media be stored as-is.

**Decision.** ZIP, with a fixed layout (`manifest.json`, `events.jsonl`, `attachments/*`), a strict name grammar, and every non-manifest entry listed with its SHA-256.

**Consequences.**
- ZIP's known attacks (zip-slip, bombs, duplicate names, symlinks) are handled by the reader by design: allowlisted layout, ratio and size limits, regular files only, and exact-size reads.
- Writers stream: events first, then attachments, then the manifest last, once hashes are known. Readers use the central directory, so order doesn't matter.
