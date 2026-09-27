package winapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The download is a single executable. Run from anywhere, it installs itself
// per user — no administrator rights — to %LOCALAPPDATA%\Programs\XMindAutoSave,
// adds a Start menu entry and an entry under Settings → Apps, and starts the
// installed copy. Running a newer download later replaces the installed copy.

const (
	mainWindowClass   = "XMindAutoSave.Main"
	uninstallKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\XMindAutoSave`
	shortcutFileName  = appDisplayName + ".lnk"
	projectURL        = "https://github.com/XiangXiaoYuan5254/xmind-autosave"
	publisherName     = "向小园"
	installedArgument = "--installed"
	uninstallArgument = "--uninstall"
	portableArgument  = "--portable"
	probeArgument     = "--probe"
)

func installedExecutable() (string, error) {
	directory, err := installDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, executableName), nil
}

func startMenuShortcut() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, `Microsoft\Windows\Start Menu\Programs`, shortcutFileName)
}

func findMainWindow() uintptr {
	return call(procFindWindowW, uintptr(unsafe.Pointer(utf16Ptr(mainWindowClass))), 0)
}

// stopRunningInstance asks a running copy to quit and waits until it has
// exited, so its executable can be replaced or deleted.
func stopRunningInstance(timeout time.Duration) bool {
	hwnd := findMainWindow()
	if hwnd == 0 {
		return true
	}
	processID, _ := windowProcess(hwnd)
	const synchronize = 0x00100000
	process, err := syscall.OpenProcess(synchronize, false, processID)
	postMessage(hwnd, wmAppQuit, 0, 0)
	if err == nil {
		defer syscall.CloseHandle(process)
		event, _ := syscall.WaitForSingleObject(process, uint32(timeout/time.Millisecond))
		return event == syscall.WAIT_OBJECT_0
	}
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if findMainWindow() == 0 {
			return true
		}
	}
	return false
}

func runInstall(source, target, version string) int {
	fail := func(message string, err error) int {
		messageBox(0, fmt.Sprintf("%s\n\n%v", message, err), appDisplayName, mbOK|mbIconError)
		return 1
	}

	if !stopRunningInstance(10 * time.Second) {
		messageBox(0, "正在运行的 XMind 自动保存没有退出。\n\n请右键单击任务栏通知区域中的图标，选择“退出 XMind 自动保存”，然后重新运行安装程序。",
			appDisplayName, mbOK|mbIconWarning)
		return 1
	}
	if err := copyExecutable(source, target); err != nil {
		return fail("无法安装到 "+filepath.Dir(target)+"。", err)
	}

	log := newLogger(dataDirectory())
	log.printf("已安装 %s 到 %s", version, target)
	if shortcut := startMenuShortcut(); shortcut != "" {
		if err := createShortcut(shortcut, target, "", "为 XMind 本地文档自动保存"); err != nil {
			log.printf("无法创建开始菜单快捷方式：%v", err)
		}
	}
	if err := registerUninstaller(target, version); err != nil {
		log.printf("无法写入卸载信息：%v", err)
	}

	installed := exec.Command(target, installedArgument)
	installed.Dir = filepath.Dir(target) // do not keep the download folder busy
	if err := installed.Start(); err != nil {
		return fail("已安装，但无法启动 XMind 自动保存。", err)
	}
	return 0
}

// copyExecutable writes next to the target and then renames over it, retrying
// while the previous copy is still being released by the exiting process or
// scanned by antivirus software.
func copyExecutable(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temporary := target + ".new"
	if err := os.WriteFile(temporary, data, 0o755); err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		err = os.Rename(temporary, target)
		if err == nil || attempt == 40 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		_ = os.Remove(temporary)
	}
	return err
}

func registerUninstaller(target, version string) error {
	textValues := map[string]string{
		"DisplayName":     appDisplayName,
		"DisplayVersion":  version,
		"Publisher":       publisherName,
		"DisplayIcon":     target,
		"InstallLocation": filepath.Dir(target),
		"UninstallString": fmt.Sprintf(`"%s" %s`, target, uninstallArgument),
		"URLInfoAbout":    projectURL,
	}
	for name, value := range textValues {
		if err := setRegistryString(uninstallKeyPath, name, value); err != nil {
			return err
		}
	}
	estimatedKilobytes := uint32(0)
	if info, err := os.Stat(target); err == nil {
		estimatedKilobytes = uint32(info.Size() / 1024)
	}
	for name, value := range map[string]uint32{"NoModify": 1, "NoRepair": 1, "EstimatedSize": estimatedKilobytes} {
		if err := setRegistryDWORD(uninstallKeyPath, name, value); err != nil {
			return err
		}
	}
	return nil
}

// runUninstall is started by Settings → Apps → Uninstall.
func runUninstall() int {
	if messageBox(0, "确定要卸载 XMind 自动保存吗？", appDisplayName, mbYesNo|mbIconQuestion) != idYes {
		return 0
	}
	stopRunningInstance(10 * time.Second)

	_ = deleteRegistryValue(runKeyPath, appName)
	_ = deleteRegistryValue(startupApprovedKeyPath, appName)
	_ = deleteRegistryKey(uninstallKeyPath)
	_ = deleteRegistryKey(settingsKeyPath)
	if shortcut := startMenuShortcut(); shortcut != "" {
		_ = os.Remove(shortcut)
	}

	question := "是否同时删除各个文件的自动保存开关记录？\n\n它们保存在 " + dataDirectory() + "。"
	if messageBox(0, question, appDisplayName, mbYesNo|mbIconQuestion) == idYes {
		_ = os.RemoveAll(dataDirectory())
	}

	messageBox(0, "XMind 自动保存已卸载。", appDisplayName, mbOK|mbIconInformation)

	directory, err := installDirectory()
	if err != nil {
		return 0
	}
	executable := currentExecutable()
	if strings.HasPrefix(strings.ToLower(executable), strings.ToLower(directory)+`\`) {
		removeDirectoryAfterExit(directory)
	} else {
		_ = os.RemoveAll(directory)
	}
	return 0
}

// removeDirectoryAfterExit deletes the install directory once this process —
// whose executable lives in it — has exited. cmd runs from System32 so it does
// not hold the directory open and "ping" resolves to the system copy.
func removeDirectoryAfterExit(directory string) {
	const createNoWindow = 0x08000000
	system := filepath.Join(os.Getenv("SystemRoot"), "System32")
	command := exec.Command(filepath.Join(system, "cmd.exe"))
	command.Dir = system
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
		// Only one quoted argument, so cmd /c keeps the quotes as written.
		CmdLine: fmt.Sprintf(`cmd.exe /d /c ping -n 3 127.0.0.1 >nul & rd /s /q "%s"`, directory),
	}
	_ = command.Start()
}

func shellOpen(path string) {
	const swShowNormal = 1
	call(procShellExecuteW, 0, uintptr(unsafe.Pointer(utf16Ptr("open"))), uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, swShowNormal)
}
