package winapp

import (
	"unicode/utf16"
	"unsafe"
)

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        guid
	BalloonIcon     uintptr
}

const (
	nimAdd      = 0
	nimModify   = 1
	nimDelete   = 2
	nifMessage  = 0x01
	nifIcon     = 0x02
	nifTip      = 0x04
	nifInfo     = 0x10
	niifInfo    = 0x01
	niifNoSound = 0x10
)

// trayIcon is the notification-area icon, the Windows counterpart of the
// macOS menu bar item.
type trayIcon struct {
	hwnd            uintptr
	callbackMessage uint32
	icon            uintptr
	tooltip         string
	added           bool
}

func newTrayIcon(hwnd uintptr, callbackMessage uint32) *trayIcon {
	return &trayIcon{hwnd: hwnd, callbackMessage: callbackMessage, icon: loadAppIcon()}
}

// loadAppIcon loads the "APP" icon embedded by go-winres at the small-icon
// size, falling back to the stock application icon in resource-less builds.
func loadAppIcon() uintptr {
	width := call(procGetSystemMetrics, smCxSmIcon)
	height := call(procGetSystemMetrics, smCySmIcon)
	if icon := call(procLoadImageW, moduleHandle(), uintptr(unsafe.Pointer(utf16Ptr("APP"))), imageIcon, width, height, lrDefaultColor); icon != 0 {
		return icon
	}
	return call(procLoadIconW, 0, idiApplication)
}

func (t *trayIcon) data(flags uint32) *notifyIconData {
	data := &notifyIconData{
		Wnd:             t.hwnd,
		ID:              1,
		Flags:           flags,
		CallbackMessage: t.callbackMessage,
		Icon:            t.icon,
	}
	data.Size = uint32(unsafe.Sizeof(*data))
	copyUTF16(data.Tip[:], t.tooltip)
	return data
}

// add (re-)creates the icon; it is called again when Explorer restarts.
func (t *trayIcon) add() bool {
	t.added = call(procShellNotifyIconW, nimAdd, uintptr(unsafe.Pointer(t.data(nifMessage|nifIcon|nifTip)))) != 0
	return t.added
}

func (t *trayIcon) setTooltip(tooltip string) {
	if tooltip == t.tooltip {
		return
	}
	t.tooltip = tooltip
	if t.added {
		call(procShellNotifyIconW, nimModify, uintptr(unsafe.Pointer(t.data(nifTip))))
	}
}

func (t *trayIcon) notify(title, text string) {
	data := t.data(nifInfo)
	copyUTF16(data.InfoTitle[:], title)
	copyUTF16(data.Info[:], text)
	data.InfoFlags = niifInfo | niifNoSound
	call(procShellNotifyIconW, nimModify, uintptr(unsafe.Pointer(data)))
}

func (t *trayIcon) remove() {
	if t.added {
		call(procShellNotifyIconW, nimDelete, uintptr(unsafe.Pointer(t.data(0))))
		t.added = false
	}
}

// copyUTF16 copies s into a fixed buffer, truncating and NUL-terminating it.
func copyUTF16(buffer []uint16, s string) {
	encoded := utf16.Encode([]rune(s))
	if len(encoded) > len(buffer)-1 {
		encoded = encoded[:len(buffer)-1]
		// Do not leave half of a surrogate pair at the end.
		if last := encoded[len(encoded)-1]; last >= 0xD800 && last < 0xDC00 {
			encoded = encoded[:len(encoded)-1]
		}
	}
	copy(buffer, encoded)
	buffer[len(encoded)] = 0
}

// Popup menu.
const (
	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfSeparator = 0x0800

	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
)

type menuItem struct {
	id        uint32
	title     string
	disabled  bool
	checked   bool
	separator bool
}

// showPopupMenu shows the menu at the cursor and returns the chosen ID, or 0.
func showPopupMenu(owner uintptr, items []menuItem) uint32 {
	menu := call(procCreatePopupMenu)
	if menu == 0 {
		return 0
	}
	defer call(procDestroyMenu, menu)
	for _, item := range items {
		if item.separator {
			call(procAppendMenuW, menu, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if item.disabled {
			flags |= mfGrayed
		}
		if item.checked {
			flags |= mfChecked
		}
		call(procAppendMenuW, menu, flags, uintptr(item.id), uintptr(unsafe.Pointer(utf16Ptr(item.title))))
	}

	var cursor point
	call(procGetCursorPos, uintptr(unsafe.Pointer(&cursor)))
	// Required so the menu closes when the user clicks elsewhere.
	call(procSetForegroundWindow, owner)
	command := call(procTrackPopupMenu, menu, tpmRightButton|tpmNoNotify|tpmReturnCmd,
		uintptr(cursor.X), uintptr(cursor.Y), 0, owner, 0)
	postMessage(owner, wmNull, 0, 0)
	return uint32(command)
}
