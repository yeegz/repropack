# Installation

Nothing is published yet, so build from source. Every command below runs offline after the first dependency download.

## Requirements

| Component | Needs |
|---|---|
| CLI, SDK | Go 1.24+ |
| Desktop app | Go 1.26+, Node.js 20+, npm, Wails CLI v2.17.1, macOS 12+ with Xcode Command Line Tools |

## CLI

```bash
git clone <repo> repropack && cd repropack
```

```bash
go build -o bin/repropack ./cmd/repropack
```

```bash
./bin/repropack version
```

Or install it to your `GOPATH/bin`:

```bash
go install ./cmd/repropack
```

## SDK

```bash
go get github.com/yeegz/repropack/sdk
```

Until the module is published, point at a local checkout:

```bash
go mod edit -replace github.com/yeegz/repropack=../repropack
```

## Desktop app (macOS)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
```

```bash
cd desktop && wails build -platform darwin/arm64
```

```bash
open build/bin/ReproPack.app
```

For Intel Macs use `-platform darwin/amd64`; `darwin/universal` builds both. The first screenshot or recording makes macOS ask for **Screen Recording** permission (System Settings → Privacy & Security → Screen Recording). Builds are ad-hoc signed. To distribute, sign and notarise with your own Developer ID (see the roadmap).

**Where data lives:**
- Drafts: `~/Library/Application Support/ReproPack`, removed after the retention period set in Settings (30 days by default).
- Exported reports: wherever you save them.

### Windows and Linux

The app compiles and runs (`wails build -platform windows/amd64` or `linux/amd64`, which need that platform's WebView prerequisites). Built-in capture is macOS-only for now. Elsewhere you can add existing screenshots, recordings and logs, and use the viewer and every other feature.

## Verify a build

```bash
go test -race ./...
```

```bash
(cd desktop && go test -race ./...)
```

```bash
(cd desktop/frontend && npm ci && npm run check && npm test)
```

```bash
./bin/repropack validate spec/examples/checkout-crash.repropack
```
