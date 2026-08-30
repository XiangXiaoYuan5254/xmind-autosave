import AppKit
import CoreGraphics

@MainActor
private final class ColorStateSwitch: NSControl {
    private static let onTrackColor = NSColor(name: nil) { appearance in
        if appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua {
            return NSColor(srgbRed: 0.42, green: 0.64, blue: 0.54, alpha: 1)
        }

        return NSColor(srgbRed: 0.38, green: 0.56, blue: 0.48, alpha: 1)
    }

    var isOn = false {
        didSet {
            guard oldValue != isOn else { return }
            setAccessibilityValue(isOn ? 1 : 0)
            needsDisplay = true
        }
    }

    override var intrinsicContentSize: NSSize {
        NSSize(width: 42, height: 22)
    }

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        focusRingType = .none
        setAccessibilityElement(true)
        setAccessibilityRole(.checkBox)
        setAccessibilityRoleDescription("开关")
        setAccessibilityLabel("自动保存")
        setAccessibilityValue(0)
    }

    required init?(coder: NSCoder) {
        nil
    }

    override func draw(_ dirtyRect: NSRect) {
        super.draw(dirtyRect)

        let trackRect = bounds.insetBy(dx: 0, dy: 1)
        let trackPath = NSBezierPath(
            roundedRect: trackRect,
            xRadius: trackRect.height / 2,
            yRadius: trackRect.height / 2
        )
        let trackColor = isOn
            ? Self.onTrackColor
            : NSColor.systemGray.withAlphaComponent(0.55)
        trackColor.setFill()
        trackPath.fill()

        let knobDiameter = trackRect.height - 4
        let knobX = isOn
            ? trackRect.maxX - knobDiameter - 2
            : trackRect.minX + 2
        let knobRect = NSRect(
            x: knobX,
            y: trackRect.minY + 2,
            width: knobDiameter,
            height: knobDiameter
        )

        NSGraphicsContext.saveGraphicsState()
        let shadow = NSShadow()
        shadow.shadowBlurRadius = 1.5
        shadow.shadowOffset = NSSize(width: 0, height: -0.5)
        shadow.shadowColor = NSColor.black.withAlphaComponent(0.28)
        shadow.set()
        NSColor.white.setFill()
        NSBezierPath(ovalIn: knobRect).fill()
        NSGraphicsContext.restoreGraphicsState()
    }

    override func mouseDown(with event: NSEvent) {
        guard isEnabled else { return }
        toggleAndSendAction()
    }

    override func keyDown(with event: NSEvent) {
        if event.keyCode == 49 {
            toggleAndSendAction()
        } else {
            super.keyDown(with: event)
        }
    }

    override func accessibilityPerformPress() -> Bool {
        guard isEnabled else { return false }
        toggleAndSendAction()
        return true
    }

    private func toggleAndSendAction() {
        isOn.toggle()
        sendAction(action, to: target)
    }
}

@MainActor
final class AttachedTogglePanel: NSObject {
    private static let panelSize = NSSize(width: 132, height: 30)

    private let panel: NSPanel
    private let toggle: ColorStateSwitch
    private let label: NSTextField
    private var filePath: String?
    private let onChange: (String, Bool) -> Void

    init(onChange: @escaping (String, Bool) -> Void) {
        self.onChange = onChange

        panel = NSPanel(
            contentRect: NSRect(origin: .zero, size: Self.panelSize),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        toggle = ColorStateSwitch(frame: .zero)
        label = NSTextField(labelWithString: "自动保存")

        super.init()
        configurePanel()
    }

    func show(filePath: String, enabled: Bool, xmindFrame: CGRect) {
        self.filePath = filePath
        updateAppearance(enabled: enabled)
        panel.contentView?.toolTip = filePath

        let origin = panelOrigin(forXMindFrame: xmindFrame)
        if panel.frame.origin != origin {
            panel.setFrameOrigin(origin)
        }
        if !panel.isVisible {
            panel.orderFrontRegardless()
        }
    }

    func update(enabled: Bool, filePath: String) {
        self.filePath = filePath
        updateAppearance(enabled: enabled)
        panel.contentView?.toolTip = filePath
    }

    func hide() {
        filePath = nil
        panel.orderOut(nil)
    }

    private func configurePanel() {
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.becomesKeyOnlyIfNeeded = true
        panel.collectionBehavior = [
            .canJoinAllSpaces,
            .fullScreenAuxiliary,
            .ignoresCycle,
            .transient
        ]

        let materialView = NSVisualEffectView(frame: NSRect(origin: .zero, size: Self.panelSize))
        materialView.material = .popover
        materialView.blendingMode = .behindWindow
        materialView.state = .active
        materialView.wantsLayer = true
        materialView.layer?.cornerRadius = 9
        materialView.layer?.masksToBounds = true
        materialView.layer?.borderWidth = 0.5
        materialView.layer?.borderColor = NSColor.separatorColor.withAlphaComponent(0.45).cgColor
        panel.contentView = materialView

        label.font = .systemFont(ofSize: 12, weight: .medium)
        label.textColor = .labelColor
        label.translatesAutoresizingMaskIntoConstraints = false
        materialView.addSubview(label)

        toggle.target = self
        toggle.action = #selector(toggleChanged(_:))
        toggle.translatesAutoresizingMaskIntoConstraints = false
        materialView.addSubview(toggle)

        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: materialView.leadingAnchor, constant: 10),
            label.centerYAnchor.constraint(equalTo: materialView.centerYAnchor),
            toggle.trailingAnchor.constraint(equalTo: materialView.trailingAnchor, constant: -8),
            toggle.centerYAnchor.constraint(equalTo: materialView.centerYAnchor),
            label.trailingAnchor.constraint(lessThanOrEqualTo: toggle.leadingAnchor, constant: -7)
        ])
    }

    @objc private func toggleChanged(_ sender: ColorStateSwitch) {
        guard let filePath else { return }
        onChange(filePath, sender.isOn)
    }

    private func updateAppearance(enabled: Bool) {
        toggle.isOn = enabled
        label.textColor = .labelColor
    }

    private func panelOrigin(forXMindFrame frame: CGRect) -> NSPoint {
        let fallbackScreen = NSScreen.main ?? NSScreen.screens[0]
        let (screen, displayBounds) = screenAndDisplayBounds(containing: frame.origin) ?? (
            fallbackScreen,
            CGDisplayBounds(CGMainDisplayID())
        )

        let xmindLeft = screen.frame.minX + (frame.minX - displayBounds.minX)
        let xmindTop = screen.frame.maxY - (frame.minY - displayBounds.minY)
        let maximumOffset = max(12, frame.width - Self.panelSize.width - 12)
        let horizontalOffset = min(maximumOffset, max(170, min(250, frame.width * 0.21)))

        return NSPoint(
            x: xmindLeft + horizontalOffset,
            y: xmindTop - Self.panelSize.height - 7
        )
    }

    private func screenAndDisplayBounds(containing point: CGPoint) -> (NSScreen, CGRect)? {
        for screen in NSScreen.screens {
            guard let number = screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber else {
                continue
            }
            let bounds = CGDisplayBounds(CGDirectDisplayID(number.uint32Value))
            if bounds.contains(point) {
                return (screen, bounds)
            }
        }
        return nil
    }
}
