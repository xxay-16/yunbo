#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != Darwin ]]; then
  echo "Run this script on macOS with Xcode command line tools." >&2
  exit 1
fi
version="$(/usr/bin/plutil -extract info.productVersion raw -o - wails.json)"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid productVersion in wails.json." >&2
  exit 1
fi
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
wails build -skipembedcreate -trimpath -platform darwin/universal -o yunbo

app="build/bin/yunbo.app"
test -d "$app"
plutil -lint "$app/Contents/Info.plist"
lipo -verify_arch x86_64 arm64 "$app/Contents/MacOS/yunbo"
# Apple Silicon requires an ad-hoc signature. Developer ID notarization needs
# a publisher-owned Apple certificate and is not claimed by this build.
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict --verbose=2 "$app"

mkdir -p dist
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
ditto "$app" "$stage/云泊.app"
ln -s /Applications "$stage/Applications"
image="dist/yunbo-$version-macos-universal.dmg"
test ! -e "$image" || rm "$image"
hdiutil create -volname "云泊 Yunbo $version" -srcfolder "$stage" \
  -ov -format UDZO "$image"
hdiutil verify "$image"
ls -lh "$image"
