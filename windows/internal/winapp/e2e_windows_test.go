//go:build e2e

package winapp

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

// End-to-end tests on a Windows desktop, run by .github/workflows/windows.yml
// on a GitHub-hosted runner with the latest XMind installed:
//
//	XMIND_EXE           path of Xmind.exe
//	XMIND_AUTOSAVE_EXE  the XMindAutoSave executable under test
//	E2E_ARTIFACTS       where screenshots, logs and reports are written
//
// Run with: go test -tags e2e -run E2E -v ./internal/winapp/

const (
	vkTab    = 0x09
	vkReturn = 0x0D
)

var (
	procBitBlt       = gdi32.NewProc("BitBlt")
	procSetCursorPos = user32.NewProc("SetCursorPos")
)

type mouseInput struct {
	Type      uint32
	_         uint32
	DX, DY    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

func clickAt(x, y int32) {
	const inputMouse, leftDown, leftUp = 0, 0x0002, 0x0004
	call(procSetCursorPos, uintptr(x), uintptr(y))
	time.Sleep(150 * time.Millisecond)
	inputs := [2]mouseInput{{Type: inputMouse, Flags: leftDown}, {Type: inputMouse, Flags: leftUp}}
	call(procSendInput, uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
}

type e2eRun struct {
	t         *testing.T
	artifacts string
	shots     int
}

func newE2ERun(t *testing.T) *e2eRun {
	directory := os.Getenv("E2E_ARTIFACTS")
	if directory == "" {
		directory = t.TempDir()
	}
	directory = filepath.Join(directory, t.Name())
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if err := initializeCOM(); err != nil {
		t.Fatal(err)
	}
	return &e2eRun{t: t, artifacts: directory}
}

func (e *e2eRun) screenshot(name string) {
	e.shots++
	path := filepath.Join(e.artifacts, fmt.Sprintf("%02d-%s.png", e.shots, name))
	if err := captureScreen(path); err != nil {
		e.t.Logf("screenshot %s failed: %v", name, err)
		return
	}
	e.t.Logf("screenshot %s", filepath.Base(path))
}

func (e *e2eRun) save(name string, data []byte) {
	if err := os.WriteFile(filepath.Join(e.artifacts, name), data, 0o644); err != nil {
		e.t.Logf("saving %s: %v", name, err)
	}
}

func (e *e2eRun) logWindows(processName string) {
	for _, hwnd := range windowsOfProcess(processName) {
		bounds := windowBounds(hwnd)
		e.t.Logf("  window 0x%X class=%s title=%q at (%d,%d) %dx%d", hwnd, windowClass(hwnd), windowText(hwnd),
			bounds.Left, bounds.Top, bounds.width(), bounds.height())
	}
}

func windowsOfProcess(processName string) []uintptr {
	names := newProcessNames()
	var result []uintptr
	for _, hwnd := range enumerateWindows(0) {
		processID, _ := windowProcess(hwnd)
		if isWindowVisible(hwnd) && strings.EqualFold(names.name(processID), processName) {
			result = append(result, hwnd)
		}
	}
	return result
}

func waitUntil(timeout, interval time.Duration, condition func() bool) bool {
	for deadline := time.Now().Add(timeout); ; time.Sleep(interval) {
		if condition() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

func sendKeys(inputs ...keyboardInput) {
	if len(inputs) > 0 {
		call(procSendInput, uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	}
}

func pressKey(virtualKey uint16) {
	const inputKeyboard, keyUp = 1, 0x0002
	sendKeys(
		keyboardInput{Type: inputKeyboard, VirtualKey: virtualKey},
		keyboardInput{Type: inputKeyboard, VirtualKey: virtualKey, Flags: keyUp},
	)
}

func typeText(text string) {
	const inputKeyboard, keyUp, unicode = 1, 0x0002, 0x0004
	for _, unit := range utf16.Encode([]rune(text)) {
		sendKeys(
			keyboardInput{Type: inputKeyboard, ScanCode: unit, Flags: unicode},
			keyboardInput{Type: inputKeyboard, ScanCode: unit, Flags: unicode | keyUp},
		)
	}
}

func captureScreen(path string) error {
	const (
		smXVirtualScreen  = 76
		smYVirtualScreen  = 77
		smCxVirtualScreen = 78
		smCyVirtualScreen = 79
		srcCopy           = 0x00CC0020
		captureBlt        = 0x40000000
	)
	left := int32(call(procGetSystemMetrics, smXVirtualScreen))
	top := int32(call(procGetSystemMetrics, smYVirtualScreen))
	width := int32(call(procGetSystemMetrics, smCxVirtualScreen))
	height := int32(call(procGetSystemMetrics, smCyVirtualScreen))
	if width <= 0 || height <= 0 {
		return fmt.Errorf("no screen (%dx%d)", width, height)
	}

	screen := call(procGetDC, 0)
	defer call(procReleaseDC, 0, screen)
	memory := call(procCreateCompatibleDC, screen)
	defer call(procDeleteDC, memory)
	info := bitmapInfo{Header: bitmapInfoHeader{Width: width, Height: -height, Planes: 1, BitCount: 32}}
	info.Header.Size = uint32(unsafe.Sizeof(info.Header))
	var pixels unsafe.Pointer
	bitmap := call(procCreateDIBSection, memory, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return fmt.Errorf("CreateDIBSection failed")
	}
	defer call(procDeleteObject, bitmap)
	previous := call(procSelectObject, memory, bitmap)
	defer call(procSelectObject, memory, previous)
	if call(procBitBlt, memory, 0, 0, uintptr(width), uintptr(height), screen, uintptr(left), uintptr(top), srcCopy|captureBlt) == 0 {
		return fmt.Errorf("BitBlt failed")
	}

	source := unsafe.Slice((*byte)(pixels), int(width)*int(height)*4)
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for index := 0; index < len(source); index += 4 {
		img.Pix[index] = source[index+2]
		img.Pix[index+1] = source[index+1]
		img.Pix[index+2] = source[index]
		img.Pix[index+3] = 255
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

// writeXMindFixture writes a minimal XMind document with one sheet.
func writeXMindFixture(path, rootTitle string) error {
	content := []map[string]any{{
		"id":    "e2e-sheet",
		"class": "sheet",
		"title": "画布 1",
		"rootTopic": map[string]any{
			"id":             "e2e-root",
			"class":          "topic",
			"title":          rootTitle,
			"structureClass": "org.xmind.ui.logic.right",
			"children": map[string]any{"attached": []map[string]any{
				{"id": "e2e-child", "title": "分支主题 1"},
			}},
		},
		"topicOverlapping": "overlap",
	}}
	files := map[string]any{
		"content.json":  content,
		"metadata.json": map[string]any{"dataStructureVersion": "3", "layoutEngineVersion": "5", "creator": map[string]any{"name": "Vana", "version": "26.02.04171"}},
		"manifest.json": map[string]any{"file-entries": map[string]any{"content.json": map[string]any{}, "metadata.json": map[string]any{}}},
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	for _, name := range []string{"content.json", "metadata.json", "manifest.json"} {
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(writer).Encode(files[name]); err != nil {
			return err
		}
	}
	return archive.Close()
}

func documentContent(path string) string {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name == "content.json" {
			reader, err := file.Open()
			if err != nil {
				return ""
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			return string(data)
		}
	}
	return ""
}

// saveCommands returns when the app logged sending Ctrl+S.
func saveCommands() []time.Time {
	file, err := os.Open(filepath.Join(dataDirectory(), "log.txt"))
	if err != nil {
		return nil
	}
	defer file.Close()
	var times []time.Time
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "发送 Ctrl+S") || strings.Contains(line, "未能") || len(line) < 23 {
			continue
		}
		if at, err := time.ParseInLocation("2006-01-02 15:04:05.000", line[:23], time.Local); err == nil {
			times = append(times, at)
		}
	}
	return times
}

func countBetween(times []time.Time, start, end time.Time) int {
	count := 0
	for _, at := range times {
		if at.After(start) && at.Before(end) {
			count++
		}
	}
	return count
}

func killProcess(imageName string) {
	_ = exec.Command("taskkill", "/IM", imageName, "/F", "/T").Run()
}

// TestE2EChromiumExposesDocumentURL checks the assumption the Windows version
// rests on, independent of XMind: Chromium publishes a page's URL as the MSAA
// value of its document, so the ?source= parameter can be read back.
func TestE2EChromiumExposesDocumentURL(t *testing.T) {
	e := newE2ERun(t)
	var edge string
	for _, candidate := range []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			edge = candidate
			break
		}
	}
	if edge == "" {
		t.Skip("Microsoft Edge is not installed")
	}

	directory := t.TempDir()
	page := filepath.Join(directory, "editor.html")
	if err := os.WriteFile(page, []byte("<!doctype html><meta charset=utf-8><title>XMind E2E Editor</title><p>editor</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := `C:\e2e docs\计划 A.xmind`
	source := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(want)}).String()
	pageURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(page), RawQuery: "source=" + url.QueryEscape(source)}).String()
	t.Logf("page URL: %s", pageURL)

	browser := exec.Command(edge, "--user-data-dir="+filepath.Join(directory, "profile"),
		"--no-first-run", "--no-default-browser-check", "--new-window", pageURL)
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killProcess("msedge.exe") })

	var window uintptr
	if !waitUntil(60*time.Second, time.Second, func() bool {
		for _, hwnd := range windowsOfProcess("msedge.exe") {
			if strings.Contains(windowText(hwnd), "XMind E2E Editor") {
				window = hwnd
				return true
			}
		}
		return false
	}) {
		e.screenshot("edge-not-found")
		e.logWindows("msedge.exe")
		t.Fatal("Edge window not found")
	}
	e.screenshot("edge-opened")

	var path string
	var err error
	waitUntil(45*time.Second, 2*time.Second, func() bool {
		path, err = documentPathFromAccessibility(window)
		if err != nil {
			t.Logf("accessibility: %v", err)
		}
		return err == nil
	})
	if err != nil || path != want {
		e.save("edge-diagnostics.txt", []byte(buildDiagnostics("e2e", core.Configuration{ProcessNames: []string{"msedge.exe"}}, "")))
		t.Fatalf("document path = %q, %v; want %q", path, err, want)
	}
	t.Logf("read %q from Edge's accessibility tree", path)
}

