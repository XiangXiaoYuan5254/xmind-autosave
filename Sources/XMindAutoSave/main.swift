import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import OSLog
import XMindAutoSaveCore

@MainActor
private func handleMaintenanceCommand() -> Bool {
    guard let command = CommandLine.arguments.dropFirst().first else { return false }
    let controller = LaunchAtLoginController()

    do {
        switch command {
        case "--login-item-status":
            print(controller.status.rawValue)
        case "--register-login-item":
            try controller.setEnabled(true)
            print(controller.status.rawValue)
        case "--unregister-login-item":
            try controller.setEnabled(false)
            print(controller.status.rawValue)
        default:
            return false
        }
    } catch {
        FileHandle.standardError.write(Data("\(error.localizedDescription)\n".utf8))
        exit(1)
    }
    return true
}

private let xmindBundleIdentifier = "net.xmind.vana.app"
private let logger = Logger(subsystem: "local.xmind.autosave", category: "autosave")
private let showOverlayWhenInactive = ProcessInfo.processInfo.environment["XMIND_AUTOSAVE_SHOW_WHEN_INACTIVE"] == "1"

private struct ScanResult {
    let filePath: String
    let isDirty: Bool
    let windowFrame: CGRect
    let isFullScreen: Bool
    let activitySignature: String
}

private struct SaveState {
    var dirtySince: Date?
    var lastEditActivityAt: Date?
    var observedInputGeneration = -1
    var observedActivitySignature: String?
    var lastAttempt: Date?
    var lastSuccessfulSave: Date?
}

