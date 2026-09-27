package winapp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

const (
	wmAppQuit           = wmApp + 1
	wmAppAlreadyRunning = wmApp + 2
	wmAppWorkerDone     = wmApp + 3
	wmAppTray           = wmApp + 4

	timerPoll      = 1
	timerForceSave = 2

	eventSystemForeground     = 0x0003
	eventSystemMoveSizeStart  = 0x000A
	eventSystemMoveSizeEnd    = 0x000B
	eventSystemMinimizeStart  = 0x0016
	eventSystemMinimizeEnd    = 0x0017
	winEventSkipOwnProcess    = 0x0002
	accessibilityRecheckAfter = 20 * time.Second
)

const (
	commandToggleCurrent = iota + 1
	commandSaveNow
	commandLaunchAtLogin
	commandOpenDataFolder
	commandDiagnostics
	commandQuit
)

type appOptions struct {
	version       string
	configuration core.Configuration
	configError   error
	installed     bool
	justInstalled bool
	executable    string
}

type App struct {
	appOptions
	store     *core.PerFileStore
	scheduler *core.Scheduler
	log       *logger
	launch    launchAtLogin
	processes *processNames
	ownPID    uint32

	hwnd           uintptr
	tray           *trayIcon
	panel          *togglePanel
	worker         *worker
	taskbarCreated uint32
	winEventHook   uintptr

	// The XMind window being followed and its document.
	window         uintptr
	focused        bool
	documents      map[uintptr]*documentEntry
	currentPath    string
	currentEnabled bool
	movingWindow   uintptr
	dark           bool

	// letterSinceCommit is true while the last key typed in XMind was a
	// letter: with a Chinese input method on, a composition may be open.
	letterSinceCommit bool

	status           string
	lastLoggedSave   time.Time
	showWhenInactive bool
}

// documentEntry caches which file a window shows, per window title.
type documentEntry struct {
	titleKey    string
	path        string
	method      string
	inflight    bool
	attempts    int
	nextAttempt time.Time
}

var theApp *App

var mainWindowProc = syscall.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	if theApp != nil {
		if result, handled := theApp.handleMessage(uint32(msg), wParam, lParam); handled {
			return result
		}
	}
	return defWindowProc(hwnd, uint32(msg), wParam, lParam)
})

var winEventProc = syscall.NewCallback(func(hook, event, hwnd, objectID, childID, threadID, eventTime uintptr) uintptr {
	if theApp != nil && int32(uint32(objectID)) == 0 { // OBJID_WINDOW
		theApp.handleWinEvent(uint32(event), hwnd)
	}
	return 0
})