// TestE2EXMindAutoSave runs the real app against the real XMind: the document
// is detected, the toggle panel appears, and an edit is saved once typing
// stops — but not while it continues.
func TestE2EXMindAutoSave(t *testing.T) {
	xmindExe, appExe := os.Getenv("XMIND_EXE"), os.Getenv("XMIND_AUTOSAVE_EXE")
	if xmindExe == "" || appExe == "" {
		t.Skip("XMIND_EXE and XMIND_AUTOSAVE_EXE are required")
	}
	e := newE2ERun(t)
	xmindImage := filepath.Base(xmindExe)
	t.Logf("XMind: %s", xmindExe)

	documents := filepath.Join(e.artifacts, "documents")
	if err := os.MkdirAll(documents, 0o755); err != nil {
		t.Fatal(err)
	}
	stem := "自动保存测试"
	document := filepath.Join(documents, stem+".xmind")
	if err := writeXMindFixture(document, stem); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	_ = os.Chtimes(document, past, past)
	initialModTime := fileModTime(document)

	// 1. XMind opens the document.
	if err := exec.Command(xmindExe, document).Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killProcess(xmindImage) })

	var window uintptr
	started := time.Now()
	lastShot, lastOpen, lastDismiss := started, started, time.Time{}
	found := waitUntil(240*time.Second, 2*time.Second, func() bool {
		for _, hwnd := range windowsOfProcess(xmindImage) {
			if strings.Contains(windowText(hwnd), stem) {
				window = hwnd
				return true
			}
		}
		// The first launch shows "What's New" in a 680×546 window whose main
		// button ("Continue") sits near the bottom centre.
		if time.Since(lastDismiss) > 8*time.Second {
			for _, hwnd := range windowsOfProcess(xmindImage) {
				bounds := windowBounds(hwnd)
				if bounds.width() < 600 || bounds.width() > 780 || bounds.height() < 460 || bounds.height() > 640 {
					continue
				}
				lastDismiss = time.Now()
				call(procSetForegroundWindow, hwnd)
				time.Sleep(500 * time.Millisecond)
				e.screenshot("xmind-dialog")
				clickAt(bounds.Left+bounds.width()/2, bounds.Top+bounds.height()*872/1000)
				time.Sleep(2 * time.Second)
				e.screenshot("xmind-dialog-clicked")
				e.logWindows(xmindImage)
			}
		}
		// XMind may drop the file it was launched with while onboarding;
		// asking the running instance again opens it.
		if time.Since(lastOpen) > 25*time.Second {
			lastOpen = time.Now()
			t.Logf("asking XMind to open the document again")
			_ = exec.Command(xmindExe, document).Start()
		}
		if time.Since(lastShot) > 20*time.Second {
			lastShot = time.Now()
			e.screenshot("waiting-for-xmind")
			e.logWindows(xmindImage)
		}
		return false
	})
	if !found {
		e.screenshot("xmind-document-not-found")
		e.logWindows(xmindImage)
		t.Fatalf("no XMind window showing %q after %v", stem, time.Since(started).Round(time.Second))
	}
	time.Sleep(3 * time.Second) // let the editor settle
	call(procSetForegroundWindow, window)
	time.Sleep(time.Second)
	e.screenshot("xmind-opened")
	e.logWindows(xmindImage)
	t.Logf("document window: %q", windowText(window))

	// 2. The document path is readable the same way the macOS version reads it.
	var detected string
	var detectErr error
	waitUntil(60*time.Second, 2*time.Second, func() bool {
		detected, detectErr = documentPathFromAccessibility(window)
		return detectErr == nil
	})
	e.save("xmind-diagnostics.txt", []byte(buildDiagnostics("e2e", core.Configuration{ProcessNames: []string{xmindImage}}, "")))
	if detectErr != nil || !samePath(detected, document) {
		t.Errorf("accessibility path = %q, %v; want %q", detected, detectErr, document)
	} else {
		t.Logf("accessibility path: %s", detected)
	}

	// 3. The app starts with this document switched on.
	configuration := core.DefaultConfiguration()
	configuration.MonitoredFiles = []string{document}
	configuration.ProcessNames = []string{xmindImage}
	configData, _ := json.Marshal(configuration)
	configPath := filepath.Join(documents, "config.json")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dataDirectory(), "log.txt"))
	app := exec.Command(appExe, portableArgument)
	app.Env = append(os.Environ(), "XMIND_AUTOSAVE_CONFIG="+configPath)
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hwnd := findMainWindow(); hwnd != 0 {
			postMessage(hwnd, wmAppQuit, 0, 0)
			time.Sleep(2 * time.Second)
		}
		_ = app.Process.Kill()
		for _, name := range []string{"log.txt", "status.txt", "preferences.json"} {
			if data, err := os.ReadFile(filepath.Join(dataDirectory(), name)); err == nil {
				e.save("app-"+name, data)
			}
		}
	})
	if !waitUntil(15*time.Second, 200*time.Millisecond, func() bool { return findMainWindow() != 0 }) {
		t.Fatal("the app did not start")
	}
	call(procSetForegroundWindow, window)

	panel := call(procFindWindowW, uintptr(unsafe.Pointer(utf16Ptr(panelClassName))), 0)
	panelShown := waitUntil(20*time.Second, 250*time.Millisecond, func() bool {
		return panel != 0 && isWindowVisible(panel)
	})
	e.screenshot("app-started")
	if !panelShown {
		t.Errorf("toggle panel not shown next to XMind")
	} else {
		var panelRect rect
		call(procGetWindowRect, panel, uintptr(unsafe.Pointer(&panelRect)))
		xmindBounds := windowBounds(window)
		t.Logf("panel at (%d,%d) %dx%d; XMind at (%d,%d) %dx%d", panelRect.Left, panelRect.Top, panelRect.width(), panelRect.height(),
			xmindBounds.Left, xmindBounds.Top, xmindBounds.width(), xmindBounds.height())
	}
	if status, err := os.ReadFile(filepath.Join(dataDirectory(), "status.txt")); err == nil {
		t.Logf("status: %s", strings.TrimSpace(string(status)))
	}

	// 4. An edit is saved after typing stops.
	cleanTitle := windowText(window)
	pressKey(vkTab)
	time.Sleep(800 * time.Millisecond)
	typeText("E2E autosave")
	pressKey(vkReturn)
	edited := time.Now()
	time.Sleep(300 * time.Millisecond)
	dirtyTitle := windowText(window)
	e.screenshot("after-edit")
	t.Logf("title before edit %q, right after edit %q", cleanTitle, dirtyTitle)

	saved := waitUntil(20*time.Second, 200*time.Millisecond, func() bool {
		return fileModTime(document).After(initialModTime)
	})
	e.screenshot("after-save")
	t.Logf("title after save %q", windowText(window))
	if !saved {
		t.Fatalf("document not saved within 20 s of the edit")
	}
	savedAt := fileModTime(document)
	t.Logf("saved %v after the last keystroke", savedAt.Sub(edited).Round(10*time.Millisecond))
	if !strings.Contains(documentContent(document), "E2E autosave") {
		t.Errorf("saved document does not contain the new topic")
	}

	// 5. Continuous typing (pauses < 1.2 s) does not trigger a save.
	time.Sleep(3 * time.Second)
	pressKey(vkTab)
	time.Sleep(600 * time.Millisecond)
	typingStarted := time.Now()
	for _, letter := range "debounce" {
		typeText(string(letter))
		time.Sleep(400 * time.Millisecond)
	}
	pressKey(vkReturn)
	typingEnded := time.Now()
	beforeDebounce := fileModTime(document)
	resaved := waitUntil(15*time.Second, 200*time.Millisecond, func() bool {
		return fileModTime(document).After(beforeDebounce)
	})
	e.screenshot("after-debounce")

	commands := saveCommands()
	t.Logf("Ctrl+S sent at: %v", commands)
	if during := countBetween(commands, typingStarted, typingEnded); during != 0 {
		t.Errorf("%d save(s) sent while typing continued", during)
	}
	if !resaved || countBetween(commands, typingEnded, time.Now()) == 0 {
		t.Errorf("no save after typing stopped")
	}
	if !strings.Contains(documentContent(document), "debounce") {
		t.Errorf("saved document does not contain the typed text")
	}
}
