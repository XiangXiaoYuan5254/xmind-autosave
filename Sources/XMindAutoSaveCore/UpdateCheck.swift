import Foundation

/// A release number such as 1.0.3.
public struct AppVersion: Comparable, Sendable, CustomStringConvertible {
    public let components: [Int]

    /// Reads "1.0.3" or "v1.0.3". Anything else — a development build, a
    /// pre-release tag — is not a version, so it never looks for updates.
    public init?(_ text: String) {
        var text = text.trimmingCharacters(in: .whitespaces)
        if text.hasPrefix("v") || text.hasPrefix("V") {
            text.removeFirst()
        }
        let parts = text.split(separator: ".", omittingEmptySubsequences: false)
        guard (1...4).contains(parts.count) else { return nil }

        var components = [Int]()
        for part in parts {
            guard !part.isEmpty, part.allSatisfy(\.isASCIIDigit), let number = Int(part) else {
                return nil
            }
            components.append(number)
        }
        self.components = components
    }

    public var description: String {
        components.map(String.init).joined(separator: ".")
    }

    /// Missing parts count as 0, so 1.1 equals 1.1.0.
    public static func < (lhs: AppVersion, rhs: AppVersion) -> Bool {
        let count = max(lhs.components.count, rhs.components.count)
        for index in 0..<count {
            let left = index < lhs.components.count ? lhs.components[index] : 0
            let right = index < rhs.components.count ? rhs.components[index] : 0
            if left != right {
                return left < right
            }
        }
        return false
    }

    public static func == (lhs: AppVersion, rhs: AppVersion) -> Bool {
        !(lhs < rhs) && !(rhs < lhs)
    }
}

/// A newer release that has a macOS installer.
public struct AvailableUpdate: Equatable, Sendable {
    public let version: AppVersion
    public let pageURL: URL
}

/// Update checks read latest.json on the website (helloxxy.com), for example
/// {"version": "1.0.5", "files": ["XMindAutoSave-1.0.5.dmg", "XMindAutoSave-Setup-1.0.5.exe"]}.
/// They read only its version number and file names; nothing is uploaded.
public enum UpdateCheck {
    public static let latestReleaseURL = URL(string: "https://helloxxy.com/works/xmind-autosave/downloads/latest.json")!
    public static let downloadPage = URL(string: "https://helloxxy.com/works/xmind-autosave/#download")!

    private struct LatestRelease: Decodable {
        let version: String
        let files: [String]?
    }

    public struct UnexpectedResponse: LocalizedError {
        public let errorDescription: String?
    }

    /// Reads latest.json and returns the release when it is newer than
    /// `currentVersion` and ships a DMG — a Windows-only release is not
    /// offered here.
    public static func availableUpdate(inLatestRelease data: Data, currentVersion: AppVersion) throws -> AvailableUpdate? {
        let release = try JSONDecoder().decode(LatestRelease.self, from: data)
        guard let version = AppVersion(release.version) else {
            throw UnexpectedResponse(errorDescription: "无法识别的版本号“\(release.version)”")
        }
        guard version > currentVersion,
              (release.files ?? []).contains(where: isMacInstaller)
        else {
            return nil
        }
        return AvailableUpdate(version: version, pageURL: downloadPage)
    }

    /// Matches the DMG that package_release.sh produces.
    public static func isMacInstaller(_ name: String) -> Bool {
        let lowercased = name.lowercased()
        return lowercased.hasPrefix("xmindautosave-") && lowercased.hasSuffix(".dmg")
    }

    public static func fetchAvailableUpdate(
        currentVersion: AppVersion,
        session: URLSession,
        url: URL = latestReleaseURL
    ) async throws -> AvailableUpdate? {
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 20)
        request.setValue("XMindAutoSave/\(currentVersion) (macOS)", forHTTPHeaderField: "User-Agent")

        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse, response.statusCode == 200 else {
            let status = (response as? HTTPURLResponse)?.statusCode ?? 0
            throw UnexpectedResponse(errorDescription: "服务器返回 \(status)")
        }
        return try availableUpdate(inLatestRelease: data, currentVersion: currentVersion)
    }
}

private extension Character {
    var isASCIIDigit: Bool {
        ("0"..."9").contains(self)
    }
}
