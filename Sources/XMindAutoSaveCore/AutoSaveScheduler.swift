import Foundation

public enum AutoSaveDeferReason: Equatable, Sendable {
    case recentActivity
    case typingBurst
    case editingText
    case composingIME
    case userBusy
}

public enum AutoSaveStatus: Equatable, Sendable {
    case disabled
    case saved
    case autoSaved(Date)
    case editing(AutoSaveDeferReason)
    case saving
}

public struct AutoSaveDecision: Equatable, Sendable {
    public var shouldSave: Bool
    public var status: AutoSaveStatus

    public init(shouldSave: Bool, status: AutoSaveStatus) {
        self.shouldSave = shouldSave
        self.status = status
    }
}

public struct AutoSaveObservation: Equatable, Sendable {
    public var now: Date
    public var filePath: String
    public var isEnabled: Bool
    public var isDirty: Bool
    public var isFrontmost: Bool
    public var isEditingText: Bool
    public var isComposingText: Bool
    public var isUserBusy: Bool
    public var lastInputAt: Date?
    public var inputGeneration: Int
    public var keyboardInputGeneration: Int
    public var secondsSinceLastFrontmostHIDInput: TimeInterval?
    public var secondsSinceLastFrontmostKeyEvent: TimeInterval?
    public var activitySignature: String
    public var forceSave: Bool

    public init(
        now: Date,
        filePath: String,
        isEnabled: Bool,
        isDirty: Bool,
        isFrontmost: Bool,
        isEditingText: Bool = false,
        isComposingText: Bool = false,
        isUserBusy: Bool = false,
        lastInputAt: Date? = nil,
        inputGeneration: Int = 0,
        keyboardInputGeneration: Int = 0,
        secondsSinceLastFrontmostHIDInput: TimeInterval? = nil,
        secondsSinceLastFrontmostKeyEvent: TimeInterval? = nil,
        activitySignature: String = "",
        forceSave: Bool = false
    ) {
        self.now = now
        self.filePath = filePath
        self.isEnabled = isEnabled
        self.isDirty = isDirty
        self.isFrontmost = isFrontmost
        self.isEditingText = isEditingText
        self.isComposingText = isComposingText
        self.isUserBusy = isUserBusy
        self.lastInputAt = lastInputAt
        self.inputGeneration = inputGeneration
        self.keyboardInputGeneration = keyboardInputGeneration
        self.secondsSinceLastFrontmostHIDInput = secondsSinceLastFrontmostHIDInput
        self.secondsSinceLastFrontmostKeyEvent = secondsSinceLastFrontmostKeyEvent
        self.activitySignature = activitySignature
        self.forceSave = forceSave
    }
}

/// Decides when it is safe and non-intrusive to trigger an automatic save.
public struct AutoSaveScheduler: Sendable {
    public var saveDelay: TimeInterval
    public var typingSaveDelay: TimeInterval
    public var textEditingSaveDelay: TimeInterval
    public var retryInterval: TimeInterval
    public var composingMaxHold: TimeInterval
    public var typingBurstWindow: TimeInterval
    public var backgroundSettleGrace: TimeInterval

    private struct DocumentState: Sendable {
        var dirtySince: Date?
        var lastEditActivityAt: Date?
        var observedInputGeneration = -1
        var observedKeyboardGeneration = -1
        var observedActivitySignature: String?
        var typingBurstCount = 0
        var lastKeyboardInputAt: Date?
        var wasEditingText = false
        var wasFrontmost = false
        var leftFrontmostAt: Date?
        var previousFrontmostPollAt: Date?
        var previousHIDInputIdle: TimeInterval?
        var previousHIDKeyIdle: TimeInterval?
        var lastAttempt: Date?
        var lastSuccessfulSave: Date?
    }

    private var states: [String: DocumentState] = [:]

    public init(configuration: AutoSaveConfiguration) {
        self.saveDelay = max(Double(configuration.saveDelayMilliseconds) / 1_000.0, 0)
        self.typingSaveDelay = max(Double(configuration.typingSaveDelayMilliseconds) / 1_000.0, self.saveDelay)
        self.textEditingSaveDelay = max(Double(configuration.textEditingSaveDelayMilliseconds) / 1_000.0, self.typingSaveDelay)
        self.retryInterval = max(Double(configuration.retryMilliseconds) / 1_000.0, 0.5)
        self.composingMaxHold = 12.0
        self.typingBurstWindow = 4.0
        self.backgroundSettleGrace = 0.6
    }

