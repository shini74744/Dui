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
mv "$DEST/Xray-core-26.9.9" "$DEST/source"
patch --batch --fuzz=0 -d "$DEST/source" -p1 < "$META/dui.patch"
patch --batch --fuzz=0 -d "$DEST/source" -p1 < "$META/closewait.patch"
REALITY_URL=$(jq -r .reality_source_url "$META/manifest.json")
REALITY_HASH=$(jq -r .reality_source_sha256 "$META/manifest.json")
REALITY_PREFIX=$(jq -r .reality_source_prefix "$META/manifest.json")
curl -fL --retry 3 "$REALITY_URL" -o "$DEST/reality.zip"
printf '%s  %s\n' "$REALITY_HASH" "$DEST/reality.zip" | sha256sum -c -
unzip -q "$DEST/reality.zip" -d "$DEST/reality-module"
mv "$DEST/reality-module/$REALITY_PREFIX" "$DEST/source/dui-reality"
patch --batch --fuzz=0 -d "$DEST/source/dui-reality" -p1 < "$META/reality.patch"
cp -R "$META" "$DEST/source/dui-build"
