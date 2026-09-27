// Package winapp is XMind Auto Save for Windows: a notification-area app that
// watches the XMind document in front and sends Ctrl+S once editing pauses.
package winapp

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

// Windows, hooks and COM objects belong to the thread that created them, so
// the UI stays on the main OS thread.
func init() {
	runtime.LockOSThread()
}

// Main runs the app and returns the process exit code.
//
// Arguments:
//
//	(none)        install per user if not yet installed, otherwise run
//	--portable    run from the current location without installing
//	--uninstall   remove the app (used by Settings → Apps)
//	--probe       write the diagnostics report and open it
func Main(version string, defaultConfiguration []byte) int {
	arguments := os.Args[1:]
	if hasArgument(arguments, uninstallArgument) {
		return runUninstall()
	}

	enableHighDPI()
	configuration, configError := loadConfiguration(defaultConfiguration)
	if hasArgument(arguments, probeArgument) {
		_ = initializeCOM()
		path, err := saveDiagnostics(version, configuration, "")
		if err != nil {
			messageBox(0, "无法生成诊断报告：\n"+err.Error(), appDisplayName, mbOK|mbIconError)
			return 1
		}
		shellOpen(path)
		return 0
	}

	_ = initializeCOM()

	executable := currentExecutable()
	target, err := installedExecutable()
	installed := err == nil && samePath(executable, target)
	if err == nil && !installed && !hasArgument(arguments, portableArgument) {
		return runInstall(executable, target, version)
	}

	// One instance per user session. The handle stays open until exit.
	mutex, err := callErr(procCreateMutexW, 0, 0, uintptr(unsafe.Pointer(utf16Ptr(`Local\XMindAutoSave.Instance`))))
	if mutex != 0 && errors.Is(err, syscall.ERROR_ALREADY_EXISTS) {
		if hwnd := findMainWindow(); hwnd != 0 {
			postMessage(hwnd, wmAppAlreadyRunning, 0, 0)
		}
		return 0
	}

	app, err := newApp(appOptions{
		version:       version,
		configuration: configuration,
		configError:   configError,
		installed:     installed,
		justInstalled: hasArgument(arguments, installedArgument),
		executable:    executable,
	})
	if err != nil {
		messageBox(0, "无法启动 XMind 自动保存：\n"+err.Error(), appDisplayName, mbOK|mbIconError)
		return 1
	}
	return app.run()
}

// loadConfiguration reads XMIND_AUTOSAVE_CONFIG when set, else the built-in
// configuration.
func loadConfiguration(builtIn []byte) (core.Configuration, error) {
	data := builtIn
	if path := os.Getenv("XMIND_AUTOSAVE_CONFIG"); path != "" {
		custom, err := os.ReadFile(path)
		if err != nil {
			return core.DefaultConfiguration(), err
		}
		data = custom
	}
	configuration, err := core.ParseConfiguration(data)
	if err != nil {
		return core.DefaultConfiguration(), err
	}
	return configuration, nil
}

func hasArgument(arguments []string, name string) bool {
	for _, argument := range arguments {
		if strings.EqualFold(argument, name) {
			return true
		}
	}
	return false
}
