package winapp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// These tests exercise the Win32 bindings on a real Windows machine (the
// GitHub Actions runner); they need no desktop interaction.

func TestWin32StructLayouts(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassEx{}), 80},
		{"MSG", unsafe.Sizeof(message{}), 48},
		{"NOTIFYICONDATAW", unsafe.Sizeof(notifyIconData{}), 976},
		{"INPUT", unsafe.Sizeof(keyboardInput{}), 40},
		{"RAWINPUTDEVICE", unsafe.Sizeof(rawInputDevice{}), 16},
		{"GUITHREADINFO", unsafe.Sizeof(guiThreadInfo{}), 72},
		{"MONITORINFO", unsafe.Sizeof(monitorInfo{}), 40},
		{"VARIANT", unsafe.Sizeof(variant{}), 24},
		{"BITMAPINFOHEADER", unsafe.Sizeof(bitmapInfoHeader{}), 40},
		{"GdiplusStartupInput", unsafe.Sizeof(gdiplusStartupInput{}), 24},
		{"OSVERSIONINFOW", unsafe.Sizeof(osVersionInfo{}), 276},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("sizeof(%s) = %d; want %d", c.name, c.got, c.want)
		}
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	const path = `Software\XMindAutoSaveTest`
	t.Cleanup(func() { _ = deleteRegistryKey(path) })

	if err := setRegistryString(path, "Text", `"C:\某个 路径\a.exe" --autostart`); err != nil {
		t.Fatal(err)
	}
	if value, ok := registryString(path, "Text"); !ok || value != `"C:\某个 路径\a.exe" --autostart` {
		t.Fatalf("registryString = %q, %v", value, ok)
	}
	if err := setRegistryDWORD(path, "Number", 0xA1B2C3D4); err != nil {
		t.Fatal(err)
	}
	if value, ok := registryDWORD(path, "Number"); !ok || value != 0xA1B2C3D4 {
		t.Fatalf("registryDWORD = %#x, %v", value, ok)
	}
	if err := deleteRegistryValue(path, "Text"); err != nil {
		t.Fatal(err)
	}
	if _, ok := registryString(path, "Text"); ok {
		t.Fatal("value still present after delete")
	}
	if err := deleteRegistryValue(path, "Missing"); err != nil {
		t.Fatalf("deleting a missing value: %v", err)
	}
}

func TestFileIdentitySurvivesRename(t *testing.T) {
	directory := t.TempDir()
	original := filepath.Join(directory, "计划.xmind")
	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := fileIdentityKey(original)
	if !strings.HasPrefix(before, "file:") {
		t.Fatalf("expected a file identity, got %q", before)
	}

	moved := filepath.Join(directory, "sub", "改名后.xmind")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if after := fileIdentityKey(moved); after != before {
		t.Fatalf("identity changed after rename: %q → %q", before, after)
	}
	if missing := fileIdentityKey(original); !strings.HasPrefix(missing, "path:") {
		t.Fatalf("missing file should fall back to its path, got %q", missing)
	}
}

func TestShortcutRoundTrip(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := initializeCOM(); err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	target := filepath.Join(directory, "文档 1.xmind")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	shortcut := filepath.Join(directory, "文档 1.xmind.lnk")
	if err := createShortcut(shortcut, target, "", "测试"); err != nil {
		t.Fatal(err)
	}
	resolved, err := shortcutTarget(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(resolved, target) {
		t.Fatalf("shortcut target = %q; want %q", resolved, target)
	}
}

func TestTogglePanelRenders(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	panel, err := newTogglePanel(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer call(procDestroyWindow, panel.hwnd)

	for _, state := range []panelState{
		{enabled: true, dark: false, dpi: 96},
		{enabled: false, dark: true, dpi: 144},
		{enabled: true, dark: true, dpi: 192},
	} {
		if err := panel.render(state); err != nil {
			t.Fatalf("render(%+v): %v", state, err)
		}
		wantWidth := int32((panelWidth + 2*panelMargin) * state.dpi / 96)
		if panel.size.X < wantWidth-1 || panel.size.X > wantWidth+1 {
			t.Fatalf("width at %d DPI = %d; want about %d", state.dpi, panel.size.X, wantWidth)
		}
	}
}

func TestRecentDocumentsIgnoresOtherFiles(t *testing.T) {
	// Only checks that reading the real Recent folder works on this machine.
	for _, document := range recentDocuments() {
		if document.name == "" || !strings.HasSuffix(strings.ToLower(document.shortcut), ".xmind.lnk") {
			t.Fatalf("unexpected entry %+v", document)
		}
	}
}
