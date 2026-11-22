import CoreGraphics
import Foundation

public struct AutoSaveConfiguration: Codable, Sendable, Equatable {
    public var monitoredFiles: [String]
    public var pollIntervalMilliseconds: Int
    public var saveDelayMilliseconds: Int
    public var typingSaveDelayMilliseconds: Int
    public var textEditingSaveDelayMilliseconds: Int
    public var retryMilliseconds: Int
    public var dirtyIndicators: [String]

    public init(
        monitoredFiles: [String],
        pollIntervalMilliseconds: Int = 200,
        saveDelayMilliseconds: Int = 2_000,
        typingSaveDelayMilliseconds: Int = 3_500,
        textEditingSaveDelayMilliseconds: Int = 8_000,
        retryMilliseconds: Int = 2_000,
        dirtyIndicators: [String] = ["已编辑", "Edited"]
    ) {
        self.monitoredFiles = monitoredFiles
        self.pollIntervalMilliseconds = pollIntervalMilliseconds
        self.saveDelayMilliseconds = saveDelayMilliseconds
        self.typingSaveDelayMilliseconds = typingSaveDelayMilliseconds
        self.textEditingSaveDelayMilliseconds = textEditingSaveDelayMilliseconds
        self.retryMilliseconds = retryMilliseconds
        self.dirtyIndicators = dirtyIndicators
    }

    private enum CodingKeys: String, CodingKey {
        case monitoredFiles
        case pollIntervalMilliseconds
        case saveDelayMilliseconds
        case typingSaveDelayMilliseconds
        case textEditingSaveDelayMilliseconds
        case retryMilliseconds
        case dirtyIndicators
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        monitoredFiles = try container.decode([String].self, forKey: .monitoredFiles)
        pollIntervalMilliseconds = try container.decodeIfPresent(Int.self, forKey: .pollIntervalMilliseconds) ?? 200
        saveDelayMilliseconds = try container.decodeIfPresent(Int.self, forKey: .saveDelayMilliseconds) ?? 2_000
        typingSaveDelayMilliseconds = try container.decodeIfPresent(Int.self, forKey: .typingSaveDelayMilliseconds) ?? 3_500
        textEditingSaveDelayMilliseconds = try container.decodeIfPresent(Int.self, forKey: .textEditingSaveDelayMilliseconds) ?? 8_000
        retryMilliseconds = try container.decodeIfPresent(Int.self, forKey: .retryMilliseconds) ?? 2_000
        dirtyIndicators = try container.decodeIfPresent([String].self, forKey: .dirtyIndicators) ?? ["已编辑", "Edited"]
    }
}

public enum XMindDocumentInspector {
    private static let textEditingRoles: Set<String> = [
        "AXTextArea",
        "AXTextField",
        "AXComboBox"
    ]

    private static let editableContainerRoles: Set<String> = [
        "AXGroup",
        "AXStaticText",
        "AXWebArea"
    ]

    private static let menuOrModalRoles: Set<String> = [
        "AXMenu",
        "AXMenuItem",
        "AXMenuBar",
        "AXMenuBarItem",
        "AXSheet",
        "AXPopover"
    ]

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

    /// Classifies whether an Accessibility focused element represents an active,
    /// visible text editor (such as an inline XMind topic editor, note field, or search input).
    public static func isTextEditingElement(
        role: String?,
        subrole: String?,
        isValueSettable: Bool,
        isEditable: Bool,
        hasSelectedTextRange: Bool,
        hasMarkedText: Bool,
        size: CGSize?
    ) -> Bool {
        if hasMarkedText {
            return true
        }

        let matchesRole: Bool
        if let role, textEditingRoles.contains(role) {
            matchesRole = true
        } else if subrole == "AXSearchField" {
            matchesRole = true
        } else if let role, editableContainerRoles.contains(role) {
            matchesRole = (isValueSettable || isEditable) && hasSelectedTextRange
        } else {
            matchesRole = false
        }

        guard matchesRole else { return false }

        // Canvas apps sometimes keep a tiny hidden textarea focused to catch shortcuts.
        if let size, size.width < 4 || size.height < 8 {
            return false
        }

        return true
    }

    public static func isMenuOrModalRole(_ role: String?) -> Bool {
        guard let role else { return false }
        return menuOrModalRoles.contains(role)
    }
}
