package winapp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

// The diagnostics report is the Windows counterpart of Tools/AXProbe.swift:
// it records what the app can see of XMind (windows, accessibility tree,
// recent documents, input method) so detection problems can be fixed without
// access to the affected machine.

const diagnosticsNodeLimit = 300

func (a *App) writeDiagnostics() {
	snapshot := fmt.Sprintf("状态：%s\r\n当前文件：%s\r\n当前文件自动保存：%v\r\n已安装：%v\r\n登录时自动启动：%v\r\n",
		a.status, a.currentPath, a.currentEnabled, a.installed, a.launch.enabled())
	for hwnd, entry := range a.documents {
		snapshot += fmt.Sprintf("缓存：窗口 0x%X “%s” → %s（%s，尝试 %d 次）\r\n",
			hwnd, entry.titleKey, entry.path, entry.method, entry.attempts)
	}
	version, configuration := a.version, a.configuration
	a.setStatus("正在生成诊断报告…")
	queued := a.worker.submit(func() {
		path, err := saveDiagnostics(version, configuration, snapshot)
		a.worker.complete(func() {
			if err != nil {
				a.log.printf("无法生成诊断报告：%v", err)
				messageBox(a.hwnd, "无法生成诊断报告：\n"+err.Error(), appDisplayName, mbOK|mbIconError)
				return
			}
			shellOpen(path)
			a.poll(false)
		})
	})
	if !queued {
		a.poll(false)
	}
}

func saveDiagnostics(version string, configuration core.Configuration, snapshot string) (string, error) {
	report := buildDiagnostics(version, configuration, snapshot)
	directory := dataDirectory()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "diagnostics.txt")
	// UTF-8 with BOM so Notepad on older Windows picks the right encoding.
	return path, os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, report...), 0o644)
}

func buildDiagnostics(version string, configuration core.Configuration, snapshot string) string {
	var report strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&report, format+"\r\n", args...)
	}
	section := func(title string) {
		line("")
		line("== %s ==", title)
	}

	line("XMind 自动保存 · 诊断报告")
	line("此报告只保存在本机。其中包含 XMind 窗口标题和文件路径，发给他人前请先检查。")
	line("")
	line("生成时间：%s", time.Now().Format("2006-01-02 15:04:05"))
	line("版本：%s", version)
	line("Windows：%s", windowsVersion())
	line("程序位置：%s", currentExecutable())
	if snapshot != "" {
		report.WriteString(snapshot)
	}
	if data, err := json.MarshalIndent(configuration, "", "  "); err == nil {
		section("配置")
		report.WriteString(strings.ReplaceAll(string(data), "\n", "\r\n") + "\r\n")
	}

	processes := newProcessNames()
	isXMind := func(hwnd uintptr) bool {
		processID, _ := windowProcess(hwnd)
		return processID != 0 && matchesProcessName(processes.name(processID), configuration.ProcessNames)
	}

	section("前台窗口")
	foreground := rootWindow(foregroundWindow())
	line("%s", describeWindow(foreground, processes))

	section("XMind 窗口")
	var target uintptr
	for _, hwnd := range enumerateWindows(0) {
		if !isXMind(hwnd) || !isWindowVisible(hwnd) {
			continue
		}
		line("%s", describeWindow(hwnd, processes))
		if target == 0 || hwnd == foreground {
			target = hwnd
		}
	}
	if target == 0 {
		line("没有找到可见的 XMind 窗口（进程名：%s）。", strings.Join(configuration.ProcessNames, "、"))
	} else {
		describeDocumentDetection(&report, target, configuration)
	}

	section("“最近使用”中的 .xmind 快捷方式")
	documents := recentDocuments()
	if len(documents) == 0 {
		line("（无）")
	}
	for index, document := range documents {
		if index == 15 {
			line("……共 %d 个", len(documents))
			break
		}
		target, err := shortcutTarget(document.shortcut)
		if err != nil {
			target = "无法读取：" + err.Error()
		}
		line("%s  %s → %s", document.modified.Format("2006-01-02 15:04"), document.name, target)
	}

	section("最近日志")
	if data, err := os.ReadFile(filepath.Join(dataDirectory(), "log.txt")); err == nil {
		lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
		report.WriteString(strings.Join(lines[max(0, len(lines)-80):], "\n") + "\r\n")
	}
	return report.String()
}

