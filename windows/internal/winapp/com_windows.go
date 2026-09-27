package winapp

import (
	"errors"
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Minimal COM plumbing: an interface pointer is a pointer to a pointer to a
// table of function pointers, and every method takes the interface pointer as
// its first argument.
type comObject struct {
	vtable unsafe.Pointer
}

//go:uintptrescapes
func (o *comObject) call(method int, args ...uintptr) uintptr {
	function := *(*uintptr)(unsafe.Add(o.vtable, uintptr(method)*unsafe.Sizeof(uintptr(0))))
	arguments := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	result, _, _ := syscall.SyscallN(function, arguments...)
	return result
}

func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}

func (o *comObject) queryInterface(iid *guid) (*comObject, error) {
	var result *comObject
	if hr := o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&result))); failed(hr) || result == nil {
		return nil, hresultError(hr)
	}
	return result, nil
}

func failed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func hresultError(hr uintptr) error {
	return fmt.Errorf("HRESULT 0x%08X", uint32(hr))
}

// initializeCOM joins the calling OS thread to a single-threaded apartment.
func initializeCOM() error {
	const (
		coinitApartmentThreaded = 0x2
		coinitDisableOLE1DDE    = 0x4
	)
	hr := call(procCoInitializeEx, 0, coinitApartmentThreaded|coinitDisableOLE1DDE)
	if failed(hr) {
		return hresultError(hr)
	}
	return nil
}

func createInstance(clsid, iid *guid) (*comObject, error) {
	const clsctxInprocServer = 0x1
	var result *comObject
	hr := call(procCoCreateInstance,
		uintptr(unsafe.Pointer(clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&result)))
	if failed(hr) || result == nil {
		return nil, hresultError(hr)
	}
	return result, nil
}

// variant mirrors the 24-byte VARIANT of 64-bit Windows.
type variant struct {
	VT       uint16
	reserved [3]uint16
	Value    uintptr
	Value2   uintptr
}

const (
	vtI4       = 3
	vtBSTR     = 8
	vtDispatch = 9
)

func (v *variant) clear() {
	call(procVariantClear, uintptr(unsafe.Pointer(v)))
}

func (v *variant) object() *comObject {
	return *(**comObject)(unsafe.Pointer(&v.Value))
}

// takeBSTR converts a BSTR returned by a COM call and frees it.
func takeBSTR(bstr *uint16) string {
	if bstr == nil {
		return ""
	}
	defer call(procSysFreeString, uintptr(unsafe.Pointer(bstr)))
	length := int(uint32(call(procSysStringLen, uintptr(unsafe.Pointer(bstr)))))
	return string(utf16.Decode(unsafe.Slice(bstr, length)))
}

