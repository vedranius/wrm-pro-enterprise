#!/usr/bin/env bash
# Usage: tools/build-appimage.sh VERSION {amd64|arm64} [OUTPUT_DIRECTORY]
# Build on x86_64 Linux. Go, curl and sha256sum are required; FUSE is not.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
VERSION=${1:?Provide the version}
GOARCH=${2:?Provide amd64 or arm64}
case "$GOARCH" in
  amd64) ARCH=x86_64; RUNTIME_SHA=2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d ;;
  arm64) ARCH=aarch64; RUNTIME_SHA=00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444 ;;
  *) echo 'Supported architectures: amd64, arm64' >&2; exit 1 ;;
esac
[[ $(uname -m) == x86_64 ]] || { echo 'Build host must be x86_64 Linux.' >&2; exit 1; }
DIST=${3:-"$ROOT/dist"}
mkdir -p "$DIST"
DIST=$(cd -- "$DIST" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
download() {
  curl --fail --location --retry 5 --retry-all-errors --connect-timeout 30 --max-time 300 "$1" -o "$2"
  printf '%s  %s\n' "$3" "$2" | sha256sum --check --status
}
# Stable upstream releases with pinned SHA-256, including the runtime for each target.
download https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage \
  "$WORK/appimagetool" ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0
download "https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-$ARCH" \
  "$WORK/runtime" "$RUNTIME_SHA"
chmod +x "$WORK/appimagetool"
APPDIR=$WORK/WRM.AppDir
mkdir -p "$APPDIR/usr/bin"
cp "$ROOT/packaging/appimage/AppRun" "$APPDIR/AppRun"
cp "$ROOT/packaging/appimage/wrm.desktop" "$APPDIR/wrm.desktop"
cp "$ROOT/remote-manager/static/brand/icon-512.png" "$APPDIR/wrm.png"
cp "$ROOT/LICENSE" "$APPDIR/LICENSE"
chmod +x "$APPDIR/AppRun"
(cd "$ROOT/remote-manager" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" \
  go build -trimpath -ldflags="-s -w -X main.AppVersion=$VERSION" -o "$APPDIR/usr/bin/wrm" .)
OUTPUT=$DIST/wrm-pro-$VERSION-linux-$GOARCH.AppImage
ARCH=$ARCH VERSION=$VERSION "$WORK/appimagetool" --appimage-extract-and-run \
  --no-appstream --runtime-file "$WORK/runtime" "$APPDIR" "$OUTPUT"
chmod +x "$OUTPUT"
if [[ $GOARCH == amd64 ]]; then
  XDG_DATA_HOME=$WORK/data "$OUTPUT" --appimage-extract-and-run -version | grep -F -- "$VERSION"
fi
echo "Built $OUTPUT"
