import Foundation
import ServiceManagement

@MainActor
final class LaunchAtLoginController {
    enum Status: String {
        case disabled
        case enabled
        case requiresApproval = "requires-approval"
        case unavailable
    }

    private let preferenceKey = "launchAtLoginEnabled"
    private let label = "local.xmind.autosave"
    private let fileManager = FileManager.default
    private let defaults = UserDefaults.standard

    var isInstalledInApplicationsFolder: Bool {
        let bundlePath = Bundle.main.bundleURL.standardizedFileURL.path
        let userApplicationsPath = URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true)
            .appendingPathComponent("Applications", isDirectory: true)
            .path
        return bundlePath.hasPrefix("/Applications/")
            || bundlePath.hasPrefix(userApplicationsPath + "/")
    }

    var status: Status {
        switch SMAppService.mainApp.status {
        case .enabled:
            return .enabled
        case .requiresApproval:
            return .requiresApproval
        case .notFound, .notRegistered:
            break
        @unknown default:
            break
        }

        guard fileManager.fileExists(atPath: launchAgentURL.path) else {
            return isInstalledInApplicationsFolder ? .disabled : .unavailable
        }

        switch SMAppService.statusForLegacyPlist(at: launchAgentURL) {
        case .enabled:
            return .enabled
        case .requiresApproval:
            return .requiresApproval
        case .notFound, .notRegistered:
            // The file still represents the user's requested setting even when
            // Background Task Management has not refreshed its status yet.
            return .enabled
        @unknown default:
            return .enabled
        }
    }

    func configureDefault() throws {
        guard isInstalledInApplicationsFolder else { return }

        if defaults.object(forKey: preferenceKey) == nil {
            defaults.set(true, forKey: preferenceKey)
        }
        if defaults.bool(forKey: preferenceKey) {
            try setEnabled(true, rememberChoice: false)
        }
    }

    func setEnabled(_ enabled: Bool, rememberChoice: Bool = true) throws {
        guard isInstalledInApplicationsFolder else { return }
        if rememberChoice {
            defaults.set(enabled, forKey: preferenceKey)
        }

        if enabled {
            try enable()
        } else {
            try disable()
        }
    }

    private func enable() throws {
        switch SMAppService.mainApp.status {
        case .enabled:
            try removeLegacyLaunchAgentIfPresent()
            return
        case .requiresApproval:
            return
        case .notRegistered:
            do {
                try SMAppService.mainApp.register()
                try removeLegacyLaunchAgentIfPresent()
                return
            } catch {
                if SMAppService.mainApp.status != .notFound {
                    throw error
                }
            }
        case .notFound:
            break
        @unknown default:
            break
        }

        if legacyLaunchAgentTargetsCurrentBundle, status == .enabled {
            return
        }
        try installLegacyLaunchAgent()
    }

    private func disable() throws {
        if SMAppService.mainApp.status == .enabled
            || SMAppService.mainApp.status == .requiresApproval {
            try? SMAppService.mainApp.unregister()
        }
        try removeLegacyLaunchAgentIfPresent()
    }

    private var launchAgentURL: URL {
        URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true)
            .appendingPathComponent("Library/LaunchAgents", isDirectory: true)
            .appendingPathComponent("\(label).plist")
    }

    private var legacyLaunchAgentTargetsCurrentBundle: Bool {
        guard
            let data = try? Data(contentsOf: launchAgentURL),
            let propertyList = try? PropertyListSerialization.propertyList(from: data, format: nil),
            let dictionary = propertyList as? [String: Any],
            let arguments = dictionary["ProgramArguments"] as? [String]
        else {
            return false
        }
        return arguments.contains(Bundle.main.bundleURL.standardizedFileURL.path)
    }

    private func installLegacyLaunchAgent() throws {
        try fileManager.createDirectory(
            at: launchAgentURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        try? runLaunchctl(["bootout", launchDomain, launchAgentURL.path])

        let propertyList: [String: Any] = [
            "Label": label,
            "ProgramArguments": [
                "/usr/bin/open",
                Bundle.main.bundleURL.standardizedFileURL.path
            ],
            "RunAtLoad": true
        ]
        let data = try PropertyListSerialization.data(
            fromPropertyList: propertyList,
            format: .xml,
            options: 0
        )
        try data.write(to: launchAgentURL, options: .atomic)
        try runLaunchctl(["bootstrap", launchDomain, launchAgentURL.path])
    }

    private func removeLegacyLaunchAgentIfPresent() throws {
        guard fileManager.fileExists(atPath: launchAgentURL.path) else { return }
        try? runLaunchctl(["bootout", launchDomain, launchAgentURL.path])
        try fileManager.removeItem(at: launchAgentURL)
    }

    private var launchDomain: String {
        "gui/\(getuid())"
    }

    private func runLaunchctl(_ arguments: [String]) throws {
        let process = Process()
        let errorPipe = Pipe()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = arguments
        process.standardOutput = FileHandle.nullDevice
        process.standardError = errorPipe
        try process.run()
        process.waitUntilExit()

        guard process.terminationStatus == 0 else {
            let data = errorPipe.fileHandleForReading.readDataToEndOfFile()
            let message = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
            throw NSError(
                domain: "XMindAutoSave.LaunchAtLogin",
                code: Int(process.terminationStatus),
                userInfo: [NSLocalizedDescriptionKey: message ?? "launchctl 执行失败"]
            )
        }
    }
}
