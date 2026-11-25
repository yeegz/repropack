# Hand-off: ReproPack 0.1.0 (25 Nov 2026)

## State

- Local git repo `~/Documents/repropack`, branch `main`. **Not pushed, nothing published** (no GitHub repo, no module tag, no app release).
- Core module (stdlib only): `format`, `redact`, `sdk`, `otlp`, `ingest`, `viewer`, `cmd/repropack`.
- Desktop app: `desktop/` (separate Go module, Wails v2.17.1 + Svelte 5). It builds to `desktop/build/bin/ReproPack.app` and has been verified running on macOS 27.
- The spec is a 1.0 draft (`spec/SPEC.md`) with JSON Schemas in `format/schema/` and valid and invalid examples in `spec/examples/`.
- The local showcase is `docs/showcase/index.html`. Its desktop screenshots show the real Svelte renderer with the bundled checkout report through the development bridge. Its phone screenshots show the standalone HTML report. It needs no server or network.

## Verify

```bash
make test        # go -race (core + desktop) and vitest
make check       # gofmt, vet, svelte-check
scripts/bench.sh # benchmarks → docs/bench-raw.txt
scripts/fuzz.sh 90s
```

## Done

Format, reader hardening, redaction with verification, SDK, CLI (11 commands), OTLP in/out, log ingest, HTML summary, desktop capture/review/share/viewer, docs, CI workflows, licences (Apache-2.0, spec also CC BY 4.0).

- An isolated Go module with an empty module cache and network module lookup disabled installed the CLI and ran all three Go examples against the checkout. The HTTP example returned successful, bad-input and recovered-panic responses. The CLI validated the sample and generated reports, showed a synthetic email in a scan, rewrote and re-scanned it, extracted the report, rendered HTML and round-tripped OTLP JSON.
- Go race suites and the four frontend unit tests passed. The Svelte checker reports zero warnings after the draft and viewer state updates. Chromium exercised main-content skip focus, report tab navigation, and Describe → Capture → Review → Share with the development bridge in light and dark.
- `docs/BENCHMARKS.md` gives measured medians and links to the captured results. All four fuzz targets completed 90 seconds. `docs/ADOPTION.md` has integration proposals, distribution priorities and unpublished launch copy.
- `scripts/package-cli.sh` produced five checked CLI archives with the project’s license and notice. The extracted Apple Silicon CLI validated the bundled report. The universal macOS app built and contains both Apple Silicon and Intel slices. See [release preparation](RELEASING.md) for the local package paths and signature limitations.

## Left (see docs/ROADMAP.md)

1. Windows and Linux capture backends (`desktop/capture.go`).
2. Developer ID signing and notarisation.
3. Report signatures over `content_digest`.
4. Recording trim; video ↔ tape sync.
5. TypeScript SDK, built from `docs/extending.md` §SDKs.
6. Push to GitHub, following the owner's rule: private `yeegz/repropack` plus a public showcase repo. **Ask first.**
7. Validate the Intel app on Intel hardware, and run the CLI on a Windows and Linux machine. The public showcase and portfolio hand-off are pending publication approval.

## Gotchas

- **Wails and CSP.** Don't put a CSP `<meta>` in `desktop/frontend/index.html`: Wails injects inline bootstrap scripts and the app goes blank. The policy is served as a header by `securityHeaders` in `desktop/main.go`.
- **Background typing.** When driving the app with background computer-use, typing into the WKWebView only delivers the first character. Foreground typing works; this is not an app bug.
- **Preview mode.** `npm run dev` with no Wails loads `src/dev/mock.ts`. It's excluded from production builds; check with `grep -l "Export to PDF hangs" desktop/frontend/dist/assets/*`, which should return nothing.
- **Regenerating example reports.** `go run ./internal/genexamples` writes them deterministically.