func newApp(options appOptions) (*App, error) {
	directory := dataDirectory()
	a := &App{
		appOptions: options,
		log:        newLogger(directory),
		launch:     launchAtLogin{executable: options.executable},
		processes:  newProcessNames(),
		ownPID:     uint32(os.Getpid()),
		documents:  map[uintptr]*documentEntry{},
		dark:       systemUsesDarkTheme(),
		status:     "正在启动…",

		showWhenInactive: os.Getenv("XMIND_AUTOSAVE_SHOW_WHEN_INACTIVE") == "1",
	}
	configuration := options.configuration
	a.store = core.NewPerFileStore(filepath.Join(directory, "preferences.json"), fileIdentityKey, configuration.MonitoredFiles)
	a.scheduler = core.NewScheduler(configuration.SaveDelay(), configuration.RetryInterval())

	if err := registerWindowClass(mainWindowClass, mainWindowProc, call(procLoadCursorW, 0, idcArrow)); err != nil {
		return nil, err
	}
	hwnd, err := createWindow(0, mainWindowClass, appDisplayName, wsPopup)
	if err != nil {
		return nil, err
	}
	a.hwnd = hwnd
	theApp = a

	a.taskbarCreated = uint32(call(procRegisterWindowMessageW, uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated")))))
	a.tray = newTrayIcon(hwnd, wmAppTray)
	a.tray.setTooltip(appDisplayName + "：" + a.status)
	a.tray.add()

	a.panel, err = newTogglePanel(func() {
		if a.currentPath != "" {
			a.setAutoSave(!a.currentEnabled, a.currentPath)
		}
	})
	if err != nil {
		a.log.printf("无法创建悬浮开关：%v", err)
	}
	a.worker = startWorker(hwnd)

	if err := registerRawInput(hwnd); err != nil {
		a.log.printf("无法监听键盘和鼠标输入：%v", err)
	}
	a.winEventHook = call(procSetWinEventHook, eventSystemForeground, eventSystemMinimizeEnd, 0, winEventProc, 0, 0, winEventSkipOwnProcess)

	if a.installed {
		if err := a.launch.configureDefault(); err != nil {
			a.log.printf("无法设置登录时自动启动：%v", err)
		}
	}
	a.log.printf("启动 %s（%s）", a.version, a.executable)
	return a, nil
}

func (a *App) run() int {
	if a.configError != nil {
		a.setStatus("配置读取失败")
		a.log.printf("配置读取失败：%v", a.configError)
	} else {
		call(procSetTimer, a.hwnd, timerPoll, uintptr(a.configuration.PollInterval()/time.Millisecond), 0)
		a.poll(false)
	}
	if a.justInstalled {
		a.tray.notify("XMind 自动保存已安装",
			"它在后台运行，并会随 Windows 登录自动启动。打开 XMind 文档后，用标题栏旁的“自动保存”开关为当前文件开启。")
	}
	runMessageLoop()
	return 0
}

func (a *App) quit() {
	call(procKillTimer, a.hwnd, timerPoll)
	if a.winEventHook != 0 {
		call(procUnhookWinEvent, a.winEventHook)
		a.winEventHook = 0
	}
	if a.panel != nil {
		a.panel.hide()
	}
	a.tray.remove()
	a.log.printf("退出")
	call(procDestroyWindow, a.hwnd)
}

func (a *App) handleMessage(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case wmTimer:
		switch wParam {
		case timerPoll:
			a.poll(false)
		case timerForceSave:
			call(procKillTimer, a.hwnd, timerForceSave)
			a.poll(true)
		}
		return 0, true
	case wmInput:
		a.handleRawInput(lParam)
		return 0, false // DefWindowProc must still clean up the input
	case wmAppTray:
		switch uint32(loword(lParam)) {
		case wmLButtonUp, wmRButtonUp:
			a.showMenu()
		}
		return 0, true
	case wmAppWorkerDone:
		a.worker.runCompletions()
		return 0, true
	case wmAppQuit:
		a.quit()
		return 0, true
	case wmAppAlreadyRunning:
		a.tray.notify(appDisplayName, "已经在运行。单击任务栏通知区域中的图标可以查看状态和设置。")
		return 0, true
	case wmSettingChange:
		a.dark = systemUsesDarkTheme()
		return 0, false
	case wmEndSession:
		if wParam != 0 {
			a.tray.remove()
		}
		return 0, true
	case wmDestroy:
		call(procPostQuitMessage, 0)
		return 0, true
	}
	if a.taskbarCreated != 0 && msg == a.taskbarCreated {
		a.tray.add() // Explorer restarted
		return 0, true
	}
	return 0, false
}

func (a *App) handleWinEvent(event uint32, hwnd uintptr) {
	switch event {
	case eventSystemMoveSizeStart:
		if rootWindow(hwnd) == a.window && a.window != 0 {
			a.movingWindow = a.window
			if a.panel != nil {
				a.panel.hide()
			}
		}
	case eventSystemMoveSizeEnd:
		a.movingWindow = 0
		a.poll(false)
	case eventSystemForeground, eventSystemMinimizeStart, eventSystemMinimizeEnd:
		a.poll(false)
	}
}

// handleRawInput turns keyboard and mouse input in the followed XMind window
// into edit activity for the scheduler.
func (a *App) handleRawInput(handle uintptr) {
	event, ok := readRawInput(handle)
	if !ok || a.window == 0 {
		return
	}
	if foreground := rootWindow(foregroundWindow()); foreground != a.window || !a.isXMindWindow(foreground) {
		return
	}
	if event.mouseRelease {
		var cursor point
		call(procGetCursorPos, uintptr(unsafe.Pointer(&cursor)))
		under := call(procWindowFromPoint, uintptr(uint32(cursor.X))|uintptr(uint32(cursor.Y))<<32)
		if processID, _ := windowProcess(under); processID == a.ownPID {
			return // a click on the toggle panel is not an edit
		}
		a.letterSinceCommit = false
	} else {
		a.letterSinceCommit = isLetterKey(event.virtualKey) && !controlOrAltDown()
	}
	if a.currentPath != "" && a.currentEnabled {
		a.scheduler.NoteEdit(a.currentPath, time.Now())
	}
}

func (a *App) isXMindWindow(hwnd uintptr) bool {
	processID, _ := windowProcess(hwnd)
	return processID != 0 && processID != a.ownPID &&
		matchesProcessName(a.processes.name(processID), a.configuration.ProcessNames)
}

// locateXMindWindow returns XMind's foreground window, or — when XMind is in
// the background — the XMind window used most recently.
func (a *App) locateXMindWindow() (hwnd uintptr, focused bool) {
	if foreground := rootWindow(foregroundWindow()); foreground != 0 && a.isXMindWindow(foreground) {
		return foreground, true
	}
	if isWindow(a.window) && isWindowVisible(a.window) && a.isXMindWindow(a.window) {
		return a.window, false
	}
	for _, candidate := range enumerateWindows(0) { // front to back
		if isWindowVisible(candidate) && windowText(candidate) != "" && a.isXMindWindow(candidate) {
			return candidate, false
		}
	}
	return 0, false
}

func (a *App) poll(force bool) {
	if a.configError != nil {
		return
	}
	now := time.Now()
	window, focused := a.locateXMindWindow()
	a.window, a.focused = window, focused
	if window == 0 {
		a.clearDocument()
		a.setStatus("等待 XMind")
		return
	}

	title := windowText(window)
	entry := a.documentFor(window, title, now)
	if entry.path == "" {
		a.clearDocument()
		if entry.attempts == 0 {
			a.setStatus("正在识别当前文件…")
		} else {
			a.setStatus("当前不是本地 XMind 文档")
		}
		return
	}

	path := entry.path
	enabled := a.store.IsEnabled(path)
	a.currentPath, a.currentEnabled = path, enabled

	_, threadID := windowProcess(window)
	gui, _ := threadGUIInfo(threadID)
	busy := gui.Flags&(guiInMoveSize|guiInMenuMode|guiSystemMenuMode|guiPopupMenuMode) != 0
	bounds := windowBounds(window)
	if a.panel != nil {
		if (focused || a.showWhenInactive) && a.movingWindow == 0 && !bounds.empty() &&
			!isMinimized(window) && !isFullScreen(window, bounds) {
			a.panel.show(window, bounds, enabled, a.dark)
		} else {
			a.panel.hide()
		}
	}

	composing := false
	if enabled && focused && a.letterSinceCommit {
		target := gui.Focus
		if target == 0 {
			target = window
		}
		composing, _ = imeInNativeMode(target)
	}

	decision := a.scheduler.Evaluate(core.Observation{
		Now:        now,
		Path:       path,
		Enabled:    enabled,
		TitleDirty: core.TitleIndicatesDirty(title, a.configuration.DirtyIndicators, core.DocumentStem(path)),
		ModTime:    fileModTime(path),
		Focused:    focused && !busy,
		InputHeld:  inputHeld(),
		Composing:  composing,
		Force:      force,
	})
	if decision.Save {
		// Re-check right before typing: the keys go to whatever is in front.
		if rootWindow(foregroundWindow()) == window && sendControlS() {
			a.log.printf("发送 Ctrl+S：%s", path)
		} else {
			a.log.printf("未能发送 Ctrl+S：%s", path)
		}
	}
	if decision.Status == core.StatusAutoSaved && !decision.LastSaved.Equal(a.lastLoggedSave) {
		a.lastLoggedSave = decision.LastSaved
		a.log.printf("已自动保存：%s", path)
	}
	a.setStatus(statusText(decision))
}

func statusText(decision core.Decision) string {
	switch decision.Status {
	case core.StatusDisabled:
		return "当前文件自动保存已关闭"
	case core.StatusAutoSaved:
		return "已自动保存 " + decision.LastSaved.Format("15:04:05")
	case core.StatusEditing:
		return "检测到编辑…"
	case core.StatusSaving:
		return "正在自动保存…"
	case core.StatusWaitingForFocus:
		return "有未保存的编辑，回到 XMind 后自动保存"
	default:
		return "已保存"
	}
}

// documentFor returns the cached document of a window and starts a lookup on
// the worker when one is due.
func (a *App) documentFor(window uintptr, title string, now time.Time) *documentEntry {
	key := core.NormalizedTitle(title, a.configuration.DirtyIndicators)
	entry := a.documents[window]
	if entry == nil || entry.titleKey != key {
		if len(a.documents) > 32 {
			for hwnd := range a.documents {
				if !isWindow(hwnd) {
					delete(a.documents, hwnd)
				}
			}
		}
		entry = &documentEntry{titleKey: key}
		a.documents[window] = entry
	}
	if entry.inflight || now.Before(entry.nextAttempt) {
		return entry
	}

	entry.inflight = true
	submitted := a.worker.submit(func() {
		path, method, err := resolveDocument(window, title)
		a.worker.complete(func() { a.documentResolved(window, key, path, method, err) })
	})
	if !submitted {
		entry.inflight = false
	}
	return entry
}

func resolveDocument(window uintptr, title string) (path, method string, err error) {
	path, accessibilityErr := documentPathFromAccessibility(window)
	if accessibilityErr == nil {
		return longPath(path), methodAccessibility, nil
	}
	path, recentErr := documentPathFromRecent(title)
	if recentErr == nil {
		return longPath(path), methodRecentShortcuts, nil
	}
	return "", "", errors.Join(accessibilityErr, recentErr)
}

func (a *App) documentResolved(window uintptr, key, path, method string, err error) {
	entry := a.documents[window]
	if entry == nil || entry.titleKey != key {
		return // the window shows something else by now
	}
	entry.inflight = false
	entry.attempts++

	if path != "" && (path != entry.path || method != entry.method) {
		a.log.printf("当前文件：%s（通过%s识别）", path, methodDescription(method))
		entry.path, entry.method = path, method
	} else if path == "" && entry.attempts == 1 {
		a.log.printf("无法识别窗口“%s”的文件：%v", key, err)
	}

	// Accessibility answers are exact; re-check now and then in case the
	// window switched documents without changing its title. Anything else
	// keeps trying with backoff, as XMind builds its accessibility tree only
	// after it is first asked for it.
	if entry.method == methodAccessibility {
		entry.nextAttempt = time.Now().Add(accessibilityRecheckAfter)
	} else {
		backoff := time.Second << min(entry.attempts-1, 5)
		entry.nextAttempt = time.Now().Add(min(backoff, 30*time.Second))
	}
	a.poll(false)
}

func methodDescription(method string) string {
	if method == methodAccessibility {
		return "辅助功能信息"
	}
	return "最近使用的文件"
}

func (a *App) clearDocument() {
	a.currentPath, a.currentEnabled = "", false
	if a.panel != nil {
		a.panel.hide()
	}
}

func (a *App) setAutoSave(enabled bool, path string) {
	if path == "" {
		return
	}
	if err := a.store.SetEnabled(enabled, path); err != nil {
		a.setStatus("无法保存文件开关设置")
		a.log.printf("无法保存文件开关设置：%v", err)
		return
	}
	a.currentEnabled = enabled
	if enabled {
		a.setStatus("当前文件已开启自动保存")
	} else {
		a.scheduler.Forget(path)
		a.setStatus("当前文件自动保存已关闭")
	}
	a.poll(false)
}

func (a *App) setStatus(text string) {
	if text == a.status {
		return
	}
	a.status = text
	a.tray.setTooltip(appDisplayName + "：" + text)
	directory := dataDirectory()
	if os.MkdirAll(directory, 0o755) == nil {
		report := time.Now().Format(time.RFC3339) + "\r\n" + text + "\r\n"
		_ = os.WriteFile(filepath.Join(directory, "status.txt"), []byte(report), 0o644)
	}
}

func (a *App) showMenu() {
	documentLine := "当前文件：无本地文档"
	switch {
	case a.window == 0:
		documentLine = "当前文件：等待 XMind"
	case a.currentPath != "":
		documentLine = "当前文件：" + filepath.Base(a.currentPath)
	}
	escape := func(text string) string { return strings.ReplaceAll(text, "&", "&&") }

	xmindWindow, currentPath := a.window, a.currentPath
	command := showPopupMenu(a.hwnd, []menuItem{
		{title: escape("状态：" + a.status), disabled: true},
		{title: escape(documentLine), disabled: true},
		{id: commandToggleCurrent, title: "当前文件自动保存", checked: currentPath != "" && a.currentEnabled, disabled: currentPath == ""},
		{separator: true},
		{id: commandSaveNow, title: "立即检查并保存"},
		{id: commandLaunchAtLogin, title: "登录时自动启动", checked: a.launch.enabled()},
		{id: commandOpenDataFolder, title: "打开数据文件夹"},
		{id: commandDiagnostics, title: "生成诊断报告"},
		{separator: true},
		{id: commandQuit, title: "退出 XMind 自动保存"},
	})

	switch command {
	case commandToggleCurrent:
		a.setAutoSave(!a.store.IsEnabled(currentPath), currentPath)
		a.returnToXMind(xmindWindow, false)
	case commandSaveNow:
		a.returnToXMind(xmindWindow, true)
	case commandLaunchAtLogin:
		if err := a.launch.setEnabled(!a.launch.enabled()); err != nil {
			a.log.printf("无法更改登录时自动启动：%v", err)
		}
	case commandOpenDataFolder:
		_ = os.MkdirAll(dataDirectory(), 0o755)
		shellOpen(dataDirectory())
	case commandDiagnostics:
		a.writeDiagnostics()
	case commandQuit:
		a.quit()
	}
}

// returnToXMind gives the keyboard back to XMind after the tray menu took it
// and, for "立即检查并保存", saves once XMind is in front again.
func (a *App) returnToXMind(window uintptr, save bool) {
	if isWindow(window) {
		call(procSetForegroundWindow, window)
	}
	if save {
		call(procSetTimer, a.hwnd, timerForceSave, 250, 0)
	}
}