    public init(
        saveDelay: TimeInterval = 2.0,
        typingSaveDelay: TimeInterval = 3.5,
        textEditingSaveDelay: TimeInterval = 8.0,
        retryInterval: TimeInterval = 2.0,
        composingMaxHold: TimeInterval = 12.0,
        typingBurstWindow: TimeInterval = 4.0,
        backgroundSettleGrace: TimeInterval = 0.6
    ) {
        self.saveDelay = max(saveDelay, 0)
        self.typingSaveDelay = max(typingSaveDelay, self.saveDelay)
        self.textEditingSaveDelay = max(textEditingSaveDelay, self.typingSaveDelay)
        self.retryInterval = max(retryInterval, 0.5)
        self.composingMaxHold = max(composingMaxHold, 0)
        self.typingBurstWindow = max(typingBurstWindow, 0.5)
        self.backgroundSettleGrace = max(backgroundSettleGrace, 0)
    }

    public mutating func forget(filePath: String) {
        states[filePath] = nil
    }

    public mutating func evaluate(_ observation: AutoSaveObservation) -> AutoSaveDecision {
        guard observation.isEnabled else {
            states[observation.filePath] = nil
            return AutoSaveDecision(shouldSave: false, status: .disabled)
        }

        var state = states[observation.filePath] ?? DocumentState()
        let now = observation.now

        guard observation.isDirty else {
            let justSaved = state.lastAttempt != nil && state.dirtySince != nil
            state.dirtySince = nil
            state.lastEditActivityAt = nil
            state.observedInputGeneration = observation.inputGeneration
            state.observedKeyboardGeneration = observation.keyboardInputGeneration
            state.observedActivitySignature = observation.activitySignature
            state.typingBurstCount = 0
            state.lastKeyboardInputAt = nil
            state.wasEditingText = observation.isEditingText
            state.wasFrontmost = observation.isFrontmost
            state.leftFrontmostAt = nil
            state.previousFrontmostPollAt = observation.isFrontmost ? now : nil
            state.previousHIDInputIdle = observation.isFrontmost ? observation.secondsSinceLastFrontmostHIDInput : nil
            state.previousHIDKeyIdle = observation.isFrontmost ? observation.secondsSinceLastFrontmostKeyEvent : nil
            state.lastAttempt = nil
            if justSaved {
                state.lastSuccessfulSave = now
            }
            states[observation.filePath] = state

            if let savedAt = state.lastSuccessfulSave {
                return AutoSaveDecision(shouldSave: false, status: .autoSaved(savedAt))
            }
            return AutoSaveDecision(shouldSave: false, status: .saved)
        }

        if state.dirtySince == nil {
            state.dirtySince = now
        }

        // Track focus transitions so switching away from XMind gives Cmd+Tab a brief settle moment.
        if state.wasFrontmost && !observation.isFrontmost {
            state.leftFrontmostAt = now
        } else if observation.isFrontmost {
            state.leftFrontmostAt = nil
        }
        state.wasFrontmost = observation.isFrontmost

        // Track event-monitor input generations.
        var sawKeyboardInputThisPoll = false
        if state.observedInputGeneration < 0 {
            state.observedInputGeneration = observation.inputGeneration
        } else if state.observedInputGeneration != observation.inputGeneration {
            state.observedInputGeneration = observation.inputGeneration
            let inputTime = observation.lastInputAt ?? now
            advanceActivity(&state, to: min(inputTime, now))
        }

        if state.observedKeyboardGeneration < 0 {
            state.observedKeyboardGeneration = observation.keyboardInputGeneration
        } else if state.observedKeyboardGeneration != observation.keyboardInputGeneration {
            let rawDelta = observation.keyboardInputGeneration &- state.observedKeyboardGeneration
            let delta = max(1, min(rawDelta, 20))
            state.observedKeyboardGeneration = observation.keyboardInputGeneration
            recordKeyboardBurst(&state, count: delta, at: min(observation.lastInputAt ?? now, now))
            sawKeyboardInputThisPoll = true
        }

        // Track hardware HID idle timers while XMind is frontmost. This works even if
        // global NSEvent monitoring missed events or was delayed.
        if observation.isFrontmost {
            let dt = state.previousFrontmostPollAt.map { max(0, now.timeIntervalSince($0)) } ?? 0

            if let hidIdle = observation.secondsSinceLastFrontmostHIDInput {
                let isFreshHIDActivity: Bool
                if let prevIdle = state.previousHIDInputIdle {
                    isFreshHIDActivity = (hidIdle + 0.05) < (prevIdle + dt)
                } else {
                    isFreshHIDActivity = hidIdle < max(saveDelay, 1.0)
                }
                if isFreshHIDActivity {
                    advanceActivity(&state, to: now.addingTimeInterval(-max(0, hidIdle)))
                }
                state.previousHIDInputIdle = hidIdle
            }

            if let keyIdle = observation.secondsSinceLastFrontmostKeyEvent {
                let isFreshKeyActivity: Bool
                if let prevKeyIdle = state.previousHIDKeyIdle {
                    isFreshKeyActivity = (keyIdle + 0.05) < (prevKeyIdle + dt)
                } else {
                    isFreshKeyActivity = keyIdle < max(saveDelay, 1.0)
                }
                if isFreshKeyActivity && !sawKeyboardInputThisPoll {
                    let keyTime = now.addingTimeInterval(-max(0, keyIdle))
                    advanceActivity(&state, to: keyTime)
                    recordKeyboardBurst(&state, count: 1, at: keyTime)
                }
                state.previousHIDKeyIdle = keyIdle
            }

            state.previousFrontmostPollAt = now
        } else {
            state.previousFrontmostPollAt = nil
            state.previousHIDInputIdle = nil
            state.previousHIDKeyIdle = nil
        }

        // Track accessibility text & cursor changes.
        if state.observedActivitySignature == nil {
            state.observedActivitySignature = observation.activitySignature
        } else if state.observedActivitySignature != observation.activitySignature {
            state.observedActivitySignature = observation.activitySignature
            advanceActivity(&state, to: now)
        }

        // Exiting an inline text editor (e.g. pressing Enter/Esc after finishing a topic)
        // is a natural save boundary, so clear any accumulated typing-burst hold.
        if state.wasEditingText && !observation.isEditingText {
            state.typingBurstCount = 0
        }
        state.wasEditingText = observation.isEditingText

        if state.lastEditActivityAt == nil {
            state.lastEditActivityAt = state.dirtySince ?? now
        }

        // Compute effective idle duration since the most recent edit/input.
        let activityIdle = max(0, now.timeIntervalSince(state.lastEditActivityAt ?? now))
        let effectiveIdle: TimeInterval
        if observation.isFrontmost, let hidIdle = observation.secondsSinceLastFrontmostHIDInput {
            effectiveIdle = min(activityIdle, max(0, hidIdle))
        } else {
            effectiveIdle = activityIdle
        }

        let mayRetry = state.lastAttempt.map { now.timeIntervalSince($0) >= retryInterval } ?? true

        if observation.forceSave {
            state.lastAttempt = now
            states[observation.filePath] = state
            return AutoSaveDecision(shouldSave: true, status: .saving)
        }

        if observation.isUserBusy {
            states[observation.filePath] = state
            return AutoSaveDecision(shouldSave: false, status: .editing(.userBusy))
        }

        if observation.isComposingText && effectiveIdle < composingMaxHold {
            states[observation.filePath] = state
            return AutoSaveDecision(shouldSave: false, status: .editing(.composingIME))
        }

        let requiredDelay: TimeInterval
        let deferReason: AutoSaveDeferReason
        if !observation.isFrontmost {
            requiredDelay = saveDelay
            deferReason = .recentActivity
        } else if observation.isEditingText {
            requiredDelay = textEditingSaveDelay
            deferReason = .editingText
        } else if state.typingBurstCount >= 3 {
            requiredDelay = typingSaveDelay
            deferReason = .typingBurst
        } else {
            requiredDelay = saveDelay
            deferReason = .recentActivity
        }

        let settledAfterBackgrounding: Bool
        if !observation.isFrontmost, let leftAt = state.leftFrontmostAt {
            settledAfterBackgrounding = now.timeIntervalSince(leftAt) >= backgroundSettleGrace
        } else {
            settledAfterBackgrounding = true
        }

        let quietEnough = effectiveIdle >= requiredDelay && settledAfterBackgrounding

        if quietEnough && mayRetry {
            state.lastAttempt = now
            states[observation.filePath] = state
            return AutoSaveDecision(shouldSave: true, status: .saving)
        }

        states[observation.filePath] = state
        if quietEnough && state.lastAttempt != nil {
            return AutoSaveDecision(shouldSave: false, status: .saving)
        }
        return AutoSaveDecision(shouldSave: false, status: .editing(deferReason))
    }

    private func advanceActivity(_ state: inout DocumentState, to date: Date) {
        if let existing = state.lastEditActivityAt {
            if date > existing {
                state.lastEditActivityAt = date
            }
        } else {
            state.lastEditActivityAt = date
        }
    }

    private func recordKeyboardBurst(_ state: inout DocumentState, count: Int, at date: Date) {
        if let last = state.lastKeyboardInputAt, date.timeIntervalSince(last) <= typingBurstWindow {
            state.typingBurstCount += count
        } else {
            state.typingBurstCount = count
        }
        if let last = state.lastKeyboardInputAt {
            if date > last {
                state.lastKeyboardInputAt = date
            }
        } else {
            state.lastKeyboardInputAt = date
        }
    }
}
