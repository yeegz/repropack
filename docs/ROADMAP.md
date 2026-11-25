# Roadmap

## Current limitations

- **Capture is macOS-only.** Windows and Linux builds can add existing files and view reports, but have no built-in screenshots or recording yet.
- **Unsigned app.** The app is ad-hoc signed and not notarised, so macOS Gatekeeper will warn on other machines.
- **Pattern redaction.** Secrets with no recognisable shape are missed, and some version strings are flagged as public IPs. People must review. There is no OCR, so text inside images isn't scanned automatically.
- **No signatures.** `content_digest` proves internal consistency, not who made the report.
- **OTLP coverage is logs only.** Spans travel inside events but aren't exported as OTLP traces.
- **The viewer loads up to 50,000 events into the UI** (the reader supports 2 million). Larger reports point to `repropack inspect --events`.
- **The HTML summary renders up to 5,000 rows and embeds up to 48 MB of media.**
- **The SDK lives in Go only.** Other languages are specified ([extending.md](extending.md)) but not built.
- **The "watch an app" sampler polls `ps` once a second.** Lightweight, but coarse.
- **Recording length is capped at 5 minutes**, and recordings can't be trimmed in the app.
- **App-level process crash capture** (catching another app's crash) isn't attempted. macOS crash reports can be added as files.

## Next (0.2)

- Windows capture (Graphics Capture API, picker-based) and Linux capture (xdg-desktop-portal ScreenCast).
- Signed and notarised macOS builds; CI release artifacts (built, not auto-published).
- Report signing (minisign or Sigstore) over `content_digest`, with signer identity shown in the viewer.
- Recording trim and frame scrubbing on the tape; jump from a tape pin to the video timestamp.
- OTLP traces export; Sentry envelope export adapter.
- `repropack diff a.repropack b.repropack` (compare two reproductions).

## Later

- TypeScript SDK (browser and Node), then Python, Rust and .NET, all validated against the conformance examples.
- Optional local OCR for screenshots, suggesting areas to hide (on-device only).
- A shared, versioned rule corpus for redaction across SDKs.
- A WebAssembly reader so the HTML viewer can open `.repropack` files by drag and drop, fully offline.
- Accessibility audit with VoiceOver and NVDA users; localisation.
