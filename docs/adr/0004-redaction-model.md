# ADR 4: redaction model

**Status:** accepted, 25 Nov 2026

**Context.** Shared diagnostics leak secrets (HAR files with session tokens; replay tools capturing passwords). Pattern redaction is never perfect, and pixels can't be pattern-matched.

**Decisions.**
1. **Redact early.** The SDK redacts at record time, so secrets never sit in the in-memory buffer.
2. **Record what happened, never the values.** `redaction.rules[]` holds counts per rule, plus `manual` box counts and `reviewed_attachments`.
3. **Verify after writing.** Producers reopen the written archive and re-scan it. `verified: true` means zero matches remain for that policy. If a person chose to keep a value, the report says `verified: false`. It is honest, not falsely reassuring.
4. **People review pixels.** Export is blocked until every image and recording is confirmed. Black-out boxes are burned into the pixels, the image is re-encoded as a clean PNG, and the original file is deleted.
5. **Rules are data.** JSON policies can extend the defaults (`{"extends":"default","disable":[...],"rules":[...]}`). Each rule has fast literal *hints*, so most lines skip the regex entirely. That brought SDK record cost from about 5.8 µs to 0.7 µs.
6. **Idempotent replacements.** Replacement text never re-matches any rule. A fuzzer enforces this, and it found and fixed one case: a `home-path` value captured `[REDACTED`.

**Known limits.** Pattern rules miss secrets with no recognisable shape, and they flag some false positives, such as version-like dotted quads that look like public IPs. Reviewing everything is the point of the Review step.