// IAccessible (MSAA). Chromium, and therefore Electron apps such as XMind,
// answers get_accValue on a document with the document's URL.
var iidIAccessible = guid{0x618736E0, 0x3C3D, 0x11CF, [8]byte{0x81, 0x0C, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}

const (
	accChildCount = 8
	accName       = 10
	accValue      = 11
	accRole       = 13

	objidClient      = 0xFFFFFFFC
	roleSystemClient = 0x0A
	roleDocument     = 0x0F
)

var (
	iidServiceProvider = guid{0x6D5140C1, 0x7436, 0x11CE, [8]byte{0x80, 0x34, 0x00, 0xAA, 0x00, 0x60, 0x09, 0xFA}}
	iidIAccessible2    = guid{0xE89F726E, 0xC4F4, 0x4C19, [8]byte{0xBB, 0x19, 0xB6, 0x47, 0xD7, 0xFA, 0x84, 0x78}}
)

// requestWebAccessibility asks for the IAccessible2 interface, the way screen
// readers announce themselves. Chromium — and so Electron apps like XMind —
// only builds the accessibility tree of web content, document URL included,
// after that; plain MSAA calls see an empty document. It is the Windows
// counterpart of setting AXManualAccessibility in the macOS version.
func (o *comObject) requestWebAccessibility() {
	o.accessibleString(accName) // some Chromium versions switch on basic support here
	provider, err := o.queryInterface(&iidServiceProvider)
	if err != nil {
		return
	}
	defer provider.release()
	const serviceProviderQueryService = 3
	var accessible2 *comObject
	hr := provider.call(serviceProviderQueryService,
		uintptr(unsafe.Pointer(&iidIAccessible2)), uintptr(unsafe.Pointer(&iidIAccessible2)),
		uintptr(unsafe.Pointer(&accessible2)))
	if !failed(hr) && accessible2 != nil {
		accessible2.release()
	}
}

func accessibleFromWindow(hwnd uintptr) (*comObject, error) {
	var result *comObject
	hr := call(procAccessibleObjectFromWindow, hwnd, objidClient,
		uintptr(unsafe.Pointer(&iidIAccessible)), uintptr(unsafe.Pointer(&result)))
	if failed(hr) || result == nil {
		return nil, hresultError(hr)
	}
	return result, nil
}

// VARIANT arguments are passed by value in C; on x64 that means a pointer to
// a caller-owned copy, so each call gets a fresh CHILDID_SELF.
func childSelf() *variant {
	return &variant{VT: vtI4}
}

func (o *comObject) accessibleRole() int32 {
	var role variant
	hr := o.call(accRole, uintptr(unsafe.Pointer(childSelf())), uintptr(unsafe.Pointer(&role)))
	defer role.clear()
	if failed(hr) || role.VT != vtI4 {
		return 0
	}
	return int32(uint32(role.Value))
}

func (o *comObject) accessibleString(method int) string {
	var bstr *uint16
	if hr := o.call(method, uintptr(unsafe.Pointer(childSelf())), uintptr(unsafe.Pointer(&bstr))); failed(hr) {
		return ""
	}
	return takeBSTR(bstr)
}

func (o *comObject) accessibleChildCount() int {
	var count int32
	if hr := o.call(accChildCount, uintptr(unsafe.Pointer(&count))); failed(hr) {
		return 0
	}
	return int(count)
}

// accessibleChildren returns the children that are objects in their own
// right; simple elements (plain child IDs) have no children or URL.
func (o *comObject) accessibleChildren(limit int) []*comObject {
	count := min(o.accessibleChildCount(), limit)
	if count <= 0 {
		return nil
	}
	values := make([]variant, count)
	var obtained int32
	hr := call(procAccessibleChildren, uintptr(unsafe.Pointer(o)), 0, uintptr(count),
		uintptr(unsafe.Pointer(&values[0])), uintptr(unsafe.Pointer(&obtained)))
	if failed(hr) {
		return nil
	}

	var children []*comObject
	for index := range values[:min(int(obtained), count)] {
		value := &values[index]
		if value.VT == vtDispatch && value.Value != 0 {
			if child, err := value.object().queryInterface(&iidIAccessible); err == nil {
				children = append(children, child)
			}
		}
		value.clear()
	}
	return children
}

// IShellLinkW / IPersistFile, for reading Recent shortcuts and creating the
// Start menu entry.
var (
	clsidShellLink = guid{0x00021401, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLinkW  = guid{0x000214F9, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile = guid{0x0000010B, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	shellLinkGetPath             = 3
	shellLinkSetDescription      = 7
	shellLinkSetWorkingDirectory = 9
	shellLinkSetArguments        = 11
	shellLinkSetIconLocation     = 17
	shellLinkSetPath             = 20
	persistFileLoad              = 5
	persistFileSave              = 6
)

func shortcutTarget(shortcutPath string) (string, error) {
	link, err := createInstance(&clsidShellLink, &iidShellLinkW)
	if err != nil {
		return "", err
	}
	defer link.release()
	file, err := link.queryInterface(&iidPersistFile)
	if err != nil {
		return "", err
	}
	defer file.release()

	const stgmRead = 0
	if hr := file.call(persistFileLoad, uintptr(unsafe.Pointer(utf16Ptr(shortcutPath))), stgmRead); failed(hr) {
		return "", hresultError(hr)
	}
	buffer := make([]uint16, 4096)
	if hr := link.call(shellLinkGetPath, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0); failed(hr) {
		return "", hresultError(hr)
	}
	target := syscall.UTF16ToString(buffer)
	if target == "" {
		return "", errors.New("shortcut has no file target")
	}
	return target, nil
}

func createShortcut(shortcutPath, target, arguments, description string) error {
	link, err := createInstance(&clsidShellLink, &iidShellLinkW)
	if err != nil {
		return err
	}
	defer link.release()

	for _, step := range []struct {
		method int
		value  string
	}{
		{shellLinkSetPath, target},
		{shellLinkSetArguments, arguments},
		{shellLinkSetDescription, description},
		{shellLinkSetWorkingDirectory, parentDirectory(target)},
	} {
		if hr := link.call(step.method, uintptr(unsafe.Pointer(utf16Ptr(step.value)))); failed(hr) {
			return hresultError(hr)
		}
	}
	if hr := link.call(shellLinkSetIconLocation, uintptr(unsafe.Pointer(utf16Ptr(target))), 0); failed(hr) {
		return hresultError(hr)
	}

	file, err := link.queryInterface(&iidPersistFile)
	if err != nil {
		return err
	}
	defer file.release()
	if hr := file.call(persistFileSave, uintptr(unsafe.Pointer(utf16Ptr(shortcutPath))), 1); failed(hr) {
		return hresultError(hr)
	}
	return nil
}
