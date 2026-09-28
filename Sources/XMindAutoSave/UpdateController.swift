import AppKit
import Foundation
import OSLog
import XMindAutoSaveCore

/// Asks the website for the latest release shortly after launch and then once a
/// day. A newer version is announced once and stays in the menu until it is
/// installed; downloading and replacing the app is left to the user.
@MainActor
final class UpdateController: NSObject, NSMenuItemValidation {
    let availableItem = NSMenuItem(title: "", action: #selector(openDownloadPage), keyEquivalent: "")
    let availableSeparator = NSMenuItem.separator()
    let checkNowItem = NSMenuItem(title: "检查更新…", action: #selector(checkNow), keyEquivalent: "")
    let automaticChecksItem = NSMenuItem(title: "自动检查更新", action: #selector(toggleAutomaticChecks), keyEquivalent: "")

    private let firstCheckDelay: TimeInterval = 30
    private let tickInterval: TimeInterval = 60 * 60
    private let checkInterval: TimeInterval = 24 * 60 * 60
    private let automaticChecksKey = "automaticallyChecksForUpdates"
    private let notifiedVersionKey = "notifiedUpdateVersion"

    private let xmindBundleIdentifier: String
    /// nil in builds without a release version, which never check.
    private let currentVersion = (Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String)
        .flatMap(AppVersion.init)
    private let session = URLSession(configuration: .ephemeral)
    private let defaults = UserDefaults.standard
    private let logger = Logger(subsystem: "local.xmind.autosave", category: "updates")

    private var timer: Timer?
    private var activationObserver: NSObjectProtocol?
    private var isChecking = false
    private var lastSuccessfulCheck: Date?
    private var availableUpdate: AvailableUpdate?
    private var pendingAnnouncement: AvailableUpdate?
    private var shownAlert: NSAlert?
    private var shownAlertAction: (() -> Void)?

    init(xmindBundleIdentifier: String) {
        self.xmindBundleIdentifier = xmindBundleIdentifier
        super.init()
        for item in [availableItem, checkNowItem, automaticChecksItem] {
            item.target = self
        }
        refreshMenuItems()
    }

    private var automaticChecks: Bool {
        defaults.object(forKey: automaticChecksKey) as? Bool ?? true
    }

    func start() {
        guard currentVersion != nil, timer == nil else { return }

        // Announcements wait while XMind is in front: the alert would take
        // the keyboard, and Return — which adds a topic in XMind — would
        // press its default button.
        activationObserver = NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.announcePendingUpdate()
            }
        }

        timer = Timer.scheduledTimer(withTimeInterval: firstCheckDelay, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.timerFired()
            }
        }
    }

    /// Runs 30 seconds after launch and then every hour. Failed checks are
    /// retried on the next tick, successful ones after a day.
    private func timerFired() {
        if timer?.isValid != true {
            timer = Timer.scheduledTimer(withTimeInterval: tickInterval, repeats: true) { [weak self] _ in
                MainActor.assumeIsolated {
                    self?.timerFired()
                }
            }
            timer?.tolerance = 60
        }

        let checkIsDue = lastSuccessfulCheck.map { Date().timeIntervalSince($0) >= checkInterval } ?? true
        if automaticChecks && checkIsDue {
            check(userInitiated: false)
        }
    }

    @objc private func checkNow() {
        check(userInitiated: true)
    }

    @objc private func toggleAutomaticChecks() {
        defaults.set(!automaticChecks, forKey: automaticChecksKey)
        refreshMenuItems()
        if automaticChecks && lastSuccessfulCheck == nil {
            check(userInitiated: false)
        }
    }

    @objc private func openDownloadPage() {
        NSWorkspace.shared.open(availableUpdate?.pageURL ?? UpdateCheck.downloadPage)
    }

    /// Checks made from the menu report every outcome; automatic ones only
    /// announce a new version.
    private func check(userInitiated: Bool) {
        guard let currentVersion, !isChecking else { return }
        isChecking = true
        refreshMenuItems()

        Task {
            let result: Result<AvailableUpdate?, Error>
            do {
                result = .success(try await UpdateCheck.fetchAvailableUpdate(currentVersion: currentVersion, session: session))
            } catch {
                result = .failure(error)
            }
            finishCheck(result, currentVersion: currentVersion, userInitiated: userInitiated)
        }
    }

