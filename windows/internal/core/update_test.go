package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	for text, want := range map[string]string{
		"1.0.3":    "1.0.3",
		"v1.0.4":   "1.0.4",
		" 2.10 ":   "2.10",
		"V1.2.3.4": "1.2.3.4",
	} {
		version, ok := ParseVersion(text)
		if !ok || version.String() != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %s", text, version, ok, want)
		}
	}
	for _, text := range []string{"", "dev", "ci-abc1234", "1.0.3-beta", "1..3", "1.2.3.4.5", "v", "+1.0"} {
		if version, ok := ParseVersion(text); ok {
			t.Errorf("ParseVersion(%q) = %v; want no version", text, version)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.3", "1.0.4", -1},
		{"1.0.10", "1.0.9", 1},
		{"1.1", "1.1.0", 0},
		{"2.0.0", "1.99.99", 1},
	}
	for _, testCase := range cases {
		a, _ := ParseVersion(testCase.a)
		b, _ := ParseVersion(testCase.b)
		if got := a.Compare(b); got != testCase.want {
			t.Errorf("%s vs %s = %d; want %d", testCase.a, testCase.b, got, testCase.want)
		}
	}
}

const releaseResponse = `{
	"tag_name": "v1.0.4",
	"html_url": "https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/tag/v1.0.4",
	"assets": [
		{"name": "SHA256SUMS.txt"},
		{"name": "XMindAutoSave-1.0.4.dmg"},
		{"name": "XMindAutoSave-Setup-1.0.4.exe"}
	]
}`

func TestAvailableUpdateOffersNewerRelease(t *testing.T) {
	current, _ := ParseVersion("1.0.3")
	update, ok, err := AvailableUpdate([]byte(releaseResponse), current)
	if err != nil || !ok {
		t.Fatalf("AvailableUpdate = %v, %v", ok, err)
	}
	if update.Version.String() != "1.0.4" ||
		update.Page != "https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/tag/v1.0.4" {
		t.Fatalf("update = %+v", update)
	}
}

func TestAvailableUpdateIgnoresSameOrOlderRelease(t *testing.T) {
	for _, text := range []string{"1.0.4", "1.1.0"} {
		current, _ := ParseVersion(text)
		if update, ok, err := AvailableUpdate([]byte(releaseResponse), current); ok || err != nil {
			t.Errorf("current %s: update = %+v, %v, %v", text, update, ok, err)
		}
	}
}

func TestAvailableUpdateNeedsWindowsInstaller(t *testing.T) {
	current, _ := ParseVersion("1.0.3")
	macOnly := `{"tag_name": "v1.0.4", "assets": [{"name": "XMindAutoSave-1.0.4.dmg"}]}`
	if update, ok, err := AvailableUpdate([]byte(macOnly), current); ok || err != nil {
		t.Fatalf("update = %+v, %v, %v", update, ok, err)
	}
}

func TestAvailableUpdateOpensOnlyThisProjectsPages(t *testing.T) {
	current, _ := ParseVersion("1.0.3")
	elsewhere := `{"tag_name": "v1.0.4", "html_url": "https://example.com/download",
		"assets": [{"name": "XMindAutoSave-Setup-1.0.4.exe"}]}`
	update, ok, err := AvailableUpdate([]byte(elsewhere), current)
	if err != nil || !ok || update.Page != LatestReleasePage {
		t.Fatalf("update = %+v, %v, %v", update, ok, err)
	}
}

func TestAvailableUpdateRejectsUnknownTag(t *testing.T) {
	current, _ := ParseVersion("1.0.3")
	if _, ok, err := AvailableUpdate([]byte(`{"tag_name": "nightly"}`), current); ok || err == nil {
		t.Fatalf("ok = %v, err = %v; want an error", ok, err)
	}
}

func TestCheckForUpdate(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(releaseResponse))
	}))
	defer server.Close()

	current, _ := ParseVersion("1.0.3")
	update, ok, err := CheckForUpdate(context.Background(), server.Client(), server.URL, "XMindAutoSave/1.0.3 (Windows)", current)
	if err != nil || !ok || update.Version.String() != "1.0.4" {
		t.Fatalf("CheckForUpdate = %+v, %v, %v", update, ok, err)
	}
	if userAgent != "XMindAutoSave/1.0.3 (Windows)" {
		t.Fatalf("User-Agent = %q", userAgent)
	}
}

func TestCheckForUpdateReportsHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()

	current, _ := ParseVersion("1.0.3")
	if _, ok, err := CheckForUpdate(context.Background(), server.Client(), server.URL, "test", current); ok || err == nil {
		t.Fatalf("ok = %v, err = %v; want an error", ok, err)
	}
}

// Set XMIND_AUTOSAVE_NETWORK_TESTS=1 to also ask the real GitHub API.
func TestCheckForUpdateAgainstGitHub(t *testing.T) {
	if os.Getenv("XMIND_AUTOSAVE_NETWORK_TESTS") != "1" {
		t.Skip("set XMIND_AUTOSAVE_NETWORK_TESTS=1 to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	oldest, _ := ParseVersion("0.0.1")
	update, ok, err := CheckForUpdate(ctx, http.DefaultClient, LatestReleaseAPI, "XMindAutoSave/test", oldest)
	if err != nil || !ok {
		t.Fatalf("CheckForUpdate = %+v, %v, %v", update, ok, err)
	}
	t.Logf("latest release: %s at %s", update.Version, update.Page)
}

func TestProxyAddress(t *testing.T) {
	cases := []struct {
		setting, scheme, want string
	}{
		{"127.0.0.1:7890", "https", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:8080;https=127.0.0.1:8443", "https", "http://127.0.0.1:8443"},
		{"http=proxy:80;ftp=proxy:21", "https", "http://proxy:80"},
		{"socks=127.0.0.1:1080", "https", ""},
		{"http://proxy.example:3128", "https", "http://proxy.example:3128"},
		{"", "https", ""},
	}
	for _, testCase := range cases {
		if got := ProxyAddress(testCase.setting, testCase.scheme); got != testCase.want {
			t.Errorf("ProxyAddress(%q, %q) = %q; want %q", testCase.setting, testCase.scheme, got, testCase.want)
		}
	}
}
