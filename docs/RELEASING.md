# Release preparation

Build the pre-release locally with Go, Bash, `zip`, `unzip` and `shasum`. The source remains the canonical local install until a published release exists.

```bash
make check
make test
scripts/package-cli.sh v0.1.0 dist/cli
(cd dist/cli && shasum -a 256 -c SHA256SUMS)
```

Each CLI archive contains its executable, `LICENSE` and `NOTICE`. Run the extracted binary on its native platform and validate `spec/examples/checkout-crash.repropack`. Cross-compiling a binary verifies the build on the target, and running it verifies the installation.

On macOS, from a terminal with Wails and `go-licenses` on `PATH`:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
go install github.com/google/go-licenses/v2@v2.0.1
(cd desktop && wails build -platform darwin/universal -trimpath)
scripts/package-desktop-macos.sh dist/desktop
(cd dist/desktop && shasum -a 256 -c SHA256SUMS)
```

The macOS archive contains the app, project notices, Go dependency licenses and both bundled font licenses. The current signing is ad hoc. A public release needs a Developer ID signature and notarisation and should be tested on a second Mac. On an approved tag push, GitHub Actions builds archives as workflow artifacts. Publishing them as a release is a separate approval.

## Release notes draft for 0.1.0

ReproPack bundles reproduction steps, logs, screenshots and diagnostics in one local file. The CLI validates the format, renders an offline HTML summary and converts OpenTelemetry JSON logs. The Go recorder keeps a bounded buffer alongside `slog`. The macOS app guides people through capture, privacy review and export.

The report format is a 1.0 draft. Desktop capture is macOS-only, text redaction uses recognisable patterns, and screenshots and recordings need human review. See the [changelog](../CHANGELOG.md) and [current limitations](ROADMAP.md#current-limitations).
