#!/usr/bin/env bash
set -euo pipefail

# 把 dist/release 中的 DMG（以及同版本的 Windows 安装程序，如果有）同步到
# 官网 website/downloads/，并更新页面上的版本号、下载链接、文件大小、发布日期
# 和“最近更新”。
#
# 用法：./script/sync_website_release.sh [版本号]
# 省略版本号时，使用 dist/release 中版本最高的 DMG。

APP_NAME="XMindAutoSave"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_DIR="$ROOT_DIR/dist/release"
SITE_DIR="$ROOT_DIR/website"
DOWNLOAD_DIR="$SITE_DIR/downloads"
INDEX_HTML="$SITE_DIR/index.html"
CHANGELOG="$ROOT_DIR/CHANGELOG.md"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  shopt -s nullglob
  candidates=("$RELEASE_DIR/$APP_NAME"-*.dmg)
  shopt -u nullglob
  if (( ${#candidates[@]} == 0 )); then
    echo "dist/release 中没有 $APP_NAME-*.dmg，请先运行 ./script/package_release.sh <版本号>" >&2
    exit 1
  fi
  VERSION="$(
    for path in "${candidates[@]}"; do
      name="$(basename "$path" .dmg)"
      echo "${name#"$APP_NAME"-}"
    done | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1
  )"
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "版本号必须是 x.y.z 格式" >&2
  exit 2
fi

DMG_NAME="$APP_NAME-$VERSION.dmg"
DMG_SOURCE="$RELEASE_DIR/$DMG_NAME"
if [[ ! -f "$DMG_SOURCE" ]]; then
  echo "找不到 $DMG_SOURCE，请先运行 ./script/package_release.sh $VERSION" >&2
  exit 1
fi

EXE_NAME="$APP_NAME-Setup-$VERSION.exe"
EXE_SOURCE="$RELEASE_DIR/$EXE_NAME"
if [[ ! -f "$EXE_SOURCE" ]]; then
  echo "提示：没有 $EXE_NAME（./script/package_windows.sh $VERSION 可以生成），官网上的 Windows 版保持不变。" >&2
  EXE_SOURCE=""
fi

# 与 Finder 一致，按 1000 进位显示大小
human_size() {
  awk -v bytes="$1" 'BEGIN {
    if (bytes >= 1000000) printf "%.1f MB", bytes / 1000000
    else printf "%d KB", (bytes + 500) / 1000
  }'
}

mkdir -p "$DOWNLOAD_DIR"
find "$DOWNLOAD_DIR" -maxdepth 1 -type f \( -name "$APP_NAME-*.dmg" -o -name "$APP_NAME-*.zip" \) -delete
cp "$DMG_SOURCE" "$DOWNLOAD_DIR/"
if [[ -n "$EXE_SOURCE" ]]; then
  find "$DOWNLOAD_DIR" -maxdepth 1 -type f -name "$APP_NAME-Setup-*.exe" -delete
  cp "$EXE_SOURCE" "$DOWNLOAD_DIR/"
fi
(
  cd "$DOWNLOAD_DIR"
  shopt -s nullglob
  shasum -a 256 "$DMG_NAME" "$APP_NAME"-Setup-*.exe >SHA256SUMS.txt
)

DMG_SIZE="$(human_size "$(stat -f %z "$DMG_SOURCE")")"
EXE_SIZE=""
if [[ -n "$EXE_SOURCE" ]]; then
  EXE_SIZE="$(human_size "$(stat -f %z "$EXE_SOURCE")")"
fi
RELEASE_DATE="$(stat -f %Sm -t %Y-%m-%d "$DMG_SOURCE")"

# 从 CHANGELOG.md 中取出该版本的条目
RELEASE_NOTES=""
if [[ -f "$CHANGELOG" ]]; then
  RELEASE_NOTES="$(awk -v heading="## $VERSION" '
    $0 == heading { on = 1; next }
    on && /^## / { exit }
    on && /^- / {
      line = substr($0, 3)
      gsub(/&/, "\\&amp;", line)
      gsub(/</, "\\&lt;", line)
      gsub(/>/, "\\&gt;", line)
      printf "              <li>%s</li>\n", line
    }
  ' "$CHANGELOG")"
fi
if [[ -z "$RELEASE_NOTES" ]]; then
  echo "提示：CHANGELOG.md 中没有 “## $VERSION” 的条目，页面上的“最近更新”保持不变。" >&2
fi

VERSION="$VERSION" DMG_SIZE="$DMG_SIZE" EXE_SIZE="$EXE_SIZE" RELEASE_DATE="$RELEASE_DATE" RELEASE_NOTES="$RELEASE_NOTES" \
perl -0pi -e '
  s/\Q'"$APP_NAME"'\E-\d+\.\d+\.\d+\.dmg/'"$APP_NAME"'-$ENV{VERSION}.dmg/g;
  if (length $ENV{EXE_SIZE}) {
    s/\Q'"$APP_NAME"'\E-Setup-\d+\.\d+\.\d+\.exe/'"$APP_NAME"'-Setup-$ENV{VERSION}.exe/g;
    s/(data-release-exe-size>)[^<]*/$1$ENV{EXE_SIZE}/g;
  }
  s/(data-release-version>)[^<]*/$1$ENV{VERSION}/g;
  s/(data-release-dmg-size>)[^<]*/$1$ENV{DMG_SIZE}/g;
  s/(data-release-date>)[^<]*/$1$ENV{RELEASE_DATE}/g;
  s/("softwareVersion":\s*")[^"]*/$1$ENV{VERSION}/;
  s/(<!-- release-notes:start -->\n).*?(<!-- release-notes:end -->)/$1$ENV{RELEASE_NOTES}\n$2/s if length $ENV{RELEASE_NOTES};
' "$INDEX_HTML"

echo "官网已同步到 $VERSION（DMG $DMG_SIZE${EXE_SIZE:+，Windows $EXE_SIZE}，$RELEASE_DATE）"
