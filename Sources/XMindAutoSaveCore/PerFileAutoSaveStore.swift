import Foundation

public struct FileAutoSavePreference: Codable, Sendable, Equatable {
    public var enabled: Bool
    public var lastKnownPath: String

    public init(enabled: Bool, lastKnownPath: String) {
        self.enabled = enabled
        self.lastKnownPath = lastKnownPath
    }
}

public struct StoredAutoSavePreferences: Codable, Sendable, Equatable {
    public var files: [String: FileAutoSavePreference]

    public init(files: [String: FileAutoSavePreference] = [:]) {
        self.files = files
    }
}

public final class PerFileAutoSaveStore {
    private let storageURL: URL
    private let initiallyEnabledKeys: Set<String>
    private var preferences: StoredAutoSavePreferences

    public init(storageURL: URL, initiallyEnabledPaths: [String] = []) {
        self.storageURL = storageURL
        initiallyEnabledKeys = Set(initiallyEnabledPaths.map(Self.preferenceKey(forPath:)))

        if let data = try? Data(contentsOf: storageURL),
           let decoded = try? JSONDecoder().decode(StoredAutoSavePreferences.self, from: data) {
            preferences = decoded
        } else {
            preferences = StoredAutoSavePreferences()
        }
    }

    public func isEnabled(forPath rawPath: String) -> Bool {
        let key = Self.preferenceKey(forPath: rawPath)
        if let stored = preferences.files[key] {
            return stored.enabled
        }
        return initiallyEnabledKeys.contains(key)
    }

    public func setEnabled(_ enabled: Bool, forPath rawPath: String) throws {
        let normalizedPath = XMindDocumentInspector.normalizedPath(rawPath)
        let key = Self.preferenceKey(forPath: normalizedPath)
        preferences.files[key] = FileAutoSavePreference(
            enabled: enabled,
            lastKnownPath: normalizedPath
        )

        try FileManager.default.createDirectory(
            at: storageURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(preferences).write(to: storageURL, options: .atomic)
    }

    public static func preferenceKey(forPath rawPath: String) -> String {
        let normalizedPath = XMindDocumentInspector.normalizedPath(rawPath)
        if let attributes = try? FileManager.default.attributesOfItem(atPath: normalizedPath),
           let device = attributes[.systemNumber] as? NSNumber,
           let inode = attributes[.systemFileNumber] as? NSNumber {
            return "file:\(device.uint64Value):\(inode.uint64Value)"
        }
        return "path:\(normalizedPath)"
    }
}
