package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Update checks read latest.json on the website (helloxxy.com), like the
// macOS version, for example
// {"version": "1.0.5", "files": ["XMindAutoSave-1.0.5.dmg", "XMindAutoSave-Setup-1.0.5.exe"]}.
// They read only its version number and file names; nothing is uploaded.
const (
	LatestReleaseURL = "https://helloxxy.com/works/xmind-autosave/downloads/latest.json"
	DownloadPage     = "https://helloxxy.com/works/xmind-autosave/#download"
)

// Version is a release number such as 1.0.3.
type Version []int

// ParseVersion reads "1.0.3" or "v1.0.3". Development builds ("dev",
// "ci-abc1234") are not versions, so they never look for updates.
func ParseVersion(text string) (Version, bool) {
	text = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "v"), "V")
	parts := strings.Split(text, ".")
	if len(parts) > 4 {
		return nil, false
	}
	version := make(Version, len(parts))
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return nil, false
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		version[i] = number
	}
	return version, true
}

// Compare returns -1, 0 or 1; missing parts count as 0, so 1.1 equals 1.1.0.
func (v Version) Compare(other Version) int {
	for i := 0; i < max(len(v), len(other)); i++ {
		var a, b int
		if i < len(v) {
			a = v[i]
		}
		if i < len(other) {
			b = other[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (v Version) String() string {
	parts := make([]string, len(v))
	for i, number := range v {
		parts[i] = strconv.Itoa(number)
	}
	return strings.Join(parts, ".")
}

// Update is a newer release that has a Windows installer.
type Update struct {
	Version Version
	Page    string
}

type latestRelease struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

// AvailableUpdate reads latest.json and returns the release when it is newer
// than current and ships a Windows installer — a macOS-only release is not
// offered here.
func AvailableUpdate(response []byte, current Version) (Update, bool, error) {
	var release latestRelease
	if err := json.Unmarshal(response, &release); err != nil {
		return Update{}, false, err
	}
	version, ok := ParseVersion(release.Version)
	if !ok {
		return Update{}, false, fmt.Errorf("无法识别的版本号 %q", release.Version)
	}
	if version.Compare(current) <= 0 {
		return Update{}, false, nil
	}
	hasInstaller := false
	for _, file := range release.Files {
		hasInstaller = hasInstaller || IsWindowsInstaller(file)
	}
	if !hasInstaller {
		return Update{}, false, nil
	}
	return Update{Version: version, Page: DownloadPage}, true, nil
}

// IsWindowsInstaller matches the file package_windows.sh produces.
func IsWindowsInstaller(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "xmindautosave-setup-") && strings.HasSuffix(lower, ".exe")
}

// CheckForUpdate reads latestURL (LatestReleaseURL outside tests).
func CheckForUpdate(ctx context.Context, client *http.Client, latestURL, userAgent string, current Version) (Update, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return Update{}, false, err
	}
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return Update{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Update{}, false, fmt.Errorf("服务器返回 %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Update{}, false, err
	}
	return AvailableUpdate(body, current)
}

// ProxyAddress turns the WinINet ProxyServer setting — "host:port" or
// "http=host:port;https=host:port" — into the proxy URL for scheme, or "".
func ProxyAddress(setting, scheme string) string {
	setting = strings.TrimSpace(setting)
	address := setting
	if strings.Contains(setting, "=") {
		entries := map[string]string{}
		for _, entry := range strings.Split(setting, ";") {
			if key, value, ok := strings.Cut(strings.TrimSpace(entry), "="); ok {
				entries[strings.ToLower(key)] = strings.TrimSpace(value)
			}
		}
		address = entries[strings.ToLower(scheme)]
		if address == "" {
			address = entries["http"]
		}
	}
	if address == "" {
		return ""
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	return address
}
