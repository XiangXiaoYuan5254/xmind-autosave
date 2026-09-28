package winapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/XiangXiaoYuan5254/xmind-autosave/windows/internal/core"
)

// Update checks: shortly after start and then once a day, ask GitHub for the
// latest release. A newer one is announced once with a notification and stays
// in the menu until installed. Running the new installer replaces this copy.
const (
	checkForUpdatesValue       = "CheckForUpdates"
	notifiedUpdateVersionValue = "NotifiedUpdateVersion"
	internetSettingsKeyPath    = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	firstUpdateCheckAfter = 30 * time.Second
	updateCheckTick       = time.Hour
	updateCheckInterval   = 24 * time.Hour
	updateCheckTimeout    = 20 * time.Second
)

type updateState struct {
	current     core.Version // nil in development builds
	available   *core.Update
	checking    bool
	lastChecked time.Time // last check that reached GitHub
}

func (a *App) automaticUpdateChecks() bool {
	value, ok := registryDWORD(settingsKeyPath, checkForUpdatesValue)
	return !ok || value != 0
}

func (a *App) startUpdateChecks() {
	current, ok := core.ParseVersion(a.version)
	if !ok {
		return
	}
	a.updates.current = current
	call(procSetTimer, a.hwnd, timerUpdateCheck, uintptr(firstUpdateCheckAfter/time.Millisecond), 0)
}

// updateTimerFired runs 30 seconds after start and then every hour; failed
// checks are retried on the next tick, successful ones after a day.
func (a *App) updateTimerFired() {
	call(procSetTimer, a.hwnd, timerUpdateCheck, uintptr(updateCheckTick/time.Millisecond), 0)
	if a.automaticUpdateChecks() && time.Since(a.updates.lastChecked) >= updateCheckInterval {
		a.checkForUpdates(false)
	}
}

func (a *App) setAutomaticUpdateChecks(enabled bool) {
	if err := setRegistryDWORD(settingsKeyPath, checkForUpdatesValue, boolToDWORD(enabled)); err != nil {
		a.log.printf("无法更改自动检查更新：%v", err)
		return
	}
	if enabled && a.updates.lastChecked.IsZero() {
		a.checkForUpdates(false)
	}
}

// checkForUpdates asks the website on its own goroutine. userInitiated checks
// report every outcome; automatic ones only announce a new version.
func (a *App) checkForUpdates(userInitiated bool) {
	if a.updates.current == nil || a.updates.checking {
		return
	}
	a.updates.checking = true
	current := a.updates.current
	userAgent := fmt.Sprintf("XMindAutoSave/%s (Windows)", a.version)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = systemProxy
		client := &http.Client{Transport: transport}
		update, found, err := core.CheckForUpdate(ctx, client, core.LatestReleaseURL, userAgent, current)
		a.worker.complete(func() { a.updateChecked(update, found, err, userInitiated) })
	}()
}

func (a *App) updateChecked(update core.Update, found bool, err error, userInitiated bool) {
	a.updates.checking = false
	if err != nil {
		a.log.printf("检查更新失败：%v", err)
		if userInitiated {
			messageBox(a.hwnd, "暂时无法检查更新，请检查网络后再试。\n\n"+err.Error(), appDisplayName, mbOK|mbIconWarning|mbSetForeground)
		}
		return
	}
	a.updates.lastChecked = time.Now()
	if !found {
		a.updates.available = nil
		if userInitiated {
			messageBox(a.hwnd, "当前版本 "+a.version+" 已是最新版本。", appDisplayName, mbOK|mbIconInformation|mbSetForeground)
		}
		return
	}

	a.updates.available = &update
	version := update.Version.String()
	if userInitiated {
		_ = setRegistryString(settingsKeyPath, notifiedUpdateVersionValue, version)
		question := fmt.Sprintf("发现新版本 %s（当前版本 %s）。\n\n现在打开下载页面吗？下载后运行新的安装程序即可替换旧版，各文件的开关设置会保留。", version, a.version)
		if messageBox(a.hwnd, question, appDisplayName, mbYesNo|mbIconInformation|mbSetForeground) == idYes {
			shellOpen(update.Page)
		}
		return
	}
	if notified, _ := registryString(settingsKeyPath, notifiedUpdateVersionValue); notified == version {
		return
	}
	a.log.printf("发现新版本 %s", version)
	_ = setRegistryString(settingsKeyPath, notifiedUpdateVersionValue, version)
	a.notify("XMind 自动保存有新版本 "+version, "单击这里前往下载；也可以稍后从通知区域图标的菜单中下载。", func() {
		shellOpen(update.Page)
	})
}

// systemProxy uses HTTPS_PROXY and friends when set, otherwise the proxy from
// Windows Settings → Network → Proxy, which Go does not read on its own.
// Automatic configuration scripts (PAC) are not supported.
func systemProxy(request *http.Request) (*url.URL, error) {
	if proxy, err := http.ProxyFromEnvironment(request); proxy != nil || err != nil {
		return proxy, err
	}
	if enabled, ok := registryDWORD(internetSettingsKeyPath, "ProxyEnable"); !ok || enabled == 0 {
		return nil, nil
	}
	setting, _ := registryString(internetSettingsKeyPath, "ProxyServer")
	address := core.ProxyAddress(setting, request.URL.Scheme)
	if address == "" {
		return nil, nil
	}
	return url.Parse(address)
}