@MainActor
private final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    private var statusMenuItem: NSMenuItem!
    private var currentDocumentMenuItem: NSMenuItem!
    private var currentAutoSaveMenuItem: NSMenuItem!
    private var launchAtLoginMenuItem: NSMenuItem!
    private var timer: Timer?
    private var configuration: AutoSaveConfiguration?
    private var autoSaveStore: PerFileAutoSaveStore?
    private var overlayPanel: AttachedTogglePanel?
    private var activeFilePath: String?
    private var saveStates: [String: SaveState] = [:]
    private var didRequestAccessibility = false
    private var lastStatusText = ""
    private let launchAtLoginController = LaunchAtLoginController()
    private var globalEventMonitor: Any?
    private var xmindInputGeneration = 0
    private var lastXMindInputAt: Date?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        configureStatusItem()
        configureLaunchAtLogin()

        do {
            let loaded = try loadConfiguration()
            configuration = loaded
            autoSaveStore = makePreferenceStore(initiallyEnabledPaths: loaded.monitoredFiles)
            overlayPanel = AttachedTogglePanel { [weak self] filePath, enabled in
                self?.setAutoSave(enabled, for: filePath)
            }

            requestAccessibilityIfNeeded()
            startMonitoring()
        } catch {
            updateStatus("配置读取失败", symbol: "exclamationmark.triangle")
            logger.error("Configuration error: \(error.localizedDescription, privacy: .public)")
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        timer?.invalidate()
        if let globalEventMonitor {
            NSEvent.removeMonitor(globalEventMonitor)
            self.globalEventMonitor = nil
        }
        overlayPanel?.hide()
    }

    private func configureStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = NSImage(systemSymbolName: "externaldrive.fill.badge.checkmark", accessibilityDescription: "XMind 自动保存")
            button.toolTip = "XMind 自动保存"
        }

        let menu = NSMenu()
        statusMenuItem = NSMenuItem(title: "正在启动…", action: nil, keyEquivalent: "")
        statusMenuItem.isEnabled = false
        menu.addItem(statusMenuItem)

        currentDocumentMenuItem = NSMenuItem(title: "当前文件：等待 XMind", action: nil, keyEquivalent: "")
        currentDocumentMenuItem.isEnabled = false
        menu.addItem(currentDocumentMenuItem)

        currentAutoSaveMenuItem = NSMenuItem(
            title: "当前文件自动保存",
            action: #selector(toggleCurrentFileAutoSave),
            keyEquivalent: ""
        )
        currentAutoSaveMenuItem.target = self
        currentAutoSaveMenuItem.isEnabled = false
        menu.addItem(currentAutoSaveMenuItem)
        menu.addItem(.separator())

        let checkNow = NSMenuItem(title: "立即检查并保存", action: #selector(checkNow), keyEquivalent: "s")
        checkNow.target = self
        menu.addItem(checkNow)

        let permission = NSMenuItem(title: "打开“辅助功能”设置…", action: #selector(openAccessibilitySettings), keyEquivalent: "")
        permission.target = self
        menu.addItem(permission)

        launchAtLoginMenuItem = NSMenuItem(
            title: "登录时自动启动",
            action: #selector(toggleLaunchAtLogin),
            keyEquivalent: ""
        )
        launchAtLoginMenuItem.target = self
        menu.addItem(launchAtLoginMenuItem)

        menu.addItem(.separator())
        let quit = NSMenuItem(title: "退出 XMind 自动保存", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)
        statusItem.menu = menu
    }

    private func configureLaunchAtLogin() {
        do {
            try launchAtLoginController.configureDefault()
        } catch {
            logger.error("Could not enable launch at login: \(error.localizedDescription, privacy: .public)")
        }
        refreshLaunchAtLoginMenuItem()
    }

    private func refreshLaunchAtLoginMenuItem() {
        launchAtLoginMenuItem.isEnabled = launchAtLoginController.isInstalledInApplicationsFolder
        guard launchAtLoginController.isInstalledInApplicationsFolder else {
            launchAtLoginMenuItem.state = .off
            launchAtLoginMenuItem.toolTip = "请先将 App 拖入“应用程序”文件夹"
            return
        }

        let status = launchAtLoginController.status
        launchAtLoginMenuItem.state = status == .enabled ? .on : (status == .requiresApproval ? .mixed : .off)
        launchAtLoginMenuItem.toolTip = status == .requiresApproval
            ? "请在系统设置的“登录项”中允许"
            : nil
    }

    @objc private func toggleLaunchAtLogin() {
        guard launchAtLoginController.isInstalledInApplicationsFolder else { return }
        do {
            let shouldEnable = launchAtLoginController.status != .enabled
            try launchAtLoginController.setEnabled(shouldEnable)
        } catch {
            logger.error("Could not update launch at login: \(error.localizedDescription, privacy: .public)")
        }
        refreshLaunchAtLoginMenuItem()
    }

    private func makePreferenceStore(initiallyEnabledPaths: [String]) -> PerFileAutoSaveStore {
        let applicationSupport = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
        let storageURL = applicationSupport
            .appendingPathComponent("XMindAutoSave", isDirectory: true)
            .appendingPathComponent("preferences.json")
        return PerFileAutoSaveStore(
            storageURL: storageURL,
            initiallyEnabledPaths: initiallyEnabledPaths
        )
    }

    private func loadConfiguration() throws -> AutoSaveConfiguration {
        let environmentPath = ProcessInfo.processInfo.environment["XMIND_AUTOSAVE_CONFIG"]
        let configURL: URL?

        if let environmentPath, !environmentPath.isEmpty {
            configURL = URL(fileURLWithPath: environmentPath)
        } else {
            configURL = Bundle.main.url(forResource: "config", withExtension: "json")
        }

        guard let configURL else {
            throw NSError(
                domain: "XMindAutoSave",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "找不到 config.json"]
            )
        }

        let data = try Data(contentsOf: configURL)
        return try JSONDecoder().decode(AutoSaveConfiguration.self, from: data)
    }

    private func requestAccessibilityIfNeeded() {
        guard !AXIsProcessTrusted(), !didRequestAccessibility else { return }
        didRequestAccessibility = true
        let options = ["AXTrustedCheckOptionPrompt": true] as CFDictionary
        _ = AXIsProcessTrustedWithOptions(options)
    }

    private func startMonitoring() {
        guard let configuration else { return }
        installInputActivityMonitor()
        let interval = max(Double(configuration.pollIntervalMilliseconds) / 1_000.0, 0.1)
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.poll()
            }
        }
        timer?.tolerance = min(interval * 0.1, 0.02)
        poll()
    }

    private func installInputActivityMonitor() {
        guard globalEventMonitor == nil else { return }

        globalEventMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.keyDown]) { [weak self] _ in
            guard NSWorkspace.shared.frontmostApplication?.bundleIdentifier == xmindBundleIdentifier else {
                return
            }

            Task { @MainActor [weak self] in
                guard let self else { return }
                self.xmindInputGeneration &+= 1
                self.lastXMindInputAt = Date()
            }
        }
    }

    @objc private func checkNow() {
        poll(forceSave: true)
    }

    @objc private func toggleCurrentFileAutoSave() {
        guard let activeFilePath, let autoSaveStore else { return }
        setAutoSave(!autoSaveStore.isEnabled(forPath: activeFilePath), for: activeFilePath)
    }

    @objc private func openAccessibilitySettings() {
        didRequestAccessibility = false
        requestAccessibilityIfNeeded()
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
            NSWorkspace.shared.open(url)
        }
    }

    private func setAutoSave(_ enabled: Bool, for filePath: String) {
        guard let autoSaveStore else { return }

        do {
            try autoSaveStore.setEnabled(enabled, forPath: filePath)
            currentAutoSaveMenuItem.state = enabled ? .on : .off
            overlayPanel?.update(enabled: enabled, filePath: filePath)

            if enabled {
                updateStatus("当前文件已开启自动保存", symbol: "checkmark.circle.fill")
                poll()
            } else {
                saveStates[filePath] = nil
                updateStatus("当前文件自动保存已关闭", symbol: "pause.circle")
            }
        } catch {
            updateStatus("无法保存文件开关设置", symbol: "exclamationmark.triangle")
            logger.error("Preference error: \(error.localizedDescription, privacy: .public)")
        }
    }

    private func clearCurrentDocumentUI() {
        activeFilePath = nil
        currentDocumentMenuItem.title = "当前文件：无本地文档"
        currentAutoSaveMenuItem.isEnabled = false
        currentAutoSaveMenuItem.state = .off
        overlayPanel?.hide()
    }

    private func poll(forceSave: Bool = false) {
        guard let configuration, let autoSaveStore else { return }

        guard AXIsProcessTrusted() else {
            clearCurrentDocumentUI()
            updateStatus("需要辅助功能权限", symbol: "lock.trianglebadge.exclamationmark")
            return
        }

        guard let xmind = NSWorkspace.shared.runningApplications.first(where: {
            $0.bundleIdentifier == xmindBundleIdentifier && !$0.isTerminated
        }) else {
            clearCurrentDocumentUI()
            updateStatus("等待 XMind", symbol: "moon.zzz")
            return
        }

        guard let scan = scanFocusedDocument(pid: xmind.processIdentifier, configuration: configuration) else {
            clearCurrentDocumentUI()
            updateStatus("当前不是本地 XMind 文档", symbol: "doc.badge.ellipsis")
            return
        }

        let enabled = autoSaveStore.isEnabled(forPath: scan.filePath)
        activeFilePath = scan.filePath
        currentDocumentMenuItem.title = "当前文件：\(URL(fileURLWithPath: scan.filePath).lastPathComponent)"
        currentAutoSaveMenuItem.isEnabled = true
        currentAutoSaveMenuItem.state = enabled ? .on : .off

        let xmindIsFrontmost = NSWorkspace.shared.frontmostApplication?.bundleIdentifier == xmindBundleIdentifier
        if (xmindIsFrontmost || showOverlayWhenInactive) && !scan.isFullScreen {
            overlayPanel?.show(
                filePath: scan.filePath,
                enabled: enabled,
                xmindFrame: scan.windowFrame
            )
        } else {
            overlayPanel?.hide()
        }

        guard enabled else {
            saveStates[scan.filePath] = nil
            updateStatus("当前文件自动保存已关闭", symbol: "pause.circle")
            return
        }

        var state = saveStates[scan.filePath] ?? SaveState()
        let now = Date()

        guard scan.isDirty else {
            let justSaved = state.lastAttempt != nil && state.dirtySince != nil
            state.dirtySince = nil
            state.lastEditActivityAt = nil
            state.observedInputGeneration = xmindInputGeneration
            state.observedActivitySignature = scan.activitySignature
            state.lastAttempt = nil
            if justSaved {
                state.lastSuccessfulSave = now
                logger.info("Saved \(scan.filePath, privacy: .private)")
            }
            saveStates[scan.filePath] = state

            if let savedAt = state.lastSuccessfulSave {
                updateStatus("已自动保存 \(savedAt.formatted(date: .omitted, time: .standard))", symbol: "checkmark.circle.fill")
            } else {
                updateStatus("已保存", symbol: "checkmark.circle")
            }
            return
        }

        if state.dirtySince == nil {
            state.dirtySince = now
        }

        if state.observedInputGeneration < 0 {
            state.observedInputGeneration = xmindInputGeneration
        } else if state.observedInputGeneration != xmindInputGeneration {
            state.observedInputGeneration = xmindInputGeneration
            state.lastEditActivityAt = lastXMindInputAt ?? now
        }

        if state.observedActivitySignature != scan.activitySignature {
            state.observedActivitySignature = scan.activitySignature
            state.lastEditActivityAt = now
        }

        if state.lastEditActivityAt == nil {
            state.lastEditActivityAt = state.dirtySince ?? now
        }

        let delay = max(Double(configuration.saveDelayMilliseconds) / 1_000.0, 0)
        let retry = max(Double(configuration.retryMilliseconds) / 1_000.0, 0.5)
        let dirtyLongEnough = now.timeIntervalSince(state.lastEditActivityAt ?? now) >= delay
        let mayRetry = state.lastAttempt.map { now.timeIntervalSince($0) >= retry } ?? true

        if forceSave || (dirtyLongEnough && mayRetry) {
            postCommandS(to: xmind.processIdentifier)
            state.lastAttempt = now
            updateStatus("正在自动保存…", symbol: "arrow.triangle.2.circlepath")
        } else {
            updateStatus("检测到编辑…", symbol: "pencil.circle")
        }

        saveStates[scan.filePath] = state
    }

    private func scanFocusedDocument(pid: pid_t, configuration: AutoSaveConfiguration) -> ScanResult? {
        let application = AXUIElementCreateApplication(pid)
        // Electron applications may expose only a shallow accessibility tree
        // until manual accessibility is enabled by an assistive client.
        _ = AXUIElementSetAttributeValue(
            application,
            "AXManualAccessibility" as CFString,
            kCFBooleanTrue
        )
        guard let focusedWindow = copyElementAttribute(application, kAXFocusedWindowAttribute as CFString) else {
            return nil
        }
        guard
            let windowPosition = copyPointAttribute(focusedWindow, kAXPositionAttribute as CFString),
            let windowSize = copySizeAttribute(focusedWindow, kAXSizeAttribute as CFString)
        else {
            return nil
        }
        let windowFrame = CGRect(origin: windowPosition, size: windowSize)
        let isFullScreen = copyBoolAttribute(focusedWindow, "AXFullScreen" as CFString) ?? false

        var stack = [focusedWindow]
        var visited = 0
        var matchedPath: String?
        var accumulatedTexts = [String]()

        while let element = stack.popLast(), visited < 2_500 {
            visited += 1

            if let editorURL = copyStringAttribute(element, kAXURLAttribute as CFString),
               let filePath = XMindDocumentInspector.sourceFilePath(fromEditorURL: editorURL) {
                if matchedPath == nil {
                    matchedPath = filePath
                } else if matchedPath != filePath {
                    continue
                }
            }

            for attribute in [kAXValueAttribute, kAXTitleAttribute, kAXDescriptionAttribute] {
                if let text = copyStringAttribute(element, attribute as CFString), !text.isEmpty {
                    accumulatedTexts.append(text)
                }
            }

            stack.append(contentsOf: copyChildren(element))
        }

        guard let matchedPath else { return nil }
        let activitySignature = accumulatedTexts.joined(separator: "\u{1F}")
        return ScanResult(
            filePath: matchedPath,
            isDirty: XMindDocumentInspector.isDirty(
                texts: accumulatedTexts,
                indicators: configuration.dirtyIndicators
            ),
            windowFrame: windowFrame,
            isFullScreen: isFullScreen,
            activitySignature: activitySignature
        )
    }

    private func postCommandS(to pid: pid_t) {
        let source = CGEventSource(stateID: .combinedSessionState)
        let keyCodeForS: CGKeyCode = 1

        guard
            let keyDown = CGEvent(keyboardEventSource: source, virtualKey: keyCodeForS, keyDown: true),
            let keyUp = CGEvent(keyboardEventSource: source, virtualKey: keyCodeForS, keyDown: false)
        else {
            logger.error("Could not create Command-S event")
            return
        }

        keyDown.flags = .maskCommand
        keyUp.flags = .maskCommand
        keyDown.postToPid(pid)
        keyUp.postToPid(pid)
    }

    private func updateStatus(_ text: String, symbol: String) {
        statusMenuItem.title = "状态：\(text)"
        statusItem.button?.image = NSImage(systemSymbolName: symbol, accessibilityDescription: text)
        statusItem.button?.toolTip = "XMind 自动保存：\(text)"

        guard text != lastStatusText else { return }
        lastStatusText = text
        logger.info("Status: \(text, privacy: .public)")

        let statusDirectory = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first?.appendingPathComponent("XMindAutoSave", isDirectory: true)
        if let statusDirectory {
            try? FileManager.default.createDirectory(
                at: statusDirectory,
                withIntermediateDirectories: true
            )
        }
        let statusURL = statusDirectory?.appendingPathComponent("status.txt")
        let report = "\(Date().ISO8601Format())\n\(text)\n"
        if let statusURL {
            try? report.write(to: statusURL, atomically: true, encoding: .utf8)
        }
    }

    private func copyElementAttribute(_ element: AXUIElement, _ attribute: CFString) -> AXUIElement? {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success,
            let rawValue,
            CFGetTypeID(rawValue) == AXUIElementGetTypeID()
        else {
            return nil
        }
        return unsafeDowncast(rawValue, to: AXUIElement.self)
    }

    private func copyStringAttribute(_ element: AXUIElement, _ attribute: CFString) -> String? {
        var rawValue: CFTypeRef?
        guard AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success else {
            return nil
        }

        if let value = rawValue as? String {
            return value
        }
        if let value = rawValue as? URL {
            return value.absoluteString
        }
        if let value = rawValue as? NSURL {
            return value.absoluteString
        }
        if let value = rawValue as? NSAttributedString {
            return value.string
        }
        return nil
    }

    private func copyPointAttribute(_ element: AXUIElement, _ attribute: CFString) -> CGPoint? {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success,
            let rawValue,
            CFGetTypeID(rawValue) == AXValueGetTypeID()
        else {
            return nil
        }

        let axValue = unsafeDowncast(rawValue, to: AXValue.self)
        var point = CGPoint.zero
        guard AXValueGetType(axValue) == .cgPoint, AXValueGetValue(axValue, .cgPoint, &point) else {
            return nil
        }
        return point
    }

    private func copySizeAttribute(_ element: AXUIElement, _ attribute: CFString) -> CGSize? {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success,
            let rawValue,
            CFGetTypeID(rawValue) == AXValueGetTypeID()
        else {
            return nil
        }

        let axValue = unsafeDowncast(rawValue, to: AXValue.self)
        var size = CGSize.zero
        guard AXValueGetType(axValue) == .cgSize, AXValueGetValue(axValue, .cgSize, &size) else {
            return nil
        }
        return size
    }

    private func copyBoolAttribute(_ element: AXUIElement, _ attribute: CFString) -> Bool? {
        var rawValue: CFTypeRef?
        guard AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success else {
            return nil
        }
        if let value = rawValue as? Bool {
            return value
        }
        if let value = rawValue as? NSNumber {
            return value.boolValue
        }
        return nil
    }

    private func copyChildren(_ element: AXUIElement) -> [AXUIElement] {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, kAXChildrenAttribute as CFString, &rawValue) == .success,
            let children = rawValue as? [AXUIElement]
        else {
            return []
        }
        return children
    }
}

if handleMaintenanceCommand() {
    exit(0)
} else {
    let application = NSApplication.shared
    let delegate = AppDelegate()
    application.delegate = delegate
    application.run()
}
