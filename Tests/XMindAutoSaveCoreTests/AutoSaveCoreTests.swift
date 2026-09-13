import Foundation
import Testing
@testable import XMindAutoSaveCore

@Test("解析 XMind 编辑器 URL 中的本地文件")
func parsesSourceFilePath() {
    let raw = "file:///Applications/Xmind.app/editor.html?source=file%3A%2F%2F%2FUsers%2Fdemo%2FDocuments%2F%25E7%25A4%25BA%25E4%25BE%258B.xmind&isDirty=false"
    let expected = "/Users/demo/Documents/示例.xmind"
    #expect(XMindDocumentInspector.sourceFilePath(fromEditorURL: raw) == expected)
}

@Test("拒绝没有本地 source 的 URL")
func rejectsMissingSource() {
    #expect(XMindDocumentInspector.sourceFilePath(fromEditorURL: "https://example.com/editor") == nil)
}

@Test("识别中英文未保存标记")
func detectsDirtyIndicators() {
    #expect(XMindDocumentInspector.isDirty(texts: ["示例脑图 已编辑"], indicators: ["已编辑", "Edited"]))
    #expect(XMindDocumentInspector.isDirty(texts: ["Map Edited"], indicators: ["已编辑", "Edited"]))
    #expect(!XMindDocumentInspector.isDirty(texts: ["示例脑图"], indicators: ["已编辑", "Edited"]))
}

@Test("缺省配置使用短延迟")
func decodesDefaults() throws {
    let data = #"{"monitoredFiles":["/tmp/test.xmind"]}"#.data(using: .utf8)!
    let configuration = try JSONDecoder().decode(AutoSaveConfiguration.self, from: data)
    #expect(configuration.pollIntervalMilliseconds == 200)
    #expect(configuration.saveDelayMilliseconds == 1_200)
    #expect(configuration.retryMilliseconds == 2_000)
}

@Test("每个文件的开关会持久化，并在重命名后保留")
func persistsPerFilePreferenceAcrossRename() throws {
    let root = FileManager.default.temporaryDirectory
        .appendingPathComponent("xmind-autosave-\(UUID().uuidString)", isDirectory: true)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: root) }

    let original = root.appendingPathComponent("original.xmind")
    let renamed = root.appendingPathComponent("renamed.xmind")
    let settings = root.appendingPathComponent("preferences.json")
    try Data("test".utf8).write(to: original)

    let firstStore = PerFileAutoSaveStore(storageURL: settings)
    #expect(!firstStore.isEnabled(forPath: original.path))
    try firstStore.setEnabled(true, forPath: original.path)

    try FileManager.default.moveItem(at: original, to: renamed)
    let reopenedStore = PerFileAutoSaveStore(storageURL: settings)
    #expect(reopenedStore.isEnabled(forPath: renamed.path))

    try reopenedStore.setEnabled(false, forPath: renamed.path)
    let finalStore = PerFileAutoSaveStore(storageURL: settings)
    #expect(!finalStore.isEnabled(forPath: renamed.path))
}
