import CoreGraphics
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

@Test("缺省配置使用分级防打扰延迟")
func decodesDefaults() throws {
    let data = #"{"monitoredFiles":["/tmp/test.xmind"]}"#.data(using: .utf8)!
    let configuration = try JSONDecoder().decode(AutoSaveConfiguration.self, from: data)
    #expect(configuration.pollIntervalMilliseconds == 200)
    #expect(configuration.saveDelayMilliseconds == 2_000)
    #expect(configuration.typingSaveDelayMilliseconds == 3_500)
    #expect(configuration.textEditingSaveDelayMilliseconds == 8_000)
    #expect(configuration.retryMilliseconds == 2_000)
}

@Test("准确区分可见文本编辑器与非文本或隐藏控件")
func classifiesTextEditingElements() {
    #expect(XMindDocumentInspector.isTextEditingElement(
        role: "AXTextArea",
        subrole: nil,
        isValueSettable: true,
        isEditable: true,
        hasSelectedTextRange: true,
        hasMarkedText: false,
        size: CGSize(width: 140, height: 24)
    ))
    #expect(XMindDocumentInspector.isTextEditingElement(
        role: "AXGroup",
        subrole: nil,
        isValueSettable: true,
        isEditable: false,
        hasSelectedTextRange: true,
        hasMarkedText: false,
        size: CGSize(width: 120, height: 20)
    ))
    // 隐藏的 1x1 键盘捕获框不算正在编辑可见文字
    #expect(!XMindDocumentInspector.isTextEditingElement(
        role: "AXTextArea",
        subrole: nil,
        isValueSettable: true,
        isEditable: true,
        hasSelectedTextRange: true,
        hasMarkedText: false,
        size: CGSize(width: 1, height: 1)
    ))
    // 但一旦带有输入法 marked text，无论尺寸如何都算正在输入
    #expect(XMindDocumentInspector.isTextEditingElement(
        role: "AXTextArea",
        subrole: nil,
        isValueSettable: true,
        isEditable: true,
        hasSelectedTextRange: true,
        hasMarkedText: true,
        size: CGSize(width: 1, height: 1)
    ))
    // 复选框、滑块虽然 AXValue 可写，但不属于文本编辑
    #expect(!XMindDocumentInspector.isTextEditingElement(
        role: "AXCheckBox",
        subrole: nil,
        isValueSettable: true,
        isEditable: false,
        hasSelectedTextRange: false,
        hasMarkedText: false,
        size: CGSize(width: 20, height: 20)
    ))
    #expect(XMindDocumentInspector.isMenuOrModalRole("AXMenu"))
    #expect(XMindDocumentInspector.isMenuOrModalRole("AXSheet"))
    #expect(!XMindDocumentInspector.isMenuOrModalRole("AXWebArea"))
}

