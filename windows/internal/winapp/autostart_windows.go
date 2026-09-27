package winapp

import (
	"fmt"
	"strings"
)

// Launch at login uses the per-user Run key, the Windows counterpart of the
// macOS login item. Like the macOS version it is switched on by default the
// first time the installed copy runs, and the choice in the menu is kept.
const (
	runKeyPath             = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupApprovedKeyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	settingsKeyPath        = `Software\XMindAutoSave`
	launchAtLoginValue     = "LaunchAtLogin"
	autostartArgument      = "--autostart"
)

type launchAtLogin struct {
	executable string
}

func (l launchAtLogin) command() string {
	return fmt.Sprintf(`"%s" %s`, l.executable, autostartArgument)
}

// enabled reports whether Windows will start the app at login. Task Manager's
// "Startup apps" page can disable the entry without removing it.
func (l launchAtLogin) enabled() bool {
	if _, ok := registryString(runKeyPath, appName); !ok {
		return false
	}
	valueType, data, err := registryValue(startupApprovedKeyPath, appName)
	disabledInSettings := err == nil && valueType == 3 /* REG_BINARY */ && len(data) > 0 && data[0]&1 == 1
	return !disabledInSettings
}

// configureDefault turns the Run entry on the first time and keeps it
// pointing at the current executable afterwards.
func (l launchAtLogin) configureDefault() error {
	wanted, ok := registryDWORD(settingsKeyPath, launchAtLoginValue)
	if !ok {
		return l.setEnabled(true)
	}
	if wanted == 0 {
		return nil
	}
	if current, ok := registryString(runKeyPath, appName); !ok || !strings.EqualFold(current, l.command()) {
		return setRegistryString(runKeyPath, appName, l.command())
	}
	return nil
}

func (l launchAtLogin) setEnabled(enabled bool) error {
	if err := setRegistryDWORD(settingsKeyPath, launchAtLoginValue, boolToDWORD(enabled)); err != nil {
		return err
	}
	if !enabled {
		return deleteRegistryValue(runKeyPath, appName)
	}
	if err := setRegistryString(runKeyPath, appName, l.command()); err != nil {
		return err
	}
	// Turning it on here overrides an earlier "disabled" in Startup apps.
	return deleteRegistryValue(startupApprovedKeyPath, appName)
}

func boolToDWORD(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}
