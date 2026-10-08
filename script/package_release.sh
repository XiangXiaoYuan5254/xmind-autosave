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
# 发布版用 Developer ID 证书签名，再交给 Apple 公证；钥匙串里没有这张证书时退回临时签名、跳过公证，只适合自己用。
DEVELOPER_ID="Developer ID Application: Li Ming wang (46AL7LQ9T8)"
SIGN_IDENTITY="${XMIND_SIGN_IDENTITY:-}"
if [[ -z "$SIGN_IDENTITY" ]]; then
  if security find-identity -v -p codesigning | grep -qF "\"$DEVELOPER_ID\""; then
    SIGN_IDENTITY="$DEVELOPER_ID"
  else
    SIGN_IDENTITY="-"
  fi
fi
# 公证凭据：xcrun notarytool store-credentials helloxxy-notary（存在登录钥匙串里）
NOTARY_PROFILE="${XMIND_NOTARY_PROFILE:-helloxxy-notary}"

# notarize <要上传的文件> <贴票据的文件>：交给 Apple 公证（几分钟，期间别让 Mac 锁屏，否则读不到凭据）
notarize() {
  local result
  result="$(xcrun notarytool submit "$1" --keychain-profile "$NOTARY_PROFILE" --wait --output-format json)"
  if [[ "$(plutil -extract status raw -o - - <<<"$result")" != "Accepted" ]]; then
    echo "公证没有通过：$result" >&2
    echo "查看原因：xcrun notarytool log <id> --keychain-profile $NOTARY_PROFILE" >&2
    exit 1
  fi
  xcrun stapler staple -q "$2"
}

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
if [[ "$SIGN_IDENTITY" == "-" ]]; then
  echo "提示：钥匙串里没有 $DEVELOPER_ID，使用临时签名、不公证，下载的用户首次打开会被 Gatekeeper 拦下。" >&2
  codesign --force --options runtime --sign - "$APP_BUNDLE" >/dev/null
else
  codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$APP_BUNDLE" >/dev/null
fi
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"

if [[ "$SIGN_IDENTITY" != "-" ]]; then
  # 票据贴进 App 本身，DMG 和 zip 里的都带着它
  ditto -c -k --keepParent "$APP_BUNDLE" "$ZIP_PATH"
  notarize "$ZIP_PATH" "$APP_BUNDLE"
  rm -f "$ZIP_PATH"
  # 打包用的 Mac 可能关掉了 Gatekeeper，那样 spctl 什么都放行，所以只认来源是否为已公证的 Developer ID
  spctl -a -vv -t exec "$APP_BUNDLE" 2>&1 | grep -q "source=Notarized Developer ID" || {
    echo "$APP_BUNDLE 没有公证上" >&2
    exit 1
  }
fi

cp -R "$APP_BUNDLE" "$DMG_STAGING/$APP_NAME.app"
ln -s /Applications "$DMG_STAGING/Applications"
cp "$ROOT_DIR/INSTALL.md" "$DMG_STAGING/安装说明.md"

hdiutil create \
  -volname "$DISPLAY_NAME" \
  -srcfolder "$DMG_STAGING" \
  -ov \
  -format UDZO \
  "$DMG_PATH" >/dev/null
if [[ "$SIGN_IDENTITY" != "-" ]]; then
  codesign --force --timestamp --sign "$SIGN_IDENTITY" "$DMG_PATH"
  notarize "$DMG_PATH" "$DMG_PATH"
fi

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
