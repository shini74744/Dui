#!/usr/bin/env bash
set -euo pipefail
META=$(cd "$(dirname "$0")" && pwd)
SOURCE=$(realpath "${1:?source directory}")
OUT=$(realpath -m "${2:?output directory}")
ASSET=${3:?asset name}
test ! -e "$OUT"
mkdir -p "$OUT/package"
BUILD=$(jq -r .build "$META/manifest.json")
VERSION=$(jq -r .release_version "$META/manifest.json")
BINARY=xray
if [ "$GOOS" = windows ]; then VERSION=$(jq -r .release_version "$META/manifest.json")
BINARY=xray.exe; fi
cd "$SOURCE"
# Match upstream Android linker compatibility for github.com/wlynxg/anet.
EXTRA_LDFLAGS=""
if [ "$GOOS" = android ]; then EXTRA_LDFLAGS="-checklinkname=0"; fi
go build -trimpath -buildvcs=false -ldflags "$EXTRA_LDFLAGS -s -w -X github.com/xtls/xray-core/core.build=$BUILD -X github.com/xtls/xray-core/core.releaseVersion=$VERSION" -o "$OUT/package/$BINARY" ./main
cp LICENSE "$OUT/package/LICENSE"
cp "$META/README.md" "$OUT/package/README.md"
jq --arg target "$ASSET" --arg compiler "$(go version)" --arg commit "${GITHUB_SHA:-local}" '. + {target:$target,compiler:$compiler,dui_commit:$commit}' "$META/manifest.json" > "$OUT/package/BUILD.json"
file "$OUT/package/$BINARY"
if [ "$GOOS/$GOARCH" = linux/amd64 ]; then
    "$OUT/package/$BINARY" version | tee "$OUT/version.txt"
    grep -F "$BUILD" "$OUT/version.txt"
    grep -F "Xray $VERSION " "$OUT/version.txt"
    printf '{"log":{"loglevel":"warning"},"outbounds":[{"protocol":"freedom"}]}\n' > "$OUT/smoke.json"
    "$OUT/package/$BINARY" run -test -config "$OUT/smoke.json"
fi
if [ "$GOOS" = windows ]; then
    DLL_ARCH=$GOARCH
    if [ "$GOARCH" = 386 ]; then DLL_ARCH=x86; fi
    curl -fL --retry 3 https://www.wintun.net/builds/wintun-0.14.1.zip -o "$OUT/wintun.zip"
    unzip -q "$OUT/wintun.zip" -d "$OUT"
    cp "$OUT/wintun/bin/$DLL_ARCH/wintun.dll" "$OUT/package/"
    cp "$OUT/wintun/LICENSE.txt" "$OUT/package/LICENSE-wintun.txt"
fi
cd "$OUT/package"
zip -q -9 "$OUT/Xray-$ASSET.zip" ./*
cd "$OUT"
sha256sum "Xray-$ASSET.zip" > "Xray-$ASSET.zip.sha256"
