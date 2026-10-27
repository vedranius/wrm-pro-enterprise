#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
#  Web Remote Manager PRO v14.0.0 — build all platform binaries
#  Run this from the repo root OR from inside remote-manager/
#  Usage:  bash remote-manager/build-all.sh
#          bash build-all.sh          (if you're already in remote-manager/)
# ─────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
SRC_DIR="$SCRIPT_DIR"           # remote-manager/ — where main.go lives
DIST_DIR="$REPO_ROOT/dist"      # binaries land here

# ── Colour helpers ────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✓${NC} $*"; }
warn() { echo -e "${YELLOW}⚠${NC} $*"; }
die()  { echo -e "${RED}✗ ERROR:${NC} $*" >&2; exit 1; }

echo ""
echo "  Web Remote Manager PRO v14.0.0 — multiplatform build"
echo "  =================================================="
echo ""

# ── 1. Locate / install Go ────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  warn "Go not found — trying to install via apt (Debian/Ubuntu/Codespaces)..."
  sudo apt-get update -qq
  sudo apt-get install -y -qq golang-go || die "Could not install Go. Install it manually: https://go.dev/dl/"
fi

GO_VERSION=$(go version | awk '{print $3}')
ok "Using $GO_VERSION"

# ── 2. Prepare dist/ ─────────────────────────────────────────────────────────
mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/wrm-pro-v*   # wipe old builds

# ── 3. Version tag ───────────────────────────────────────────────────────────
GIT_SHORT=""
if command -v git &>/dev/null; then
  _sha=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || true)
  [[ -n "$_sha" ]] && GIT_SHORT="-$_sha"
fi
VERSION="v14.0.0${GIT_SHORT}"
ok "Build version: $VERSION"
echo ""

# ── 4. Target matrix ─────────────────────────────────────────────────────────
# Format: "GOOS/GOARCH[/GOARM]/output-suffix"
TARGETS=(
  "linux/amd64//wrm-pro-v14.0.0-linux-amd64"
  "linux/arm64//wrm-pro-v14.0.0-linux-arm64"
  "linux/arm/7/wrm-pro-v14.0.0-linux-armv7"
  "linux/arm/6/wrm-pro-v14.0.0-linux-armv6"
  "linux/386//wrm-pro-v14.0.0-linux-386"
  "darwin/amd64//wrm-pro-v14.0.0-darwin-amd64"
  "darwin/arm64//wrm-pro-v14.0.0-darwin-arm64"
  "windows/amd64//wrm-pro-v14.0.0-windows-amd64.exe"
  "windows/arm64//wrm-pro-v14.0.0-windows-arm64.exe"
  "android/arm64//wrm-pro-v14.0.0-android-arm64"
  "freebsd/amd64//wrm-pro-v14.0.0-freebsd-amd64"
  "freebsd/arm64//wrm-pro-v14.0.0-freebsd-arm64"
  "openbsd/amd64//wrm-pro-v14.0.0-openbsd-amd64"
)

BUILT=()
FAILED=()

for target in "${TARGETS[@]}"; do
  IFS='/' read -r GOOS GOARCH GOARM OUTNAME <<< "$target"
  OUTPUT="$DIST_DIR/$OUTNAME"

  printf "  Building %-38s ... " "$OUTNAME"

  BUILD_ENV=(
    "CGO_ENABLED=0"
    "GOOS=$GOOS"
    "GOARCH=$GOARCH"
  )
  [[ -n "$GOARM" ]] && BUILD_ENV+=("GOARM=$GOARM")

  if (cd "$SRC_DIR" && env "${BUILD_ENV[@]}" go build \
       -ldflags="-s -w -X main.AppVersion=$VERSION" -trimpath \
       -o "$OUTPUT" . 2>/tmp/wrm-build-err); then
    SIZE=$(du -sh "$OUTPUT" 2>/dev/null | cut -f1)
    echo -e "${GREEN}OK${NC} (${SIZE})"
    BUILT+=("$OUTNAME")
  else
    echo -e "${RED}FAILED${NC}"
    cat /tmp/wrm-build-err | sed 's/^/    /' >&2
    FAILED+=("$OUTNAME")
  fi
done

# ── 5. Summary ───────────────────────────────────────────────────────────────
echo ""
echo "  ── Results ──────────────────────────────────────"
echo "  Built:  ${#BUILT[@]} / $((${#BUILT[@]} + ${#FAILED[@]}))"
echo "  Output: $DIST_DIR/"
echo ""

ls -lh "$DIST_DIR"/wrm-pro-v* 2>/dev/null | awk '{print "  "$NF"  ("$5")"}' || true

if [[ ${#FAILED[@]} -gt 0 ]]; then
  echo ""
  warn "Failed targets:"
  for f in "${FAILED[@]}"; do echo "    - $f"; done
  echo ""
  die "Some builds failed."
fi

echo ""
ok "All ${#BUILT[@]} binaries ready in: $DIST_DIR/"
echo ""
echo "  Quick start:"
echo "    chmod +x $DIST_DIR/wrm-pro-v14.0.0-linux-amd64"
echo "    PORT=8080 $DIST_DIR/wrm-pro-v14.0.0-linux-amd64"
echo ""
