import AppKit
import ApplicationServices
import Carbon
import CoreGraphics
import Darwin
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
private let injectedEventUserData: Int64 = 0x584D4153 // "XMAS"

private struct ScanResult {
    let filePath: String
    let isDirty: Bool
    let windowFrame: CGRect
    let isFullScreen: Bool
    let isEditingText: Bool
    let hasMarkedText: Bool
    let isInMenuOrSheet: Bool
    let activitySignature: String
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
    private var scheduler: AutoSaveScheduler?
    private var overlayPanel: AttachedTogglePanel?
    private var activeFilePath: String?
    private var didRequestAccessibility = false
    private var wasAccessibilityTrusted = false
    private var lastStatusText = ""
    private var lastLoggedSaveDate: Date?
    private let launchAtLoginController = LaunchAtLoginController()
    private let updateController = UpdateController(xmindBundleIdentifier: xmindBundleIdentifier)
    private var globalEventMonitor: Any?
    private var xmindInputGeneration = 0
    private var xmindKeyboardGeneration = 0
    private var lastXMindInputAt: Date?
    private var letterSinceCommit = false
    private var wasEditingText = false
    private var pidPathCache: [pid_t: String] = [:]

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        configureStatusItem()
        configureLaunchAtLogin()
        updateController.start()

        do {
            let loaded = try loadConfiguration()
            configuration = loaded
            scheduler = AutoSaveScheduler(configuration: loaded)
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
        removeInputActivityMonitor()
        overlayPanel?.hide()
    }

