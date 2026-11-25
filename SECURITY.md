# Security policy and threat model

## Reporting a vulnerability

Please report privately. Use GitHub's "Report a vulnerability" (private advisory) on this repository, or email the maintainer listed on the GitHub profile. Don't open a public issue. You'll get an acknowledgement within 5 working days. Fixes are released with credit unless you ask otherwise. Supported versions: the latest `0.x` release.

## What ReproPack protects

1. **The reporter's privacy.** Nothing is captured without an explicit action and nothing is uploaded, ever. Sensitive values are removed and the removal is verified.
2. **The recipient's machine.** Reports are untrusted input from strangers.
3. **Report integrity.** Recipients can tell when a report was altered after it was saved.

## Threat model

| Threat | Where | Mitigation | Tested by |
|---|---|---|---|
| Path traversal (zip-slip), absolute paths, backslashes, `:`/NUL/control chars | Reader, `extract` | Strict name grammar; only `manifest.json`, `events.jsonl`, `attachments/<one segment>` allowed; `extract` writes through `os.Root` and refuses non-empty targets | `TestMaliciousArchives`, `spec/examples/invalid/zip-slip.repropack` |
| Decompression bombs | Reader | Per-entry ratio limit (200:1 above 1 MiB); total, per-entry, manifest and line limits; exact-size reads that never trust declared sizes | `TestDecompressionBomb`, `TestLimitsTotal`, fuzzing |
| Smuggled files (unlisted entries, duplicates, symlinks, encrypted or exotic compression) | Reader | Every entry must be listed; duplicates, non-regular files, encryption and methods other than store/deflate are rejected | `TestMaliciousArchives` |
| Tampering | Reader, viewers | SHA-256 and size per file, `content_digest`; viewers show a prominent banner and **withhold media** from unverified reports | `tampered-*` cases, `TestTamperedReportHidesMedia`, `TestMediaHandler` |
| Script injection via report text (XSS) | HTML viewer, desktop | `html/template` contextual escaping; strict CSP (`default-src 'none'`, script by SHA-256 hash, `data:` images only); Svelte text binding; no `{@html}` anywhere | `TestHTMLEscapesUntrustedContent`, `TestCSPHashMatchesScript` |
| Active content as attachments (SVG, HTML, PDF), type confusion | Viewers | Allowlist of raster images and video only; magic-byte sniffing; `X-Content-Type-Options: nosniff`; `sandbox` CSP on media responses | `TestHTMLEscapesUntrustedContent` |
| Terminal escape injection | CLI | Control and C1 characters stripped from untrusted text before printing | `TestTerminalEscapesStripped` |
| Malformed JSON or deep nesting | Reader | Bounded manifest and line sizes, error cause chain ≤ 32, schema checks | `FuzzReader`, `FuzzEventLine` |
| Resource exhaustion in the SDK | SDK | Ring buffer bounded by count and bytes; oversize events dropped and counted; provider timeout (5 s default); providers that panic are contained | `TestRingBuffer*`, `TestProvidersTimeoutPanicAndError` |
| Secrets in logs, notes, paths, attachments | SDK, CLI, desktop | Default policy (17 rules: keys, tokens, JWTs, bearer, URL credentials, query secrets, emails, Luhn-checked cards, home-dir account names, public IPs, private keys, secret-named attributes); redaction at record time; post-write verification | `TestDefaultRules`, `TestRedactReportVerifies`, `TestExportRedactsAndVerifies`, `FuzzRedactString` |
| Secrets in screenshots and recordings | Desktop | Export blocked until every image and recording is confirmed; black-out boxes are burned in, the PNG is re-encoded and the original deleted | `TestReviewRequiresImageCheck`, `TestSaveMarkupReplacesOriginal` |
| Covert capture | Desktop | Capture only through OS pickers the person operates; visible recording bar plus the OS indicator; recording capped at 5 minutes; no keystroke, clipboard or accessibility capture; app watching reads CPU and RSS only | Design review (ADR 5) |
| Unauthorised upload | Desktop, SDK | No network code paths. Issue links open the browser with a prefilled, already-redacted summary; the person attaches the file. `OpenURL` accepts only github.com and gitlab.com | `TestIssueLink` |
| Leftover sensitive drafts | Desktop | Drafts live in a 0700 directory with 0600 files; retention policy (default 30 days, configurable); orphan cleanup | `TestRetentionCleanup` |
| Report files readable by other users | SDK, CLI, desktop | Reports written atomically with mode 0600 | `TestWriteFileAtomicAndPrivate`, `TestExportRedactsAndVerifies` |

## Known limitations

- Redaction is pattern-based. A secret with no recognisable shape will not be caught. People must review.
- `verified` covers the named policy only.
- There are no signatures yet. The digest proves internal consistency, not who made the report (see the roadmap).
- The desktop CSP allows inline scripts because Wails injects them. Report content is never rendered as HTML.