@Test("在文本框内编辑时延长等待，退出文本框后迅速保存")
func defersWhileEditingTextAndSavesAfterExitingEditor() {
    var scheduler = AutoSaveScheduler(
        saveDelay: 2.0,
        typingSaveDelay: 3.5,
        textEditingSaveDelay: 8.0,
        retryInterval: 2.0
    )
    let start = Date(timeIntervalSince1970: 1_000)
    let path = "/tmp/map.xmind"

    func poll(
        at offset: TimeInterval,
        isDirty: Bool = true,
        isEditingText: Bool = true,
        hidIdle: TimeInterval? = nil,
        keyIdle: TimeInterval? = nil,
        inputGen: Int = 0,
        keyGen: Int = 0
    ) -> AutoSaveDecision {
        scheduler.evaluate(AutoSaveObservation(
            now: start.addingTimeInterval(offset),
            filePath: path,
            isEnabled: true,
            isDirty: isDirty,
            isFrontmost: true,
            isEditingText: isEditingText,
            lastInputAt: start.addingTimeInterval(offset),
            inputGeneration: inputGen,
            keyboardInputGeneration: keyGen,
            secondsSinceLastFrontmostHIDInput: hidIdle,
            secondsSinceLastFrontmostKeyEvent: keyIdle,
            activitySignature: "doc"
        ))
    }

    // 开始在节点文本框里打字
    #expect(poll(at: 0.0, isEditingText: true, hidIdle: 0.05, keyIdle: 0.05, inputGen: 1, keyGen: 1)
        == AutoSaveDecision(shouldSave: false, status: .editing(.editingText)))
    // 停顿思考 4 秒，仍然在文本框内：绝不抢着保存
    #expect(poll(at: 4.0, isEditingText: true, hidIdle: 4.0, keyIdle: 4.0, inputGen: 1, keyGen: 1)
        == AutoSaveDecision(shouldSave: false, status: .editing(.editingText)))
    // 停顿思考 7.5 秒，依然等待
    #expect(poll(at: 7.5, isEditingText: true, hidIdle: 7.5, keyIdle: 7.5, inputGen: 1, keyGen: 1)
        == AutoSaveDecision(shouldSave: false, status: .editing(.editingText)))

    // 用户按回车退出节点文本框（isEditingText 变为 false）
    #expect(poll(at: 7.6, isEditingText: false, hidIdle: 0.02, keyIdle: 0.02, inputGen: 2, keyGen: 2)
        == AutoSaveDecision(shouldSave: false, status: .editing(.recentActivity)))
    // 退出后安静满 2.0 秒，立即平滑保存
    #expect(poll(at: 9.6, isEditingText: false, hidIdle: 2.02, keyIdle: 2.02, inputGen: 2, keyGen: 2)
        == AutoSaveDecision(shouldSave: true, status: .saving))

    // 保存成功，脏标记消失
    let confirmed = poll(at: 9.8, isDirty: false, isEditingText: false, hidIdle: 2.22, keyIdle: 2.22, inputGen: 2, keyGen: 2)
    #expect(confirmed == AutoSaveDecision(shouldSave: false, status: .autoSaved(start.addingTimeInterval(9.8))))
}

@Test("留在文本框内长时间不动时，兜底超时后仍会保存")
func savesAfterLongIdleInsideTextEditor() {
    var scheduler = AutoSaveScheduler(
        saveDelay: 2.0,
        typingSaveDelay: 3.5,
        textEditingSaveDelay: 8.0,
        retryInterval: 2.0
    )
    let start = Date(timeIntervalSince1970: 2_000)
    let path = "/tmp/notes.xmind"

    _ = scheduler.evaluate(AutoSaveObservation(
        now: start,
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: true,
        secondsSinceLastFrontmostHIDInput: 0.1,
        secondsSinceLastFrontmostKeyEvent: 0.1
    ))

    let decision = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(8.2),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: true,
        secondsSinceLastFrontmostHIDInput: 8.2,
        secondsSinceLastFrontmostKeyEvent: 8.2
    ))
    #expect(decision == AutoSaveDecision(shouldSave: true, status: .saving))
}

@Test("即使全局键盘监听未触发，仅凭硬件 HID 计时也能识别连续打字并延长延迟")
func usesHardwareHIDToDetectTypingBurst() {
    var scheduler = AutoSaveScheduler(
        saveDelay: 2.0,
        typingSaveDelay: 3.5,
        textEditingSaveDelay: 8.0,
        retryInterval: 2.0
    )
    let start = Date(timeIntervalSince1970: 3_000)
    let path = "/tmp/burst.xmind"

    // 模拟三次物理按键（inputGeneration 始终为 0，模拟全局监听未生效）
    for (offset, hidIdle) in [(0.0, 0.05), (0.4, 0.05), (0.8, 0.05), (1.2, 0.05)] {
        let decision = scheduler.evaluate(AutoSaveObservation(
            now: start.addingTimeInterval(offset),
            filePath: path,
            isEnabled: true,
            isDirty: true,
            isFrontmost: true,
            isEditingText: false,
            inputGeneration: 0,
            keyboardInputGeneration: 0,
            secondsSinceLastFrontmostHIDInput: hidIdle,
            secondsSinceLastFrontmostKeyEvent: hidIdle
        ))
        #expect(!decision.shouldSave)
    }

    // 停顿 2.2 秒：普通延迟已过，但仍在连续输入档（3.5s），不抢着保存
    let atTwoPointTwo = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(3.4),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: false,
        secondsSinceLastFrontmostHIDInput: 2.25,
        secondsSinceLastFrontmostKeyEvent: 2.25
    ))
    #expect(atTwoPointTwo == AutoSaveDecision(shouldSave: false, status: .editing(.typingBurst)))

    // 停顿 3.6 秒后安全保存
    let atThreePointSix = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(4.8),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: false,
        secondsSinceLastFrontmostHIDInput: 3.65,
        secondsSinceLastFrontmostKeyEvent: 3.65
    ))
    #expect(atThreePointSix == AutoSaveDecision(shouldSave: true, status: .saving))
}

