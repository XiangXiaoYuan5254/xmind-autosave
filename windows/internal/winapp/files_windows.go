package winapp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

const (
	appName        = "XMindAutoSave"
	appDisplayName = "XMind 自动保存"
	executableName = "XMindAutoSave.exe"
)

// dataDirectory is %APPDATA%\XMindAutoSave: preferences, status and logs.
func dataDirectory() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, appName)
}

// installDirectory is %LOCALAPPDATA%\Programs\XMindAutoSave, the per-user
// location Windows uses for apps installed without administrator rights.
func installDirectory() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Programs", appName), nil
}

func parentDirectory(path string) string {
	return filepath.Dir(path)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func currentExecutable() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// fileIdentityKey keys preferences by volume serial number and file ID, which
// survive renames and moves on the same volume (like device + inode on macOS).
func fileIdentityKey(path string) string {
	const (
		shareAll               = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | 0x4 // | FILE_SHARE_DELETE
		fileFlagBackupSemantic = 0x02000000
	)
	fallback := core.PathKeyPrefix + strings.ToLower(filepath.Clean(path))
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fallback
	}
	// Access 0 only queries attributes, so this works while XMind has the
	// file open for writing.
	handle, err := syscall.CreateFile(name, 0, shareAll, nil, syscall.OPEN_EXISTING, fileFlagBackupSemantic, 0)
	if err != nil {
		return fallback
	}
	defer syscall.CloseHandle(handle)

	var info syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(handle, &info) != nil {
		return fallback
	}
	fileID := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return fmt.Sprintf("file:%d:%d", info.VolumeSerialNumber, fileID)
}

// logger appends to %APPDATA%\XMindAutoSave\log.txt, keeping it small.
type logger struct {
	mu   sync.Mutex
	path string
}

func newLogger(directory string) *logger {
	path := filepath.Join(directory, "log.txt")
	if info, err := os.Stat(path); err == nil && info.Size() > 512*1024 {
		_ = os.Rename(path, filepath.Join(directory, "log.old.txt"))
	}
	return &logger{path: path}
}

func (l *logger) printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
}

// Registry helpers (HKEY_CURRENT_USER only; nothing here needs admin rights).
const (
	keyRead  = 0x20019
	keyWrite = 0x20006
	regSZ    = 1
	regDWORD = 4
)

func registryError(status uintptr) error {
	if uint32(status) == 0 {
		return nil
	}
	return syscall.Errno(uint32(status))
}

func openRegistryKey(path string, write bool) (syscall.Handle, error) {
	var key syscall.Handle
	if write {
		status := call(procRegCreateKeyExW, uintptr(syscall.HKEY_CURRENT_USER),
			uintptr(unsafe.Pointer(utf16Ptr(path))), 0, 0, 0, keyRead|keyWrite, 0,
			uintptr(unsafe.Pointer(&key)), 0)
		return key, registryError(status)
	}
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, utf16Ptr(path), 0, keyRead, &key)
	return key, err
}

func setRegistryString(path, name, value string) error {
	key, err := openRegistryKey(path, true)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	data := utf16.Encode([]rune(value + "\x00"))
	return registryError(call(procRegSetValueExW, uintptr(key), uintptr(unsafe.Pointer(utf16Ptr(name))), 0, regSZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2)))
}

func setRegistryDWORD(path, name string, value uint32) error {
	key, err := openRegistryKey(path, true)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	return registryError(call(procRegSetValueExW, uintptr(key), uintptr(unsafe.Pointer(utf16Ptr(name))), 0, regDWORD,
		uintptr(unsafe.Pointer(&value)), 4))
}

func registryValue(path, name string) (valueType uint32, data []byte, err error) {
	key, err := openRegistryKey(path, false)
	if err != nil {
		return 0, nil, err
	}
	defer syscall.RegCloseKey(key)

	var size uint32
	if err := syscall.RegQueryValueEx(key, utf16Ptr(name), nil, &valueType, nil, &size); err != nil {
		return 0, nil, err
	}
	if size == 0 {
		return valueType, nil, nil
	}
	data = make([]byte, size)
	if err := syscall.RegQueryValueEx(key, utf16Ptr(name), nil, &valueType, &data[0], &size); err != nil {
		return 0, nil, err
	}
	return valueType, data[:size], nil
}

func registryString(path, name string) (string, bool) {
	valueType, data, err := registryValue(path, name)
	if err != nil || valueType != regSZ || len(data) < 2 {
		return "", false
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&data[0])), len(data)/2)), true
}

func registryDWORD(path, name string) (uint32, bool) {
	valueType, data, err := registryValue(path, name)
	if err != nil || valueType != regDWORD || len(data) < 4 {
		return 0, false
	}
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24, true
}

func deleteRegistryValue(path, name string) error {
	key, err := openRegistryKey(path, false)
	if err != nil {
		return nil // nothing to delete
	}
	syscall.RegCloseKey(key)
	var writable syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, utf16Ptr(path), 0, keyWrite, &writable); err != nil {
		return err
	}
	defer syscall.RegCloseKey(writable)
	err = registryError(call(procRegDeleteValueW, uintptr(writable), uintptr(unsafe.Pointer(utf16Ptr(name)))))
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

func deleteRegistryKey(path string) error {
	err := registryError(call(procRegDeleteKeyW, uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(utf16Ptr(path)))))
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}
