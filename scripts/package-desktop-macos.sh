#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

app="$PWD/desktop/build/bin/ReproPack.app"
out=${1:-dist}
codesign --verify --deep --strict "$app"
mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

cp LICENSE NOTICE "$stage/"
(cd desktop && GOFLAGS='-tags=desktop,production' go-licenses save . \
  --ignore github.com/yeegz/repropack/desktop --save_path "$stage/licenses")
cp desktop/frontend/node_modules/@fontsource-variable/schibsted-grotesk/LICENSE "$stage/licenses/Schibsted-Grotesk.txt"
cp desktop/frontend/node_modules/@fontsource/ibm-plex-mono/LICENSE "$stage/licenses/IBM-Plex-Mono.txt"
ditto --norsrc "$app" "$stage/ReproPack.app"
ditto --norsrc -c -k "$stage" "$out/ReproPack-macos.zip"
(cd "$out" && unzip -tq ReproPack-macos.zip && shasum -a 256 ReproPack-macos.zip > SHA256SUMS)
