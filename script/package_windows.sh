#!/usr/bin/env bash
set -euo pipefail

# 在 macOS（或任何装有 Go 的系统）上交叉编译 Windows 版。
# 产物是一个 exe：双击即按当前用户安装（无需管理员权限）并启动。
#
# 用法：./script/package_windows.sh 1.0.3

VERSION="${1:-}"
APP_NAME="XMindAutoSave"
GO_WINRES_VERSION="v0.3.3"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "用法：$0 <版本号>，版本号必须是 x.y.z 格式" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WINDOWS_DIR="$ROOT_DIR/windows"
COMMAND_DIR="$WINDOWS_DIR/cmd/xmindautosave"
RELEASE_DIR="$ROOT_DIR/dist/release"
EXE_NAME="$APP_NAME-Setup-$VERSION.exe"
EXE_PATH="$RELEASE_DIR/$EXE_NAME"

if ! command -v go >/dev/null 2>&1; then
  echo "需要 Go 1.22 或更高版本：https://go.dev/dl/" >&2
  exit 1
fi

GO_WINRES="$(go env GOPATH)/bin/go-winres"
if [[ ! -x "$GO_WINRES" ]]; then
  echo "安装资源工具 go-winres $GO_WINRES_VERSION…"
  go install "github.com/tc-hib/go-winres@$GO_WINRES_VERSION"
fi

cd "$WINDOWS_DIR"
go test ./internal/core/...
GOOS=windows GOARCH=amd64 go vet ./...

# 图标、版本信息和清单（DPI 感知、asInvoker）
(
  cd "$COMMAND_DIR"
  "$GO_WINRES" make \
    --in winres/winres.json \
    --arch amd64 \
    --out rsrc \
    --product-version "$VERSION.0" \
    --file-version "$VERSION.0"
)

mkdir -p "$RELEASE_DIR"
rm -f "$EXE_PATH" "$EXE_PATH.sha256"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$VERSION" \
  -o "$EXE_PATH" \
  ./cmd/xmindautosave

(
  cd "$RELEASE_DIR"
  shasum -a 256 "$EXE_NAME" >"$EXE_NAME.sha256"
)

echo "已生成："
echo "  $EXE_PATH"
echo "  $EXE_PATH.sha256"
