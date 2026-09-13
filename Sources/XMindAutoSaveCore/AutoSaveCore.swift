import Foundation

public struct AutoSaveConfiguration: Codable, Sendable, Equatable {
    public var monitoredFiles: [String]
    public var pollIntervalMilliseconds: Int
    public var saveDelayMilliseconds: Int
    public var retryMilliseconds: Int
    public var dirtyIndicators: [String]

    public init(
        monitoredFiles: [String],
        pollIntervalMilliseconds: Int = 200,
        saveDelayMilliseconds: Int = 1_200,
        retryMilliseconds: Int = 2_000,
        dirtyIndicators: [String] = ["已编辑", "Edited"]
    ) {
        self.monitoredFiles = monitoredFiles
        self.pollIntervalMilliseconds = pollIntervalMilliseconds
        self.saveDelayMilliseconds = saveDelayMilliseconds
        self.retryMilliseconds = retryMilliseconds
        self.dirtyIndicators = dirtyIndicators
    }

    private enum CodingKeys: String, CodingKey {
        case monitoredFiles
        case pollIntervalMilliseconds
        case saveDelayMilliseconds
        case retryMilliseconds
        case dirtyIndicators
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        monitoredFiles = try container.decode([String].self, forKey: .monitoredFiles)
        pollIntervalMilliseconds = try container.decodeIfPresent(Int.self, forKey: .pollIntervalMilliseconds) ?? 200
        saveDelayMilliseconds = try container.decodeIfPresent(Int.self, forKey: .saveDelayMilliseconds) ?? 1_200
        retryMilliseconds = try container.decodeIfPresent(Int.self, forKey: .retryMilliseconds) ?? 2_000
        dirtyIndicators = try container.decodeIfPresent([String].self, forKey: .dirtyIndicators) ?? ["已编辑", "Edited"]
    }
}

public enum XMindDocumentInspector {
    public static func normalizedPath(_ rawPath: String) -> String {
        let expanded = NSString(string: rawPath).expandingTildeInPath
        return URL(fileURLWithPath: expanded)
            .standardizedFileURL
            .resolvingSymlinksInPath()
            .path
    }

    public static func sourceFilePath(fromEditorURL rawURL: String) -> String? {
        guard
            let components = URLComponents(string: rawURL),
            let source = components.queryItems?.first(where: { $0.name == "source" })?.value,
            let sourceURL = URL(string: source),
            sourceURL.isFileURL
        else {
            return nil
        }

        return normalizedPath(sourceURL.path)
    }

    public static func isDirty(texts: [String], indicators: [String]) -> Bool {
        texts.contains { text in
            indicators.contains { indicator in
                !indicator.isEmpty && text.localizedCaseInsensitiveContains(indicator)
            }
        }
    }
}
