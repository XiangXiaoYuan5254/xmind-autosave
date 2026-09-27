//go:build windows

// Command xmindautosave is XMind Auto Save for Windows.
package main

import (
	_ "embed"
	"os"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/winapp"
)

//go:embed config.json
var defaultConfiguration []byte

// version is set when packaging: -ldflags "-X main.version=1.0.3".
var version = "dev"

func main() {
	os.Exit(winapp.Main(version, defaultConfiguration))
}
