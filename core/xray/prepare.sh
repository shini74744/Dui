#!/usr/bin/env bash
set -euo pipefail
META=$(cd "$(dirname "$0")" && pwd)
DEST=${1:?usage: prepare.sh destination}
test ! -e "$DEST"
mkdir -p "$DEST"
URL=$(jq -r .source_url "$META/manifest.json")
HASH=$(jq -r .source_sha256 "$META/manifest.json")
curl -fL --retry 3 "$URL" -o "$DEST/upstream.zip"
printf '%s  %s\n' "$HASH" "$DEST/upstream.zip" | sha256sum -c -
unzip -q "$DEST/upstream.zip" -d "$DEST"
mv "$DEST/Xray-core-26.3.27" "$DEST/source"
patch --batch --fuzz=0 -d "$DEST/source" -p1 < "$META/closewait.patch"
install -m 0644 "$META/pipe_close_test.go.txt" "$DEST/source/common/singbridge/pipe_close_test.go"
patch --batch --fuzz=0 -d "$DEST/source" -p1 < "$META/version.patch"
install -m 0644 "$META/dui_version_test.go.txt" "$DEST/source/core/dui_version_test.go"
gofmt -w "$DEST/source/core/core.go" "$DEST/source/core/dui_version_test.go"
cp -R "$META" "$DEST/source/dui-build"