    private func finishCheck(_ result: Result<AvailableUpdate?, Error>, currentVersion: AppVersion, userInitiated: Bool) {
        isChecking = false
        defer { refreshMenuItems() }

        let update: AvailableUpdate?
        switch result {
        case .failure(let error):
            logger.error("Update check failed: \(error.localizedDescription, privacy: .public)")
            if userInitiated {
                let alert = NSAlert()
                alert.alertStyle = .warning
                alert.messageText = "暂时无法检查更新"
                alert.informativeText = "请检查网络后再试。\n\n\(error.localizedDescription)"
                show(alert)
            }
            return
        case .success(let found):
            update = found
        }

        lastSuccessfulCheck = Date()
        availableUpdate = update
        guard let update else {
            pendingAnnouncement = nil
            if userInitiated {
                let alert = NSAlert()
                alert.messageText = "已是最新版本"
                alert.informativeText = "当前版本 \(currentVersion) 已是最新版本。"
                show(alert)
            }
            return
        }

        logger.info("Update available: \(update.version.description, privacy: .public)")
        if userInitiated {
            announce(update, currentVersion: currentVersion)
        } else if defaults.string(forKey: notifiedVersionKey) != update.version.description {
            pendingAnnouncement = update
            announcePendingUpdate()
        }
    }

    private func announcePendingUpdate() {
        guard
            let update = pendingAnnouncement,
            let currentVersion,
            NSWorkspace.shared.frontmostApplication?.bundleIdentifier != xmindBundleIdentifier
        else {
            return
        }
        pendingAnnouncement = nil
        announce(update, currentVersion: currentVersion)
    }

    private func announce(_ update: AvailableUpdate, currentVersion: AppVersion) {
        defaults.set(update.version.description, forKey: notifiedVersionKey)

        let alert = NSAlert()
        alert.messageText = "XMind 自动保存有新版本 \(update.version)"
        alert.informativeText = """
        当前版本是 \(currentVersion)。下载新的 DMG 后，先从菜单栏退出本程序，再把新版拖进“应用程序”替换旧版；各文件的开关设置会保留。

        如果更新后自动保存不工作，请在“系统设置 → 隐私与安全性 → 辅助功能”中移除 XMindAutoSave，再打开新版重新授权。
        """
        alert.addButton(withTitle: "前往下载")
        alert.addButton(withTitle: "以后再说")
        show(alert) { [weak self] in
            self?.openDownloadPage()
        }
    }

    /// Shows the alert as an ordinary window instead of running it modally,
    /// so autosave and the title bar toggle keep working while it is open.
    private func show(_ alert: NSAlert, primaryAction: (() -> Void)? = nil) {
        shownAlert?.window.orderOut(nil)

        if alert.buttons.isEmpty {
            alert.addButton(withTitle: "好")
        }
        for (index, button) in alert.buttons.enumerated() {
            button.tag = index
            button.target = self
            button.action = #selector(alertButtonPressed(_:))
        }
        if alert.buttons.count > 1 {
            alert.buttons[1].keyEquivalent = "\u{1b}"
        }
        alert.layout()

        let window = alert.window
        window.hidesOnDeactivate = false
        window.level = .floating
        window.center()
        shownAlert = alert
        shownAlertAction = primaryAction

        if #available(macOS 14.0, *) {
            NSApp.activate()
        } else {
            NSApp.activate(ignoringOtherApps: true)
        }
        window.makeKeyAndOrderFront(nil)
    }

    @objc private func alertButtonPressed(_ sender: NSButton) {
        let action = sender.tag == 0 ? shownAlertAction : nil
        shownAlert?.window.orderOut(nil)
        shownAlert = nil
        shownAlertAction = nil
        action?()
    }

    private func refreshMenuItems() {
        if let availableUpdate {
            availableItem.title = "有新版本 \(availableUpdate.version)，前往下载…"
        }
        availableItem.isHidden = availableUpdate == nil
        availableSeparator.isHidden = availableUpdate == nil
        checkNowItem.title = isChecking ? "正在检查更新…" : "检查更新…"
        automaticChecksItem.state = automaticChecks ? .on : .off
    }

    func validateMenuItem(_ menuItem: NSMenuItem) -> Bool {
        switch menuItem.action {
        case #selector(checkNow):
            return currentVersion != nil && !isChecking
        case #selector(toggleAutomaticChecks):
            return currentVersion != nil
        default:
            return true
        }
    }
}