    private func configureStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = NSImage(systemSymbolName: "externaldrive.fill.badge.checkmark", accessibilityDescription: "XMind 自动保存")
            button.toolTip = "XMind 自动保存"
        }

        let menu = NSMenu()
        menu.addItem(updateController.availableItem)
        menu.addItem(updateController.availableSeparator)

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
        menu.addItem(updateController.checkNowItem)
        menu.addItem(updateController.automaticChecksItem)

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
        if AXIsProcessTrusted() {
            wasAccessibilityTrusted = true
            installInputActivityMonitor()
        }
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
        removeInputActivityMonitor()

        let mask: NSEvent.EventTypeMask = [
            .keyDown,
            .keyUp,
            .flagsChanged,
            .leftMouseDown,
            .leftMouseDragged,
            .leftMouseUp,
            .rightMouseDown
        ]

        globalEventMonitor = NSEvent.addGlobalMonitorForEvents(matching: mask) { [weak self] event in
            if event.cgEvent?.getIntegerValueField(.eventSourceUserData) == injectedEventUserData {
                return
            }
            guard NSWorkspace.shared.frontmostApplication?.bundleIdentifier == xmindBundleIdentifier else {
                return
            }

            let handleEvent = { @MainActor [weak self] in
                guard let self else { return }
                self.recordXMindEvent(event)
            }

            if Thread.isMainThread {
                MainActor.assumeIsolated {
                    handleEvent()
                }
            } else {
                Task { @MainActor in
                    handleEvent()
                }
            }
        }
    }

    private func removeInputActivityMonitor() {
        if let globalEventMonitor {
            NSEvent.removeMonitor(globalEventMonitor)
            self.globalEventMonitor = nil
        }
    }

    private func recordXMindEvent(_ event: NSEvent) {
        let now = Date()
        xmindInputGeneration &+= 1
        lastXMindInputAt = now

        switch event.type {
        case .keyDown:
            xmindKeyboardGeneration &+= 1
            let modifiers = event.modifierFlags.intersection([.command, .control, .option])
            if !modifiers.isEmpty {
                letterSinceCommit = false
            } else if isCommitOrCancelKey(event.keyCode) {
                letterSinceCommit = false
            } else if isLetterOrPinyinKey(event) {
                letterSinceCommit = true
            }
        case .leftMouseDown, .leftMouseUp, .rightMouseDown:
            letterSinceCommit = false
        default:
            break
        }
    }

    private func isCommitOrCancelKey(_ keyCode: UInt16) -> Bool {
        switch keyCode {
        case 36, 76, 53, 49, 48: // Return, Enter, Escape, Space, Tab
            return true
        case 18, 19, 20, 21, 23, 22, 26, 28, 25: // Top-row 1-9 (IME candidate selection)
            return true
        case 83, 84, 85, 86, 87, 88, 89, 91, 92: // Numpad 1-9
            return true
        default:
            return false
        }
    }

    private func isLetterOrPinyinKey(_ event: NSEvent) -> Bool {
        guard let chars = event.charactersIgnoringModifiers, let scalar = chars.unicodeScalars.first, chars.unicodeScalars.count == 1 else {
            return false
        }
        return (scalar.value >= 65 && scalar.value <= 90)
            || (scalar.value >= 97 && scalar.value <= 122)
            || scalar == "'"
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
                scheduler?.forget(filePath: filePath)
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
        guard let configuration, let autoSaveStore, scheduler != nil else { return }

        guard AXIsProcessTrusted() else {
            if wasAccessibilityTrusted || globalEventMonitor != nil {
                wasAccessibilityTrusted = false
                removeInputActivityMonitor()
            }
            clearCurrentDocumentUI()
            updateStatus("需要辅助功能权限", symbol: "lock.trianglebadge.exclamationmark")
            return
        }

        if !wasAccessibilityTrusted || globalEventMonitor == nil {
            wasAccessibilityTrusted = true
            installInputActivityMonitor()
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

        if !xmindIsFrontmost || (wasEditingText && !scan.isEditingText) {
            letterSinceCommit = false
        }
        wasEditingText = scan.isEditingText

        // Sample hardware HID idle clocks AFTER scanning the AX tree so any keystroke
        // that landed during the AX walk is immediately visible before deciding to save.
        let now = Date()
        let hidInputIdle: TimeInterval? = xmindIsFrontmost ? secondsSinceLastHIDInput() : nil
        let hidKeyIdle: TimeInterval? = xmindIsFrontmost ? secondsSinceLastHIDKeyEvent() : nil
        let isUserBusy = scan.isInMenuOrSheet || (xmindIsFrontmost && isHardwareInputHeld())
        let isComposingText: Bool
        if xmindIsFrontmost {
            let candidateWindowVisible = hasVisibleIMECandidateWindow(recentHIDInputSeconds: hidInputIdle ?? .infinity)
            let nativeIMELetterPending = letterSinceCommit && isIMEInputModeActive() && (hidKeyIdle ?? .infinity) < 6.0
            isComposingText = scan.hasMarkedText || candidateWindowVisible || nativeIMELetterPending
        } else {
            isComposingText = false
        }

        let observation = AutoSaveObservation(
            now: now,
            filePath: scan.filePath,
            isEnabled: enabled,
            isDirty: scan.isDirty,
            isFrontmost: xmindIsFrontmost,
            isEditingText: scan.isEditingText,
            isComposingText: isComposingText,
            isUserBusy: isUserBusy,
            lastInputAt: lastXMindInputAt,
            inputGeneration: xmindInputGeneration,
            keyboardInputGeneration: xmindKeyboardGeneration,
            secondsSinceLastFrontmostHIDInput: hidInputIdle,
            secondsSinceLastFrontmostKeyEvent: hidKeyIdle,
            activitySignature: scan.activitySignature,
            forceSave: forceSave
        )

        guard var activeScheduler = scheduler else { return }
        let decision = activeScheduler.evaluate(observation)

        if decision.shouldSave {
            if !forceSave && xmindIsFrontmost && (isHardwareInputHeld() || secondsSinceLastHIDInput() < 0.25) {
                updateStatus("正在输入…", symbol: "pencil.circle")
                return
            }
            postCommandS(to: xmind.processIdentifier)
        }
        scheduler = activeScheduler

        switch decision.status {
        case .disabled:
            updateStatus("当前文件自动保存已关闭", symbol: "pause.circle")
        case .saved:
            updateStatus("已保存", symbol: "checkmark.circle")
        case .autoSaved(let savedAt):
            if lastLoggedSaveDate != savedAt {
                lastLoggedSaveDate = savedAt
                logger.info("Saved \(scan.filePath, privacy: .private)")
            }
            updateStatus("已自动保存 \(savedAt.formatted(date: .omitted, time: .standard))", symbol: "checkmark.circle.fill")
        case .saving:
            updateStatus("正在自动保存…", symbol: "arrow.triangle.2.circlepath")
        case .editing(let reason):
            switch reason {
            case .composingIME:
                updateStatus("正在输入（输入法）…", symbol: "pencil.circle")
            case .editingText:
                updateStatus("正在编辑文字…", symbol: "pencil.circle")
            case .typingBurst:
                updateStatus("正在连续输入…", symbol: "pencil.circle")
            case .userBusy:
                updateStatus("等待操作完成…", symbol: "pencil.circle")
            case .recentActivity:
                updateStatus("检测到编辑…", symbol: "pencil.circle")
            }
        }
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

        var isEditingText = false
        var hasMarkedText = false
        var isInMenuOrSheet = !copyElementArrayAttribute(focusedWindow, "AXSheets" as CFString).isEmpty
        var focusedSignatureParts = [String]()

        if let focusedElement = copyElementAttribute(application, kAXFocusedUIElementAttribute as CFString)
            ?? copyElementAttribute(focusedWindow, kAXFocusedUIElementAttribute as CFString) {
            let role = copyStringAttribute(focusedElement, kAXRoleAttribute as CFString)
            let subrole = copyStringAttribute(focusedElement, kAXSubroleAttribute as CFString)
            let valueSettable = isAttributeSettable(focusedElement, kAXValueAttribute as CFString)
            let isEditable = copyBoolAttribute(focusedElement, "AXEditable" as CFString) ?? false
            let elementSize = copySizeAttribute(focusedElement, kAXSizeAttribute as CFString)
            let selectedRange = copyRangeAttribute(focusedElement, kAXSelectedTextRangeAttribute as CFString)
            let markedRange = copyRangeAttribute(focusedElement, "AXMarkedTextRange" as CFString)

            hasMarkedText = (markedRange?.length ?? 0) > 0
            isEditingText = XMindDocumentInspector.isTextEditingElement(
                role: role,
                subrole: subrole,
                isValueSettable: valueSettable,
                isEditable: isEditable,
                hasSelectedTextRange: selectedRange != nil,
                hasMarkedText: hasMarkedText,
                size: elementSize
            )
            if XMindDocumentInspector.isMenuOrModalRole(role) {
                isInMenuOrSheet = true
            }
            if isEditingText {
                if let role {
                    focusedSignatureParts.append("role:\(role)")
                }
                if let focusedValue = copyStringAttribute(focusedElement, kAXValueAttribute as CFString) {
                    focusedSignatureParts.append("val:\(focusedValue)")
                }
                if let selectedRange {
                    focusedSignatureParts.append("sel:\(selectedRange.location):\(selectedRange.length)")
                }
                if let markedRange, markedRange.length > 0 {
                    focusedSignatureParts.append("mark:\(markedRange.location):\(markedRange.length)")
                }
            }
        }

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
        var signatureComponents = accumulatedTexts
        if !focusedSignatureParts.isEmpty {
            signatureComponents.append(contentsOf: focusedSignatureParts)
        }
        let activitySignature = signatureComponents.joined(separator: "\u{1F}")
        return ScanResult(
            filePath: matchedPath,
            isDirty: XMindDocumentInspector.isDirty(
                texts: accumulatedTexts,
                indicators: configuration.dirtyIndicators
            ),
            windowFrame: windowFrame,
            isFullScreen: isFullScreen,
            isEditingText: isEditingText,
            hasMarkedText: hasMarkedText,
            isInMenuOrSheet: isInMenuOrSheet,
            activitySignature: activitySignature
        )
    }

    private func secondsSinceLastHIDInput() -> TimeInterval {
        let eventTypes: [CGEventType] = [
            .keyDown,
            .keyUp,
            .flagsChanged,
            .leftMouseDown,
            .leftMouseDragged,
            .leftMouseUp,
            .rightMouseDown,
            .rightMouseUp
        ]
        return eventTypes.reduce(TimeInterval.infinity) { minimum, eventType in
            let elapsed = CGEventSource.secondsSinceLastEventType(.hidSystemState, eventType: eventType)
            return min(minimum, elapsed)
        }
    }

    private func secondsSinceLastHIDKeyEvent() -> TimeInterval {
        let eventTypes: [CGEventType] = [.keyDown, .keyUp, .flagsChanged]
        return eventTypes.reduce(TimeInterval.infinity) { minimum, eventType in
            let elapsed = CGEventSource.secondsSinceLastEventType(.hidSystemState, eventType: eventType)
            return min(minimum, elapsed)
        }
    }

    private func isHardwareInputHeld() -> Bool {
        let heldModifiers: CGEventFlags = [.maskShift, .maskControl, .maskAlternate, .maskCommand]
        let flags = CGEventSource.flagsState(.hidSystemState)
        if !flags.intersection(heldModifiers).isEmpty {
            return true
        }
        return NSEvent.pressedMouseButtons != 0
    }

    private func isIMEInputModeActive() -> Bool {
        guard
            let source = TISCopyCurrentKeyboardInputSource()?.takeRetainedValue(),
            let rawType = TISGetInputSourceProperty(source, kTISPropertyInputSourceType)
        else {
            return false
        }
        let type = Unmanaged<CFString>.fromOpaque(rawType).takeUnretainedValue() as String
        return type == (kTISTypeKeyboardInputMode as String)
    }

    private func hasVisibleIMECandidateWindow(recentHIDInputSeconds: TimeInterval) -> Bool {
        // Guard against third-party IMEs that keep a permanent floating status bar on screen.
        guard recentHIDInputSeconds < 12.0 else { return false }

        let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
        guard let windowList = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] else {
            return false
        }

        let statusBarLevel = Int(CGWindowLevelForKey(.statusWindow))
        for window in windowList {
            guard
                let layer = window[kCGWindowLayer as String] as? Int,
                layer > 0,
                layer != statusBarLevel,
                let pid = window[kCGWindowOwnerPID as String] as? pid_t
            else {
                continue
            }

            if let alpha = window[kCGWindowAlpha as String] as? Double, alpha <= 0.05 {
                continue
            }

            if let boundsDict = window[kCGWindowBounds as String] as? NSDictionary,
               let bounds = CGRect(dictionaryRepresentation: boundsDict) {
                if bounds.width < 20 || bounds.height < 16 {
                    continue
                }
                if bounds.minY < 40 && bounds.height <= 40 {
                    continue
                }
            }

            if let path = executablePath(for: pid), path.contains("/Input Methods/") {
                return true
            }
        }
        return false
    }

    private func executablePath(for pid: pid_t) -> String? {
        if let cached = pidPathCache[pid] {
            return cached.isEmpty ? nil : cached
        }
        if pidPathCache.count > 128 {
            pidPathCache.removeAll(keepingCapacity: true)
        }

        var buffer = [CChar](repeating: 0, count: 4096)
        let length = proc_pidpath(pid, &buffer, UInt32(buffer.count))
        guard length > 0 else {
            pidPathCache[pid] = ""
            return nil
        }

        let path = buffer.withUnsafeBufferPointer { pointer -> String in
            guard let baseAddress = pointer.baseAddress else { return "" }
            return String(cString: baseAddress)
        }
        pidPathCache[pid] = path
        return path.isEmpty ? nil : path
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
        keyDown.setIntegerValueField(.eventSourceUserData, value: injectedEventUserData)
        keyUp.setIntegerValueField(.eventSourceUserData, value: injectedEventUserData)
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
        copyElementArrayAttribute(element, kAXChildrenAttribute as CFString)
    }

    private func copyElementArrayAttribute(_ element: AXUIElement, _ attribute: CFString) -> [AXUIElement] {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success,
            let children = rawValue as? [AXUIElement]
        else {
            return []
        }
        return children
    }

    private func copyRangeAttribute(_ element: AXUIElement, _ attribute: CFString) -> CFRange? {
        var rawValue: CFTypeRef?
        guard
            AXUIElementCopyAttributeValue(element, attribute, &rawValue) == .success,
            let rawValue,
            CFGetTypeID(rawValue) == AXValueGetTypeID()
        else {
            return nil
        }

        let axValue = unsafeDowncast(rawValue, to: AXValue.self)
        var range = CFRange(location: 0, length: 0)
        guard AXValueGetType(axValue) == .cfRange, AXValueGetValue(axValue, .cfRange, &range) else {
            return nil
        }
        return range
    }

    private func isAttributeSettable(_ element: AXUIElement, _ attribute: CFString) -> Bool {
        var settable = DarwinBoolean(false)
        guard AXUIElementIsAttributeSettable(element, attribute, &settable) == .success else {
            return false
        }
        return settable.boolValue
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