@Test("输入法组字和按键按住期间绝不保存，结束后再保存")
func defersDuringIMECompositionAndBusyState() {
    var scheduler = AutoSaveScheduler(
        saveDelay: 2.0,
        typingSaveDelay: 3.5,
        textEditingSaveDelay: 8.0,
        retryInterval: 2.0,
        composingMaxHold: 12.0
    )
    let start = Date(timeIntervalSince1970: 4_000)
    let path = "/tmp/ime.xmind"

    _ = scheduler.evaluate(AutoSaveObservation(
        now: start,
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isComposingText: true,
        secondsSinceLastFrontmostHIDInput: 0.1,
        secondsSinceLastFrontmostKeyEvent: 0.1
    ))

    // 停顿 5 秒挑候选词：不保存
    let composing = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(5.0),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isComposingText: true,
        secondsSinceLastFrontmostHIDInput: 5.0,
        secondsSinceLastFrontmostKeyEvent: 5.0
    ))
    #expect(composing == AutoSaveDecision(shouldSave: false, status: .editing(.composingIME)))

    // 上屏了，但鼠标/修饰键正按住：仍不保存
    let busy = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(5.2),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isComposingText: false,
        isUserBusy: true,
        secondsSinceLastFrontmostHIDInput: 5.2,
        secondsSinceLastFrontmostKeyEvent: 5.2
    ))
    #expect(busy == AutoSaveDecision(shouldSave: false, status: .editing(.userBusy)))

    // 操作结束：立刻可以保存
    let ready = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(5.4),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isComposingText: false,
        isUserBusy: false,
        secondsSinceLastFrontmostHIDInput: 5.4,
        secondsSinceLastFrontmostKeyEvent: 5.4
    ))
    #expect(ready == AutoSaveDecision(shouldSave: true, status: .saving))
}

@Test("切到其他软件后不受前台打字阻塞，短暂缓冲后后台保存")
func savesInBackgroundAfterFocusLossGrace() {
    var scheduler = AutoSaveScheduler(
        saveDelay: 2.0,
        typingSaveDelay: 3.5,
        textEditingSaveDelay: 8.0,
        retryInterval: 2.0,
        backgroundSettleGrace: 0.6
    )
    let start = Date(timeIntervalSince1970: 5_000)
    let path = "/tmp/bg.xmind"

    _ = scheduler.evaluate(AutoSaveObservation(
        now: start,
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: true,
        secondsSinceLastFrontmostHIDInput: 0.1,
        secondsSinceLastFrontmostKeyEvent: 0.1
    ))

    // 3 秒后切走（比如切到浏览器），刚切走时给 0.6 秒缓冲避免撞上 Cmd+Tab
    let justLeft = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(3.0),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: false,
        isEditingText: true
    ))
    #expect(justLeft == AutoSaveDecision(shouldSave: false, status: .editing(.recentActivity)))

    // 切走 0.7 秒后，后台静默保存
    let settled = scheduler.evaluate(AutoSaveObservation(
        now: start.addingTimeInterval(3.7),
        filePath: path,
        isEnabled: true,
        isDirty: true,
        isFrontmost: false,
        isEditingText: true
    ))
    #expect(settled == AutoSaveDecision(shouldSave: true, status: .saving))
}

@Test("手动立即保存不受编辑和输入法状态阻塞")
func forceSaveBypassesDeferrals() {
    var scheduler = AutoSaveScheduler()
    let now = Date(timeIntervalSince1970: 6_000)
    let decision = scheduler.evaluate(AutoSaveObservation(
        now: now,
        filePath: "/tmp/force.xmind",
        isEnabled: true,
        isDirty: true,
        isFrontmost: true,
        isEditingText: true,
        isComposingText: true,
        isUserBusy: true,
        secondsSinceLastFrontmostHIDInput: 0.01,
        secondsSinceLastFrontmostKeyEvent: 0.01,
        forceSave: true
    ))
    #expect(decision == AutoSaveDecision(shouldSave: true, status: .saving))
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
