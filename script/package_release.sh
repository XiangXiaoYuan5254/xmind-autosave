#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-1.0.0}"
APP_NAME="XMindAutoSave"
DISPLAY_NAME="XMind 自动保存"
BUNDLE_ID="local.xmind.autosave"
MIN_SYSTEM_VERSION="13.0"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "版本号必须是 x.y.z 格式" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_DIR="$ROOT_DIR/dist/release"
APP_BUNDLE="$RELEASE_DIR/$APP_NAME.app"
APP_CONTENTS="$APP_BUNDLE/Contents"
APP_MACOS="$APP_CONTENTS/MacOS"
APP_RESOURCES="$APP_CONTENTS/Resources"
APP_BINARY="$APP_MACOS/$APP_NAME"
DMG_STAGING="$RELEASE_DIR/dmg-staging"
DMG_PATH="$RELEASE_DIR/$APP_NAME-$VERSION.dmg"
ZIP_PATH="$RELEASE_DIR/$APP_NAME-$VERSION.zip"
CHECKSUM_PATH="$RELEASE_DIR/SHA256SUMS.txt"

cd "$ROOT_DIR"
swift test
swift build -c release --arch arm64 --arch x86_64
BUILD_DIR="$(swift build -c release --arch arm64 --arch x86_64 --show-bin-path)"

rm -rf "$APP_BUNDLE" "$DMG_STAGING"
rm -f "$DMG_PATH" "$ZIP_PATH" "$CHECKSUM_PATH"
mkdir -p "$APP_MACOS" "$APP_RESOURCES" "$DMG_STAGING"

cp "$BUILD_DIR/$APP_NAME" "$APP_BINARY"
cp "$ROOT_DIR/Resources/config.json" "$APP_RESOURCES/config.json"
cp "$ROOT_DIR/Resources/AppIcon.icns" "$APP_RESOURCES/AppIcon.icns"
chmod +x "$APP_BINARY"

cat >"$APP_CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>$APP_NAME</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleName</key>
  <string>$DISPLAY_NAME</string>
  <key>CFBundleDisplayName</key>
  <string>$DISPLAY_NAME</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundleVersion</key>
  <string>$VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>$MIN_SYSTEM_VERSION</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSPrincipalClass</key>
  <string>NSApplication</string>
</dict>
</plist>
PLIST

plutil -lint "$APP_CONTENTS/Info.plist" >/dev/null
codesign --force --deep --options runtime --sign - "$APP_BUNDLE" >/dev/null
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"

cp -R "$APP_BUNDLE" "$DMG_STAGING/$APP_NAME.app"
ln -s /Applications "$DMG_STAGING/Applications"
cp "$ROOT_DIR/INSTALL.md" "$DMG_STAGING/安装说明.md"

hdiutil create \
  -volname "$DISPLAY_NAME" \
  -srcfolder "$DMG_STAGING" \
  -ov \
  -format UDZO \
  "$DMG_PATH" >/dev/null

ditto -c -k --sequesterRsrc --keepParent "$APP_BUNDLE" "$ZIP_PATH"
(
  cd "$RELEASE_DIR"
  shasum -a 256 "$(basename "$DMG_PATH")" "$(basename "$ZIP_PATH")" >"$(basename "$CHECKSUM_PATH")"
)

rm -rf "$DMG_STAGING"

echo "已生成："
echo "  $DMG_PATH"
echo "  $ZIP_PATH"
echo "  $CHECKSUM_PATH"
