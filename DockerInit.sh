#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/core/xray/default.env"
case $1 in
    amd64)
        ARCH="64"
        FNAME="amd64"
        HELPER_PLATFORM="amd64"
        ;;
    386 | i386)
        ARCH="32"
        FNAME="386"
        HELPER_PLATFORM="386"
        ;;
    armv8 | arm64 | aarch64)
        ARCH="arm64-v8a"
        FNAME="arm64"
        HELPER_PLATFORM="arm64"
        ;;
    armv7 | arm | arm32)
        ARCH="arm32-v7a"
        FNAME="arm"
        HELPER_PLATFORM="armv7"
        ;;
    armv6)
        ARCH="arm32-v6"
        FNAME="arm"
        HELPER_PLATFORM="armv6"
        ;;
    armv5)
        ARCH="arm32-v5"
        FNAME="arm"
        HELPER_PLATFORM="armv5"
        ;;
    s390x)
        ARCH="s390x"
        FNAME="s390x"
        HELPER_PLATFORM="s390x"
        ;;
    *)
        echo "Unsupported Docker architecture: $1" >&2
        exit 1
        ;;
esac
mkdir -p build/bin
cd build/bin
XRAY_ZIP="Xray-linux-${ARCH}.zip"
XRAY_BASE="https://github.com/$XRAY_REPO/releases/download/$XRAY_TAG"
wget -q "$XRAY_BASE/$XRAY_ZIP"
wget -q "$XRAY_BASE/$XRAY_ZIP.sha256"
sha256sum -c "$XRAY_ZIP.sha256"
unzip "Xray-linux-${ARCH}.zip"
rm -f "$XRAY_ZIP" "$XRAY_ZIP.sha256" geoip.dat geosite.dat
mv xray "xray-linux-${FNAME}"
python3 "$SCRIPT_DIR/core/helpers/assemble.py" --platform "$HELPER_PLATFORM" --output "$SCRIPT_DIR/build/bin"
wget -q https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
wget -q https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
wget -q -O geoip_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
wget -q -O geosite_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
wget -q -O geoip_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat
wget -q -O geosite_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat
cd ../../
