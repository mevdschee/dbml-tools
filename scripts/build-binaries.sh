#!/usr/bin/env bash
#
# Cross-compile dbml-tools for every platform the VSCode extension supports.
#
# Output layout:
#   dist/<vscode-target>/dbml-tools[.exe]
#   dist/SHA256SUMS                       (collated checksums for all binaries)
#
# <vscode-target> values match VSCode's `vsce package --target` naming, which
# is also the layout the extension expects under server-bin/:
#   linux-x64, linux-arm64
#   darwin-x64, darwin-arm64
#   win32-x64, win32-arm64
#
# Version metadata embedded into each binary:
#   main.version    = contents of ./VERSION
#   main.commit     = `git rev-parse --short HEAD`  (+ "-dirty" if working tree dirty)
#   main.buildDate  = ISO-8601 UTC at build time
#
# Verify after build:
#   ./dist/<target>/dbml-tools version --sha256
#   sha256sum -c dist/SHA256SUMS
#
# Usage:
#   ./scripts/build-binaries.sh                 # build all targets
#   ./scripts/build-binaries.sh linux-x64       # build one target
#   ./scripts/build-binaries.sh --archives      # also produce .tar.gz / .zip
#
# All dependencies of dbml-tools are pure-Go (modernc.org/sqlite, lib/pq,
# go-sql-driver/mysql) so CGO is disabled and no C toolchain is needed.

set -euo pipefail

cd "$(dirname "$0")/.."

DIST=dist
BIN=dbml-tools

# Map of "vscode-target → GOOS/GOARCH". Order is deterministic.
ALL_TARGETS=(
  "linux-x64        linux   amd64"
  "linux-arm64      linux   arm64"
  "darwin-x64       darwin  amd64"
  "darwin-arm64     darwin  arm64"
  "win32-x64        windows amd64"
  "win32-arm64      windows arm64"
)

# Parse args.
SELECTED=()
WITH_ARCHIVES=0
for a in "$@"; do
  case "$a" in
    --archives) WITH_ARCHIVES=1 ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) SELECTED+=("$a") ;;
  esac
done

# ---------------------------------------------------------------------------
# Version metadata (compiled into every binary via -ldflags)
# ---------------------------------------------------------------------------

if [ ! -f VERSION ]; then
  echo "VERSION file not found at $(pwd)/VERSION" >&2
  exit 1
fi
VERSION=$(tr -d '[:space:]' < VERSION)
if [ -z "$VERSION" ]; then
  echo "VERSION file is empty" >&2
  exit 1
fi

if git rev-parse --short HEAD >/dev/null 2>&1; then
  COMMIT=$(git rev-parse --short HEAD)
  if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
    COMMIT="${COMMIT}-dirty"
  fi
else
  COMMIT="none"
fi
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS="-s -w"
LDFLAGS="$LDFLAGS -X main.version=$VERSION"
LDFLAGS="$LDFLAGS -X main.commit=$COMMIT"
LDFLAGS="$LDFLAGS -X main.buildDate=$BUILD_DATE"

# ---------------------------------------------------------------------------
# Build helpers
# ---------------------------------------------------------------------------

build_one() {
  local target=$1 goos=$2 goarch=$3
  local out="$DIST/$target/$BIN"
  if [ "$goos" = "windows" ]; then
    out="${out}.exe"
  fi
  mkdir -p "$(dirname "$out")"
  echo "→ $target  ($goos/$goarch)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$out" .
}

archive_one() {
  local target=$1 goos=$2
  local dir="$DIST/$target"
  local stem="$BIN-$VERSION-$target"
  if [ "$goos" = "windows" ]; then
    (cd "$dir" && zip -q "../$stem.zip" "$BIN.exe")
    echo "  archived $stem.zip"
  else
    tar -C "$dir" -czf "$DIST/$stem.tar.gz" "$BIN"
    echo "  archived $stem.tar.gz"
  fi
}

# ---------------------------------------------------------------------------
# Select targets
# ---------------------------------------------------------------------------

TO_BUILD=()
if [ ${#SELECTED[@]} -eq 0 ]; then
  TO_BUILD=("${ALL_TARGETS[@]}")
else
  for sel in "${SELECTED[@]}"; do
    match=""
    for row in "${ALL_TARGETS[@]}"; do
      # shellcheck disable=SC2086
      read -r t _ _ <<< $row
      if [ "$t" = "$sel" ]; then
        match=$row
        break
      fi
    done
    if [ -z "$match" ]; then
      echo "unknown target: $sel" >&2
      echo "known: $(echo "${ALL_TARGETS[@]}" | tr ' ' '\n' | awk 'NR%3==1' | tr '\n' ' ')" >&2
      exit 1
    fi
    TO_BUILD+=("$match")
  done
fi

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

mkdir -p "$DIST"
for row in "${TO_BUILD[@]}"; do
  # shellcheck disable=SC2086
  read -r target goos goarch <<< $row
  build_one "$target" "$goos" "$goarch"
  if [ "$WITH_ARCHIVES" = "1" ]; then
    archive_one "$target" "$goos"
  fi
done

# ---------------------------------------------------------------------------
# SHA256SUMS — single file listing the hash of every produced binary
# (and any archives), in the format `sha256sum -c` understands.
# ---------------------------------------------------------------------------

(
  cd "$DIST"
  # find binaries
  files=()
  for row in "${TO_BUILD[@]}"; do
    # shellcheck disable=SC2086
    read -r target goos _ <<< $row
    if [ "$goos" = "windows" ]; then
      files+=("$target/$BIN.exe")
    else
      files+=("$target/$BIN")
    fi
  done
  # add archives if present
  if [ "$WITH_ARCHIVES" = "1" ]; then
    for row in "${TO_BUILD[@]}"; do
      # shellcheck disable=SC2086
      read -r target goos _ <<< $row
      stem="$BIN-$VERSION-$target"
      if [ "$goos" = "windows" ]; then
        [ -f "$stem.zip" ] && files+=("$stem.zip")
      else
        [ -f "$stem.tar.gz" ] && files+=("$stem.tar.gz")
      fi
    done
  fi
  # produce a deterministic SHA256SUMS file.
  sha256sum "${files[@]}" | sort > SHA256SUMS
)

echo
echo "version:    $VERSION ($COMMIT, $BUILD_DATE)"
echo "binaries:   $DIST/"
echo "checksums:  $DIST/SHA256SUMS"
