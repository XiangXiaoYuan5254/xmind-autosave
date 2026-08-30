import AppKit
import ApplicationServices
import Foundation

func copyAttribute(_ element: AXUIElement, _ attribute: CFString) -> CFTypeRef? {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, attribute, &value) == .success else {
        return nil
    }
    return value
}

func stringValue(_ value: CFTypeRef?) -> String {
    guard let value else { return "<nil>" }
    return String(describing: value)
}

func children(_ element: AXUIElement) -> [AXUIElement] {
    let ordinary = copyAttribute(element, kAXChildrenAttribute as CFString) as? [AXUIElement] ?? []
    return ordinary
}

guard let xmind = NSWorkspace.shared.runningApplications.first(where: {
    $0.bundleIdentifier == "net.xmind.vana.app" && !$0.isTerminated
}) else {
    fatalError("XMind is not running")
}

print("trusted=\(AXIsProcessTrusted()) pid=\(xmind.processIdentifier)")
let application = AXUIElementCreateApplication(xmind.processIdentifier)
let manualResult = AXUIElementSetAttributeValue(application, "AXManualAccessibility" as CFString, kCFBooleanTrue)
let enhancedResult = AXUIElementSetAttributeValue(application, "AXEnhancedUserInterface" as CFString, kCFBooleanTrue)
print("manualAccessibility=\(manualResult.rawValue) enhancedUI=\(enhancedResult.rawValue)")
guard let rawWindow = copyAttribute(application, kAXFocusedWindowAttribute as CFString),
      CFGetTypeID(rawWindow) == AXUIElementGetTypeID()
else {
    fatalError("No focused XMind window")
}

let window = unsafeDowncast(rawWindow, to: AXUIElement.self)
var stack: [(AXUIElement, Int)] = [(window, 0)]
var visited = 0

while let (element, depth) = stack.popLast(), visited < 1_000 {
    visited += 1
    let role = stringValue(copyAttribute(element, kAXRoleAttribute as CFString))
    let value = stringValue(copyAttribute(element, kAXValueAttribute as CFString))
    let title = stringValue(copyAttribute(element, kAXTitleAttribute as CFString))
    let description = stringValue(copyAttribute(element, kAXDescriptionAttribute as CFString))
    let url = stringValue(copyAttribute(element, kAXURLAttribute as CFString))

    if role.contains("WebArea") || url != "<nil>" || value.contains("已编辑") || title.contains("已编辑") {
        print("depth=\(depth) role=\(role)")
        print("  url=\(url)")
        print("  value=\(value)")
        print("  title=\(title)")
        print("  description=\(description)")

        var names: CFArray?
        if AXUIElementCopyAttributeNames(element, &names) == .success,
           let attributes = names as? [String] {
            print("  attributes=\(attributes.sorted())")
        }
    }

    stack.append(contentsOf: children(element).map { ($0, depth + 1) })
}

print("visited=\(visited)")
