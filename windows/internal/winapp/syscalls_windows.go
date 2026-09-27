package winapp

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// The first run happens from the Downloads folder, a classic place for DLL
// planting, so restrict DLL lookups to System32 before loading anything.
func init() {
	const loadLibrarySearchSystem32 = 0x00000800
	setDefaultDllDirectories := syscall.NewLazyDLL("kernel32.dll").NewProc("SetDefaultDllDirectories")
	if setDefaultDllDirectories.Find() == nil {
		setDefaultDllDirectories.Call(loadLibrarySearchSystem32)
	}
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	oleaut32 = syscall.NewLazyDLL("oleaut32.dll")
	oleacc   = syscall.NewLazyDLL("oleacc.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	imm32    = syscall.NewLazyDLL("imm32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	procAppendMenuW                   = user32.NewProc("AppendMenuW")
	procCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procDestroyMenu                   = user32.NewProc("DestroyMenu")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procEnumChildWindows              = user32.NewProc("EnumChildWindows")
	procEnumWindows                   = user32.NewProc("EnumWindows")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procGetAncestor                   = user32.NewProc("GetAncestor")
	procGetAsyncKeyState              = user32.NewProc("GetAsyncKeyState")
	procGetClassNameW                 = user32.NewProc("GetClassNameW")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procGetDC                         = user32.NewProc("GetDC")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	procGetGUIThreadInfo              = user32.NewProc("GetGUIThreadInfo")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	procGetRawInputData               = user32.NewProc("GetRawInputData")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procGetWindowRect                 = user32.NewProc("GetWindowRect")
	procGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW                = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	procIsIconic                      = user32.NewProc("IsIconic")
	procIsWindow                      = user32.NewProc("IsWindow")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsZoomed                      = user32.NewProc("IsZoomed")
	procKillTimer                     = user32.NewProc("KillTimer")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procLoadIconW                     = user32.NewProc("LoadIconW")
	procLoadImageW                    = user32.NewProc("LoadImageW")
	procMapVirtualKeyW                = user32.NewProc("MapVirtualKeyW")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	procPostMessageW                  = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procRegisterRawInputDevices       = user32.NewProc("RegisterRawInputDevices")
	procRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	procReleaseDC                     = user32.NewProc("ReleaseDC")
	procSendInput                     = user32.NewProc("SendInput")
	procSendMessageTimeoutW           = user32.NewProc("SendMessageTimeoutW")
	procSetCursor                     = user32.NewProc("SetCursor")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetTimer                      = user32.NewProc("SetTimer")
	procSetWinEventHook               = user32.NewProc("SetWinEventHook")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procUnhookWinEvent                = user32.NewProc("UnhookWinEvent")
	procUpdateLayeredWindow           = user32.NewProc("UpdateLayeredWindow")
	procWindowFromPoint               = user32.NewProc("WindowFromPoint")

	procCreateMutexW               = kernel32.NewProc("CreateMutexW")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")

	procSysFreeString = oleaut32.NewProc("SysFreeString")
	procSysStringLen  = oleaut32.NewProc("SysStringLen")
	procVariantClear  = oleaut32.NewProc("VariantClear")

	procAccessibleChildren         = oleacc.NewProc("AccessibleChildren")
	procAccessibleObjectFromWindow = oleacc.NewProc("AccessibleObjectFromWindow")

	procRegCreateKeyExW = advapi32.NewProc("RegCreateKeyExW")
	procRegDeleteKeyW   = advapi32.NewProc("RegDeleteKeyW")
	procRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	procRegSetValueExW  = advapi32.NewProc("RegSetValueExW")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
	procSelectObject       = gdi32.NewProc("SelectObject")

	procImmGetDefaultIMEWnd = imm32.NewProc("ImmGetDefaultIMEWnd")

	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")

	procRtlGetVersion = ntdll.NewProc("RtlGetVersion")
)

// call invokes a Win32 function and returns its primary result.
//
//go:uintptrescapes
func call(proc *syscall.LazyProc, args ...uintptr) uintptr {
	result, _, _ := proc.Call(args...)
	return result
}

// callErr is like call but also returns GetLastError.
//
//go:uintptrescapes
func callErr(proc *syscall.LazyProc, args ...uintptr) (uintptr, error) {
	result, _, err := proc.Call(args...)
	return result, err
}

func available(proc *syscall.LazyProc) bool {
	return proc.Find() == nil
}

func utf16Ptr(s string) *uint16 {
	pointer, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		// Strings with embedded NULs never come from the user; fall back to
		// the part before the NUL rather than failing the call.
		pointer, _ = syscall.UTF16PtrFromString(s[:indexNUL(s)])
	}
	return pointer
}

func indexNUL(s string) int {
	for index := range len(s) {
		if s[index] == 0 {
			return index
		}
	}
	return len(s)
}