func describeDocumentDetection(report *strings.Builder, hwnd uintptr, configuration core.Configuration) {
	line := func(format string, args ...any) {
		fmt.Fprintf(report, format+"\r\n", args...)
	}
	title := windowText(hwnd)

	line("")
	line("== 子窗口（0x%X）==", hwnd)
	counts := map[string]int{}
	for _, child := range enumerateWindows(hwnd) {
		counts[windowClass(child)]++
	}
	for class, count := range counts {
		line("%s × %d", class, count)
	}

	line("")
	line("== 辅助功能树（每个根最多 %d 个节点）==", diagnosticsNodeLimit)
	roots := accessibleRoots(hwnd)
	if len(roots) == 0 {
		line("无法取得辅助功能对象。")
	}
	for index, root := range roots {
		line("-- 根 %d --", index+1)
		walkAccessible(root, diagnosticsNodeLimit, time.Now().Add(3*time.Second), func(node *comObject, depth int, role int32) bool {
			name := truncate(node.accessibleString(accName), 60)
			value := truncate(node.accessibleString(accValue), 300)
			line("%s%s  name=%q  value=%q", strings.Repeat("  ", depth), roleName(role), name, value)
			return false
		})
		root.release()
	}

	line("")
	line("== 识别结果 ==")
	accessiblePath, err := documentPathFromAccessibility(hwnd)
	if err == nil {
		line("辅助功能：%s", accessiblePath)
	} else {
		line("辅助功能：失败（%v）", err)
	}
	if path, err := documentPathFromRecent(title); err == nil {
		line("最近使用：%s", path)
	} else {
		line("最近使用：失败（%v）", err)
	}
	line("标题显示未保存：%v", core.TitleIndicatesDirty(title, configuration.DirtyIndicators, core.DocumentStem(accessiblePath)))

	_, threadID := windowProcess(hwnd)
	gui, _ := threadGUIInfo(threadID)
	focus := gui.Focus
	if focus == 0 {
		focus = hwnd
	}
	native, known := imeInNativeMode(focus)
	line("输入法：中文模式=%v，可读取=%v（焦点窗口 0x%X，类 %s）", native, known, focus, windowClass(focus))
}

func describeWindow(hwnd uintptr, processes *processNames) string {
	if hwnd == 0 {
		return "（无）"
	}
	processID, _ := windowProcess(hwnd)
	bounds := windowBounds(hwnd)
	return fmt.Sprintf("0x%X  进程 %d %s  类 %s  标题 %q  位置 (%d,%d) 大小 %d×%d  DPI %d  最大化=%v 最小化=%v 全屏=%v",
		hwnd, processID, processes.name(processID), windowClass(hwnd), windowText(hwnd),
		bounds.Left, bounds.Top, bounds.width(), bounds.height(), dpiForWindow(hwnd),
		isMaximized(hwnd), isMinimized(hwnd), isFullScreen(hwnd, bounds))
}

func roleName(role int32) string {
	names := map[int32]string{
		0x09: "window", 0x0A: "client", 0x0F: "document", 0x10: "pane", 0x14: "grouping",
		0x1E: "link", 0x21: "list", 0x22: "listitem", 0x23: "outline", 0x24: "outlineitem",
		0x28: "graphic", 0x29: "text", 0x2A: "editable", 0x2B: "button", 0x2C: "checkbox",
		0x39: "buttonmenu", 0x3C: "tab", 0x3D: "tabs", 0x0B: "menupopup", 0x0C: "menuitem",
		0x02: "menubar", 0x01: "titlebar", 0x16: "toolbar", 0x17: "statusbar",
	}
	if name, ok := names[role]; ok {
		return name
	}
	return fmt.Sprintf("role(0x%X)", role)
}

func truncate(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

type osVersionInfo struct {
	Size         uint32
	MajorVersion uint32
	MinorVersion uint32
	BuildNumber  uint32
	PlatformID   uint32
	CSDVersion   [128]uint16
}

func windowsVersion() string {
	info := osVersionInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	if !available(procRtlGetVersion) || call(procRtlGetVersion, uintptr(unsafe.Pointer(&info))) != 0 {
		return "未知"
	}
	return fmt.Sprintf("%d.%d.%d", info.MajorVersion, info.MinorVersion, info.BuildNumber)
}
