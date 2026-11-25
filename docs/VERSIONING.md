# Versioning and migration

Two independent version lines:

1. **Format** (`spec_version`, currently `1.0`). Minor versions are additive. Readers must accept any `1.y` and ignore unknown fields. A major version bump means readers refuse files they don't understand, with a clear message. See [spec §9](../spec/SPEC.md#9-versioning-and-compatibility).
2. **Software** (Semantic Versioning). The Go module, CLI and desktop app are versioned together while under `0.x`; breaking Go API changes may happen in minor releases and are listed in the changelog. From `1.0`:
   - the `format`, `redact` and `sdk` APIs follow Go module compatibility rules;
   - CLI flags and exit codes are stable;
   - JSON output (`--json`) only gains fields.

## Compatibility promises

| Change | Allowed in |
|---|---|
| New optional manifest or event field, new kind, role or level | format minor |
| New redaction rule in the default policy | software minor (it changes what gets redacted, so it's announced) |
| Tighter default reader limits | software minor, only if every existing example still validates |
| Field removal, meaning change, new required field | format major |

## Migrating reports

When a format major version ships, the CLI will gain `repropack migrate <in> -o <out>` for the previous major version. The old reader stays in the codebase for at least one major version.

Desktop drafts are stored in SQLite with `CREATE TABLE IF NOT EXISTS`. Schema changes ship as idempotent `ALTER TABLE` steps at startup, and exported reports never depend on draft storage.
