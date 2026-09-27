package winapp

import (
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

func foregroundWindow() uintptr {
	return call(procGetForegroundWindow)
}

func rootWindow(hwnd uintptr) uintptr {
	if hwnd == 0 {
		return 0
	}
	return call(procGetAncestor, hwnd, gaRoot)
}

func windowProcess(hwnd uintptr) (processID, threadID uint32) {
	threadID = uint32(call(procGetWindowThreadProcessId, hwnd, uintptr(unsafe.Pointer(&processID))))
	return processID, threadID
}

func windowText(hwnd uintptr) string {
	length := int(int32(call(procGetWindowTextLengthW, hwnd)))
	if length <= 0 {
		return ""
	}
	buffer := make([]uint16, length+1)
	copied := int(int32(call(procGetWindowTextW, hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))))
	return string(utf16.Decode(buffer[:max(0, min(copied, length))]))
}

func windowClass(hwnd uintptr) string {
	var buffer [256]uint16
	copied := int(int32(call(procGetClassNameW, hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))))
	return string(utf16.Decode(buffer[:max(0, min(copied, len(buffer)))]))
}

func isWindow(hwnd uintptr) bool        { return hwnd != 0 && call(procIsWindow, hwnd) != 0 }
func isWindowVisible(hwnd uintptr) bool { return call(procIsWindowVisible, hwnd) != 0 }
func isMinimized(hwnd uintptr) bool     { return call(procIsIconic, hwnd) != 0 }
func isMaximized(hwnd uintptr) bool     { return call(procIsZoomed, hwnd) != 0 }

// windowBounds returns the visible frame of a window. GetWindowRect includes
// the invisible resize borders of Windows 10/11, DWM's frame bounds do not.
func windowBounds(hwnd uintptr) rect {
	const dwmwaExtendedFrameBounds = 9
	var bounds rect
	if available(procDwmGetWindowAttribute) &&
		call(procDwmGetWindowAttribute, hwnd, dwmwaExtendedFrameBounds, uintptr(unsafe.Pointer(&bounds)), unsafe.Sizeof(bounds)) == 0 &&
		!bounds.empty() {
		return bounds
	}
	call(procGetWindowRect, hwnd, uintptr(unsafe.Pointer(&bounds)))
	return bounds
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

func monitorBounds(hwnd uintptr) (rect, bool) {
	const monitorDefaultToNearest = 2
	monitor := call(procMonitorFromWindow, hwnd, monitorDefaultToNearest)
	info := monitorInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	if monitor == 0 || call(procGetMonitorInfoW, monitor, uintptr(unsafe.Pointer(&info))) == 0 {
		return rect{}, false
	}
	return info.Monitor, true
}

// isFullScreen treats a window covering its whole monitor as full screen,
// which is how XMind's full-screen and presentation modes look.
func isFullScreen(hwnd uintptr, bounds rect) bool {
	if isMaximized(hwnd) {
		return false
	}
	monitor, ok := monitorBounds(hwnd)
	return ok && bounds.Left <= monitor.Left && bounds.Top <= monitor.Top &&
		bounds.Right >= monitor.Right && bounds.Bottom >= monitor.Bottom
}

func processImagePath(processID uint32) string {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, processID)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(handle)

	buffer := make([]uint16, 1024)
	size := uint32(len(buffer))
	if call(procQueryFullProcessImageNameW, uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size))) == 0 {
		return ""
	}
	return string(utf16.Decode(buffer[:size]))
}

// processNames caches the executable name of processes by ID, so input
// events can be attributed to XMind without opening the process each time.
type processNames struct {
	mu      sync.Mutex
	entries map[uint32]processNameEntry
}

type processNameEntry struct {
	name      string
	checkedAt time.Time
}

func newProcessNames() *processNames {
	return &processNames{entries: map[uint32]processNameEntry{}}
}

func (p *processNames) name(processID uint32) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if entry, ok := p.entries[processID]; ok && now.Sub(entry.checkedAt) < 10*time.Second {
		return entry.name
	}
	if len(p.entries) > 512 {
		clear(p.entries)
	}
	name := filepath.Base(processImagePath(processID))
	p.entries[processID] = processNameEntry{name: name, checkedAt: now}
	return name
}

func matchesProcessName(name string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

// Window enumeration. EnumWindows needs a C callback; the callback appends to
// the list registered under the lParam token so concurrent enumerations on
// different threads stay separate.
var (
	enumerations      sync.Map // token → *[]uintptr
	enumerationTokens uintptr
	enumerationMutex  sync.Mutex
	enumerateCallback = syscall.NewCallback(func(hwnd, token uintptr) uintptr {
		if list, ok := enumerations.Load(token); ok {
			windows := list.(*[]uintptr)
			*windows = append(*windows, hwnd)
		}
		return 1
	})
)

func enumerateWindows(parent uintptr) []uintptr {
	enumerationMutex.Lock()
	enumerationTokens++
	token := enumerationTokens
	enumerationMutex.Unlock()

	var windows []uintptr
	enumerations.Store(token, &windows)
	defer enumerations.Delete(token)
	if parent == 0 {
		call(procEnumWindows, enumerateCallback, token)
	} else {
		call(procEnumChildWindows, parent, enumerateCallback, token)
	}
	return windows
}
