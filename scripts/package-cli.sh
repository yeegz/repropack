#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

version=${1:?usage: scripts/package-cli.sh VERSION [OUTPUT_DIR]}
if [[ ! $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Use a version such as v0.1.0 or v0.1.0-rc.1." >&2
  exit 2
fi

out=${2:-dist}
mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp LICENSE NOTICE "$stage/"
archives=()

for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  os=${target%/*}
  arch=${target#*/}
  binary=repropack
  if [[ $os == windows ]]; then
    binary=repropack.exe
  fi
  archive="repropack-${version}-${os}-${arch}.zip"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${version}" -o "$stage/$binary" ./cmd/repropack
  (cd "$stage" && zip -q "$archive" "$binary" LICENSE NOTICE)
  unzip -tq "$stage/$archive"
  archives+=("$archive")
done

(cd "$stage" && shasum -a 256 "${archives[@]}" > SHA256SUMS)
mv "${archives[@]/#/$stage/}" "$stage/SHA256SUMS" "$out/"
