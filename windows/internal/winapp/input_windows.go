package winapp

import (
	"encoding/binary"
	"unsafe"
)

// Keyboard and mouse activity is observed through raw input rather than
// low-level hooks: raw input is delivered asynchronously, so a busy moment in
// this app can never slow down typing elsewhere.

const (
	ridevInputSink   = 0x00000100
	ridInput         = 0x10000003
	rimTypeMouse     = 0
	rimTypeKeyboard  = 1
	riKeyBreak       = 0x0001
	riMouseButtonsUp = 0x0002 | 0x0008 | 0x0020 // left, right, middle
	rawHeaderSize    = 24                       // sizeof(RAWINPUTHEADER) on 64-bit Windows

	// injectedInputMarker tags the Ctrl+S events this app sends so they are
	// not mistaken for the user editing.
	injectedInputMarker = 0x584D4153 // "XMAS"
)

// Virtual-key codes.
const (
	vkLButton = 0x01
	vkRButton = 0x02
	vkMButton = 0x04
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
	vkCapital = 0x14
	vkLWin    = 0x5B
	vkRWin    = 0x5C
	vkNumLock = 0x90
	vkScroll  = 0x91
	vkLShift  = 0xA0
	vkRMenu   = 0xA5
	vkS       = 0x53
	vkA       = 0x41
	vkZ       = 0x5A
)

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

func registerRawInput(hwnd uintptr) error {
	devices := [2]rawInputDevice{
		{UsagePage: 0x01, Usage: 0x06, Flags: ridevInputSink, Target: hwnd}, // keyboard
		{UsagePage: 0x01, Usage: 0x02, Flags: ridevInputSink, Target: hwnd}, // mouse
	}
	if ok, err := callErr(procRegisterRawInputDevices,
		uintptr(unsafe.Pointer(&devices[0])), uintptr(len(devices)), unsafe.Sizeof(devices[0])); ok == 0 {
		return err
	}
	return nil
}

type rawInputEvent struct {
	keyDown      bool
	virtualKey   uint16
	mouseRelease bool
}

// readRawInput decodes a WM_INPUT message. Only key presses and mouse button
// releases are reported; mouse movement is dropped.
func readRawInput(handle uintptr) (rawInputEvent, bool) {
	var storage [8]uint64 // 64 bytes, 8-byte aligned
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), len(storage)*8)
	size := uint32(len(buffer))
	copied := uint32(call(procGetRawInputData, handle, ridInput,
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), rawHeaderSize))
	if copied == 0 || copied == ^uint32(0) || copied < rawHeaderSize+16 {
		return rawInputEvent{}, false
	}

	data := buffer[rawHeaderSize:copied]
	switch binary.LittleEndian.Uint32(buffer[0:4]) {
	case rimTypeKeyboard:
		// RAWKEYBOARD: MakeCode, Flags, Reserved, VKey (uint16); Message, ExtraInformation (uint32).
		flags := binary.LittleEndian.Uint16(data[2:4])
		virtualKey := binary.LittleEndian.Uint16(data[6:8])
		extra := binary.LittleEndian.Uint32(data[12:16])
		if extra == injectedInputMarker || flags&riKeyBreak != 0 || isModifierKey(virtualKey) {
			return rawInputEvent{}, false
		}
		return rawInputEvent{keyDown: true, virtualKey: virtualKey}, true
	case rimTypeMouse:
		if len(data) < 24 {
			return rawInputEvent{}, false
		}
		// RAWMOUSE: usFlags, padding, usButtonFlags, usButtonData, …, ulExtraInformation.
		buttonFlags := binary.LittleEndian.Uint16(data[4:6])
		if buttonFlags&riMouseButtonsUp == 0 || binary.LittleEndian.Uint32(data[20:24]) == injectedInputMarker {
			return rawInputEvent{}, false
		}
		return rawInputEvent{mouseRelease: true}, true
	}
	return rawInputEvent{}, false
}

