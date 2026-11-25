# Installation

Build from your local ReproPack checkout. The CLI and SDK need only the Go toolchain. The desktop app also needs the dependencies listed below. This pre-release has no package registry or public download.

## Requirements

| Component | Needs |
|---|---|
| CLI, SDK | Go 1.24+ |
| Desktop app | Go 1.26+, Node.js 20+, npm, Wails CLI v2.17.1, macOS 12+ with Xcode Command Line Tools |

## CLI

Run from the checkout root.

```bash
go build -o bin/repropack ./cmd/repropack
./bin/repropack version
./bin/repropack validate spec/examples/checkout-crash.repropack
```

`go install ./cmd/repropack` installs into `GOBIN` or `GOPATH/bin`.

## SDK

From your Go module, point to your local checkout. The placeholder version is resolved from that directory.

```bash
go mod edit -replace github.com/yeegz/repropack=/path/to/repropack
go get github.com/yeegz/repropack/sdk@v0.0.0
```

To try the SDK in a new module, set `REPROPACK_SOURCE` to the absolute path of the source checkout, then run the following commands in an empty directory.

```bash
go mod init example.com/repropack-app
go mod edit -replace "github.com/yeegz/repropack=$REPROPACK_SOURCE"
go get github.com/yeegz/repropack/sdk@v0.0.0
cp "$REPROPACK_SOURCE/examples/basic/main.go" .
go run .
go run github.com/yeegz/repropack/cmd/repropack validate basic.repropack
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

The CLI targets Windows and Linux as well as macOS. The desktop release packages are currently macOS only. The desktop capture backend uses the macOS system picker. Windows and Linux desktop builds need platform-specific WebView prerequisites and runtime verification.

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