// utf16PtrToString reads a NUL-terminated UTF-16 string owned by Windows.
func utf16PtrToString(pointer *uint16) string {
	if pointer == nil {
		return ""
	}
	length := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(pointer), length*2)) != 0 {
		length++
	}
	return string(utf16.Decode(unsafe.Slice(pointer, length)))
}

func boolToUintptr(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}

func loword(value uintptr) uint16 { return uint16(value & 0xFFFF) }

type point struct {
	X, Y int32
}

type rect struct {
	Left, Top, Right, Bottom int32
}

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }
func (r rect) empty() bool   { return r.width() <= 0 || r.height() <= 0 }

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// Window messages and styles.
const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmSettingChange   = 0x001A
	wmSetCursor       = 0x0020
	wmMouseActivate   = 0x0021
	wmNCHitTest       = 0x0084
	wmInput           = 0x00FF
	wmTimer           = 0x0113
	wmLButtonDown     = 0x0201
	wmLButtonUp       = 0x0202
	wmRButtonUp       = 0x0205
	wmDpiChanged      = 0x02E0
	wmNull            = 0x0000
	wmApp             = 0x8000

	wsPopup           = 0x80000000
	wsExTopmost       = 0x00000008
	wsExToolWindow    = 0x00000080
	wsExLayered       = 0x00080000
	wsExNoActivate    = 0x08000000
	swHide            = 0
	swShowNoActivate  = 4
	swpNoSize         = 0x0001
	swpNoMove         = 0x0002
	swpNoActivate     = 0x0010
	swpShowWindow     = 0x0040
	hwndTopmost       = ^uintptr(0) // (HWND)-1
	maNoActivate      = 3
	htClient          = 1
	gaRoot            = 2
	idcHand           = 32649
	idcArrow          = 32512
	idiApplication    = 32512
	imageIcon         = 1
	lrDefaultColor    = 0
	smCxSmIcon        = 49
	smCySmIcon        = 50
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconInformation = 0x00000040
	mbIconWarning     = 0x00000030
	mbIconQuestion    = 0x00000020
	mbIconError       = 0x00000010
	mbSetForeground   = 0x00010000
	mbTopmost         = 0x00040000
	idYes             = 6
	smtoAbortIfHung   = 0x0002
	smtoBlock         = 0x0001
	logPixelsX        = 88
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type message struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

func moduleHandle() uintptr {
	return call(procGetModuleHandleW, 0)
}

func registerWindowClass(name string, wndProc uintptr, cursor uintptr) error {
	class := wndClassEx{
		WndProc:   wndProc,
		Instance:  moduleHandle(),
		Cursor:    cursor,
		ClassName: utf16Ptr(name),
	}
	class.Size = uint32(unsafe.Sizeof(class))
	const errorClassAlreadyExists = syscall.Errno(1410)
	if atom, err := callErr(procRegisterClassExW, uintptr(unsafe.Pointer(&class))); atom == 0 && err != errorClassAlreadyExists {
		return err
	}
	return nil
}

func createWindow(exStyle uint32, className, title string, style uint32) (uintptr, error) {
	hwnd, err := callErr(procCreateWindowExW,
		uintptr(exStyle),
		uintptr(unsafe.Pointer(utf16Ptr(className))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		uintptr(style),
		0, 0, 0, 0,
		0, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		return 0, err
	}
	return hwnd, nil
}

func defWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	return call(procDefWindowProcW, hwnd, uintptr(msg), wParam, lParam)
}

func postMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	return call(procPostMessageW, hwnd, uintptr(msg), wParam, lParam) != 0
}

func messageBox(owner uintptr, text, caption string, flags uint32) int {
	return int(int32(call(procMessageBoxW, owner,
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(unsafe.Pointer(utf16Ptr(caption))),
		uintptr(flags|mbSetForeground),
	)))
}

func runMessageLoop() {
	var msg message
	for {
		result := int32(call(procGetMessageW, uintptr(unsafe.Pointer(&msg)), 0, 0, 0))
		if result == 0 || result == -1 {
			return
		}
		call(procTranslateMessage, uintptr(unsafe.Pointer(&msg)))
		call(procDispatchMessageW, uintptr(unsafe.Pointer(&msg)))
	}
}

// enableHighDPI opts into per-monitor DPI awareness so window coordinates
// read from XMind and used for the toggle panel are real pixels. The
// manifest normally does this already; the call covers builds without it.
func enableHighDPI() {
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	if available(procSetProcessDpiAwarenessContext) && call(procSetProcessDpiAwarenessContext, perMonitorAwareV2) != 0 {
		return
	}
	call(procSetProcessDPIAware)
}

func dpiForWindow(hwnd uintptr) uint32 {
	if available(procGetDpiForWindow) {
		if dpi := uint32(call(procGetDpiForWindow, hwnd)); dpi != 0 {
			return dpi
		}
	}
	screen := call(procGetDC, 0)
	defer call(procReleaseDC, 0, screen)
	if dpi := uint32(call(procGetDeviceCaps, screen, logPixelsX)); dpi != 0 {
		return dpi
	}
	return 96
}