// isModifierKey is true for keys that never edit anything on their own.
// Keys without a usable code (0, 0xFF, VK_PACKET) still count as typing:
// on-screen keyboards, dictation and other text-injecting tools produce them.
func isModifierKey(virtualKey uint16) bool {
	switch virtualKey {
	case vkShift, vkControl, vkMenu, vkLWin, vkRWin, vkCapital, vkNumLock, vkScroll:
		return true
	}
	return virtualKey >= vkLShift && virtualKey <= vkRMenu
}

func isLetterKey(virtualKey uint16) bool {
	return virtualKey >= vkA && virtualKey <= vkZ
}

func keyIsDown(virtualKey int) bool {
	return uint16(call(procGetAsyncKeyState, uintptr(virtualKey)))&0x8000 != 0
}

// inputHeld reports whether a modifier or mouse button is down. Sending
// Ctrl+S then could turn into Ctrl+Shift+S (Save As) or disturb a drag.
func inputHeld() bool {
	for _, key := range []int{vkShift, vkControl, vkMenu, vkLWin, vkRWin, vkLButton, vkRButton, vkMButton} {
		if keyIsDown(key) {
			return true
		}
	}
	return false
}

func controlOrAltDown() bool {
	return keyIsDown(vkControl) || keyIsDown(vkMenu)
}

type keyboardInput struct {
	Type       uint32
	_          uint32
	VirtualKey uint16
	ScanCode   uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
	_          [8]byte // pad to sizeof(INPUT), whose union is sized by MOUSEINPUT
}

// sendControlS types Ctrl+S into the foreground window.
func sendControlS() bool {
	const (
		inputKeyboard           = 1
		keyEventKeyUp           = 0x0002
		mapVirtualKeyToScanCode = 0
	)
	key := func(virtualKey uint16, up bool) keyboardInput {
		input := keyboardInput{
			Type:       inputKeyboard,
			VirtualKey: virtualKey,
			ScanCode:   uint16(call(procMapVirtualKeyW, uintptr(virtualKey), mapVirtualKeyToScanCode)),
			ExtraInfo:  injectedInputMarker,
		}
		if up {
			input.Flags = keyEventKeyUp
		}
		return input
	}
	inputs := [4]keyboardInput{
		key(vkControl, false),
		key(vkS, false),
		key(vkS, true),
		key(vkControl, true),
	}
	sent := call(procSendInput, uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return int(uint32(sent)) == len(inputs)
}

type guiThreadInfo struct {
	Size      uint32
	Flags     uint32
	Active    uintptr
	Focus     uintptr
	Capture   uintptr
	MenuOwner uintptr
	MoveSize  uintptr
	Caret     uintptr
	CaretRect rect
}

const (
	guiInMoveSize     = 0x0002
	guiInMenuMode     = 0x0004
	guiSystemMenuMode = 0x0008
	guiPopupMenuMode  = 0x0010
)

func threadGUIInfo(threadID uint32) (guiThreadInfo, bool) {
	info := guiThreadInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	ok := call(procGetGUIThreadInfo, uintptr(threadID), uintptr(unsafe.Pointer(&info))) != 0
	return info, ok
}

// imeInNativeMode reports whether the input method of the window is turned
// on in native (e.g. Chinese) mode, where letters start a composition.
func imeInNativeMode(hwnd uintptr) (native bool, known bool) {
	const (
		wmIMEControl         = 0x0283
		imcGetConversionMode = 0x0001
		imcGetOpenStatus     = 0x0005
		imeConversionNative  = 0x0001
	)
	ime := call(procImmGetDefaultIMEWnd, hwnd)
	if ime == 0 {
		return false, false
	}
	query := func(command uintptr) (uintptr, bool) {
		var result uintptr
		ok := call(procSendMessageTimeoutW, ime, wmIMEControl, command, 0,
			smtoAbortIfHung|smtoBlock, 100, uintptr(unsafe.Pointer(&result))) != 0
		return result, ok
	}
	open, ok := query(imcGetOpenStatus)
	if !ok {
		return false, false
	}
	conversion, ok := query(imcGetConversionMode)
	if !ok {
		return false, false
	}
	return open != 0 && conversion&imeConversionNative != 0, true
}
