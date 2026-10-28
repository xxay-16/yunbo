#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)" == Darwin ]] || { echo "Build on macOS with Xcode command line tools." >&2; exit 1; }
export CGO_ENABLED=1
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
mkdir -p build/bin dist
for arch in amd64 arm64; do
  clang_arch="$arch"
  [[ "$arch" != amd64 ]] || clang_arch=x86_64
  GOOS=darwin GOARCH="$arch" CC="clang -arch $clang_arch" CXX="clang++ -arch $clang_arch" \
    go build -trimpath -ldflags="-s -w" -o "build/bin/yunbo-fyne-$arch" .
done
bundle="build/bin/Yunbo Fyne.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
lipo -create build/bin/yunbo-fyne-amd64 build/bin/yunbo-fyne-arm64 -output "$bundle/Contents/MacOS/yunbo-fyne"
lipo "$bundle/Contents/MacOS/yunbo-fyne" -verify_arch x86_64 arm64
cp build/fyne/Info.plist "$bundle/Contents/Info.plist"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
iconset="$stage/yunbo.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" build/appicon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
  double=$((size*2))
  sips -z "$double" "$double" build/appicon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$bundle/Contents/Resources/iconfile.icns"
plutil -lint "$bundle/Contents/Info.plist"
codesign --force --deep --sign - "$bundle"
codesign --verify --deep --strict "$bundle"
mkdir -p "$stage/image"
ditto "$bundle" "$stage/image/云泊 Fyne.app"
ln -s /Applications "$stage/image/Applications"
hdiutil create -volname "云泊 Fyne 预览版" -srcfolder "$stage/image" -ov -format UDZO dist/yunbo-fyne-macos-universal.dmg
hdiutil verify dist/yunbo-fyne-macos-universal.dmg
